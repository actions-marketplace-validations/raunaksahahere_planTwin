# Privacy, redaction and the export role

PlanTwin needs planner statistics from production. Those statistics can contain real values, so this page says
exactly what is read, what leaves the database, and what each redaction mode costs.

## What the export reads

`plantwin export` runs only SELECT statements against:

| Source | Contents | Can contain data values? |
|---|---|---|
| `pg_settings` (allow-listed names) | planner settings such as `work_mem`, `random_page_cost`, `enable_*` | no |
| `pg_class`, `pg_namespace`, `pg_attribute` | table and index names, page and tuple counts, column names and types | no |
| `pg_statistic` | per-column statistics: null fraction, width, distinct count, most common values, histogram bounds, correlation | **yes**: most common values and histogram bounds are real values |
| `pg_statistic_ext` | names of extended statistics objects (only the names are read) | no |

The export never reads table rows. A role that cannot read a table cannot read its rows through PlanTwin, and the
test in `internal/export/export_integration_test.go` checks that the documented role is refused when it reads rows.

## Export role

Use a dedicated read-only role on the **replica**, not the primary. Run these as a superuser on the replica, replacing
the names:

```sql
CREATE ROLE plantwin_export LOGIN PASSWORD '<generated secret>' NOSUPERUSER NOCREATEDB NOCREATEROLE;
GRANT CONNECT ON DATABASE app TO plantwin_export;
GRANT USAGE ON SCHEMA public TO plantwin_export;   -- one line per schema that holds tables to export
GRANT SELECT ON pg_statistic TO plantwin_export;   -- the only privilege beyond connecting
```

What this role **cannot** do (verified on PostgreSQL 18):

- `SELECT` on any table: `permission denied for table ...`.
- Write anything, create objects, or change settings beyond its session.

What this role **can** do, and why you should think about it:

- It can read `pg_statistic`, which holds the most common values of every analysed column. These values are real data.
  The privilege is needed because the `pg_stats` view filters columns by table `SELECT`, and `pg_read_all_stats` does
  not cover `pg_statistic` (checked on PostgreSQL 18). PlanTwin therefore reads the catalog directly, with the same
  decoding PostgreSQL's view uses.
- So the role's **access** to common values is not limited by PlanTwin's redaction. The redaction limits what
  **leaves** the replica, in the bundle. Treat the role's credentials like any production read credential. Keep it on
  the replica, rotate it, and do not give it to CI.

PlanTwin checks the privilege before it exports and fails with the exact GRANT to run.

The export also sets its own session on the server: `default_transaction_read_only` is `on`, `statement_timeout` is five
minutes and `lock_timeout` is five seconds, and the session identifies itself as `plantwin-export` in `pg_stat_activity`.
If the server does not apply read-only mode, for example because a connection pooler drops startup parameters, the export
refuses to run. These settings are applied by the export itself, so they are a check on the export's behaviour, not a
replacement for the role's grants: keep the role to the grants listed above. The refusal path has no automated test
against a live server yet ([STATUS](STATUS.md)).

## Redaction modes

Set `redaction.default` and, per column, `redaction.columns` (keys are `table.column`) in `plantwin.yaml`.

| Mode | What the bundle keeps | Planner effect | Risk |
|---|---|---|---|
| `keep` | every value, verbatim | exact estimates | the bundle contains real common values |
| `hash` (default) | frequencies of the common values, and a same-type surrogate per value (integers: a keyed hash in 31 bits; text: `h` plus 12 hex digits). The histogram is dropped. | literal predicates on a common value get the planner's default estimate, not the real one | surrogates cannot be matched to values without the per-export salt, which is discarded |
| `drop-mcv` | no common values and no frequencies | equality estimates fall back to the distinct count; no frequency information | nothing sensitive is kept for that column |
| `drop-histogram` | common values kept, histogram bounds dropped | range estimates fall back to defaults | common values still visible |

Column types with no surrogate (for example timestamps, booleans, UUIDs) fall back to `drop-mcv` under `hash`.

### Measured fidelity cost

`bench/redaction/run_redaction.py` runs the demo (500k synthetic orders) three times in each mode and compares every
plan with the `keep` runs. Results are in `bench/results/redaction/fidelity.md`, generated from the raw plans. The
table is copied here after the final run:

| Mode | Shape agreement with keep | Median top-cost error | Verdict agreement with keep | Plans that changed shape |
|---|---:|---:|---:|---|
| keep (reference) | 24/24 | 0.19% | 6/6 | none |
| hash | 18/36 | 51.63% | 9/9 | `open_orders_count` (base and head) |
| drop-mcv | 18/36 | 44.38% | 9/9 | `open_orders_count` (base and head) |
| drop-histogram | 36/36 | 0.13% | 9/9 | none |

What this shows, on this dataset:

- **Common literal values need their frequencies.** `status = 'open'` matches 5% of rows. With `hash` or `drop-mcv` the
  planner no longer knows that value, and its estimate fell from about 25,000 rows to 1. The plan changed from a bitmap
  scan to an index-only scan. The verdict did not change here, because the migration's finding is on another query.
  A migration that depends on such a literal could be missed.
- **The histogram does not matter for equality predicates**, so `drop-histogram` costs nothing on these queries. It
  would matter for range predicates, which this dataset does not test.
- **The noise floor is 0.19%** (keep against keep, with separate `ANALYZE` samples). The mode effects above are two
  orders of magnitude larger.

Read the table with its limits: one synthetic dataset, two queries, one PostgreSQL version, three runs per mode.
`bench/results/redaction/fidelity.md` has the raw comparison.

**Recommendation.** Use `hash` (the default) for columns that hold personal or confidential values. Use `keep` for
low-cardinality categorical columns that you are willing to expose, such as `status`, and that your queries filter on:

```yaml
redaction:
  default: hash
  columns:
    orders.status: keep
```

Then run `plantwin check` on a migration that changes a query on that column, and confirm that the plan still matches
production (`plantwin explain`).

## Salt and signing

- Each export draws a fresh 32-byte salt. It is never written to the bundle.
- The bundle is signed with ed25519 (`plantwin export --sign-key`). `check` and `doctor` refuse an unsigned or
  modified bundle. Keep the private key out of git (`plantwin keygen` writes it with mode 0600).

## What goes into CI

Only the bundle and its `.sig` file, and only the public key, are needed in the repository or in CI. The export DSN
is needed only where the export runs. Never give the export DSN to a pull request from a fork.
