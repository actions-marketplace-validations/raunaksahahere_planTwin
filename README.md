# PlanTwin

PlanTwin catches PostgreSQL query-plan regressions in pull requests, using production planner statistics and no production rows.

```
## PlanTwin: HEAD~1 → HEAD

**Result: failing.** 1 error(s), 0 warning(s), 1 info.

> **Estimated:** statistics for orders were estimated, not imported. Findings on them are labelled.

### `customer_lookup`

- **ERROR** `lost_index` *(estimated)*: Index use lost on orders: a selective index is now unused, and the table is read by a sequential scan (about 500K rows).
  - Why this matters: Queries on this table used a selective index. Without it, PostgreSQL reads every row of the table.
  - Suggested fix: Recreate the index, or add it back in this migration. To accept the change, run `plantwin accept`.

<details><summary>Plans, side by side</summary>

| HEAD~1 | HEAD |
|---|---|
| `Bitmap Heap Scan on orders  rows=28 cost=110` | `Gather  rows=28 cost=7774` |
| `  Bitmap Index Scan using orders_customer_idx  rows=28 cost=5` | `  Seq Scan on orders  rows=12 cost=6771` |

</details>
```

*That is the built-in demo's real output (`plantwin demo`). The sample is from a pre-release build; see the quickstart verification for the current output.*

## Quickstart

From this repository:

```bash
make image        # builds plantwin-postgres:18 locally (the extension is installed, not preloaded)
make build        # builds bin/plantwin
./bin/plantwin demo   # synthetic production, a migration that drops an index, and the report
```

Needs Docker and git. The demo creates its own repository, database and signing key in a temporary directory and
removes them afterwards. The first run takes about 1–2 minutes (the image build); later runs take about 30 seconds.
Timing for a fresh clone is recorded in [docs/quickstart-verification.md](docs/quickstart-verification.md).

For your own repository:

```bash
plantwin init                                  # detects the migration tool, writes plantwin.yaml
plantwin keygen --priv plantwin.key --pub plantwin.pub
PLANTWIN_DSN=postgres://plantwin_export:…@replica/app plantwin export --sign-key plantwin.key
plantwin check --base main --head HEAD         # exit 0 = passed, 1 = findings failed, 2 = config, 3 = Docker or Git missing
```

Run `plantwin doctor` at any point to check Docker, the image, the bundle and the configuration. Every error says what
happened, why, and what to do next. Add `--verbose` to see the underlying error.

## How it works

Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the diagram and the code map. In short:

1. **Export** (on a read replica, with a read-only role): reads planner statistics and settings, never table rows,
   redacts common values, and writes a signed bundle.
2. **Twin** (in CI): two PostgreSQL 18 containers, one at the base revision and one at the head. Each gets the same
   statistics. A small C extension, loaded only in CI, makes the planner use them.
3. **Compare**: the query catalog is explained on both twins. Plans are reduced to shape trees (operators, relations,
   indexes, join order) and diffed. Rules turn differences into findings with thresholds you can change.

## Evidence

Every number below comes from a script in `bench/` and raw output saved next to it.

| Question | Result | Where |
|---|---|---|
| Does an empty database with imported statistics plan like production, without the hook? | No: 0 of 22 TPC-H plans and 0 of 113 JOB plans have the same shape | `bench/results/fidelity.md` |
| With the hook? | TPC-H: 22 of 22 shapes in both runs, 21 of 22 full trees. JOB: 112 of 113 and 113 of 113. | `bench/results/fidelity.md` |
| How much does ANALYZE sampling move the answer? | On JOB, 31 of 113 populated plans change shape between two runs. The twin changes in the same 31. | `bench/results/fidelity.md` |
| Does the tool catch planted regressions, and does it stay quiet on benign migrations? | 18-case corpus, measured ground truth: recall 5/5 and 0 false positives at a 2× runtime bar. Not held out: rules were developed on the first 12 cases. | `bench/results/corpus/summary.md` |
| What does redaction cost? | Measured on the demo dataset for each mode | `bench/results/redaction/fidelity.md` |

Re-run the benchmarks with `bench/spike/` (TPC-H and JOB fidelity), `bench/corpus/run_corpus.sh` (migration corpus),
and `bench/redaction/run_redaction.py` (redaction). The data sources and their limits are in
[docs/decisions/0005-benchmark-sources-and-config.md](docs/decisions/0005-benchmark-sources-and-config.md).

## Privacy and redaction

The export role needs `CONNECT`, `USAGE` on each schema, and `SELECT` on `pg_statistic`. It cannot read table rows.
Common values can still appear in `pg_statistic`, which the role can read, so treat the role like any production
read credential. The bundle is redacted (`hash` by default), signed, and contains no rows. The exact GRANTs, the
modes, and their measured cost are in [docs/privacy-and-redaction.md](docs/privacy-and-redaction.md).

## What PlanTwin cannot catch

Read [docs/limitations.md](docs/limitations.md) before trusting a clean report. In particular: extended statistics are
not restored; parameter-sensitive custom plans are not compared; execution cost, locks and I/O are not measured; and
PostgreSQL 18 is the only supported version in v0.1.

## Roadmap

Not in v0.1, and not started:

- HypoPG-based "what-if" index suggestions.
- Upgrade preview: the same statistics bundle under PostgreSQL N and N+1.
- Repeatable Flyway (`R__`) migrations and more native migration runners.
- Published multi-architecture images and signed releases (the workflows are written; nothing is published from here).
- A held-out migration corpus with an independent set of queries.

## Prior art

- **pg-plan-guard** snapshots plan shapes into a lock file, but needs representative data in the CI database.
- **Database Lab Engine** provides thin clones of production, which need production data and dedicated infrastructure.
- **HypoPG** and **pg_dbms_stats** use the same planner hook mechanism that PlanTwin's extension uses.
- **GitLab's database migration pipeline** runs migrations against production-like clones.
- Migration linters (**Squawk**, **Atlas**, **pgroll**) check DDL safety, not plan behaviour.

## Licence

The CLI and the GitHub Action are Apache-2.0 ([LICENSE](LICENSE)). The C extension is under the PostgreSQL License
([ext/plantwin/LICENSE](ext/plantwin/LICENSE)).

## Contributing and security

[CONTRIBUTING.md](CONTRIBUTING.md) · [SECURITY.md](SECURITY.md) · [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) · [CHANGELOG.md](CHANGELOG.md) · [docs/STATUS.md](docs/STATUS.md) · [docs/LEARNING.md](docs/LEARNING.md)
