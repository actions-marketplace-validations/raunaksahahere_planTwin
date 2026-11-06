# Limitations: what PlanTwin cannot catch

PlanTwin predicts plan changes from statistics. It does not run your queries on production data. Read this page
before trusting a clean report.

## Evidence limits (measured)

- **Fidelity is measured on two datasets only**: TPC-H SF1 (22 queries, DuckDB's `tpch` generator) and the Join Order
  Benchmark (113 queries, May 2013 IMDb snapshot). On both, with the hook and imported statistics, the twin reproduces
  the populated database's plan shapes for 112–113 of 113 JOB queries and 22 of 22 TPC-H queries
  (`bench/results/fidelity.md`). Your schema may behave differently.
- **Plan shapes agree, but not every estimate does**. On TPC-H, 21 of 22 full plan trees (including every row and cost estimate) match. One
  mismatch (JOB 33a) is a near-tie that I could not explain (`bench/results/fidelity.md`, narrative).
- **Recall and false-positive rate come from a 18-case synthetic corpus**, with 5 measured regressions. The rules
  were developed on the first 12 cases, so there is no held-out result. The runtime bar matters: at 1.5× recall drops
  to 5/8 (`bench/results/corpus/summary.md`).
- **The ground truth is one machine, one dataset, warm cache, three timed runs.** It is a measurement, not production.
- **Run-to-run noise is large on JOB.** The populated database's own plan shapes change between two `ANALYZE` runs for
  31 of 113 queries. A check result describes one statistics sample.

## What PlanTwin does not model

- **Extended statistics** (`CREATE STATISTICS`) are not exported and not restored. Multi-column estimates will differ.
  The CLI lists them and warns (ADR-0004). Verified: `pg_dump --statistics-only` carries no extended-statistics calls.
- **Parameter-sensitive plans.** Parameterised queries are explained with `GENERIC_PLAN`, the generic plan. A custom
  plan, chosen for a specific literal, can differ. Literal queries use the literal.
- **Data-dependent behaviour** that statistics do not describe: functions with side effects, volatile expressions,
  triggers, row-level security, foreign data wrappers, partitions beyond their statistics.
- **Execution, not planning.** PlanTwin compares estimates. It does not measure I/O, lock waits, cache state,
  concurrency, or the effect of `work_mem` on real spills. Cost is reported as info, not as proof.
- **Commit-time locks and migration duration.** A migration that takes an `ACCESS EXCLUSIVE` lock for minutes is out of
  scope. Migration linters (Squawk, Atlas) cover that.
- **Non-B-tree indexes.** Existing GiST, GIN, BRIN and hash indexes get their imported sizes, but no height estimate.
  New indexes of these types are not sized at all, so the planner sees them as empty. Treat findings that involve them
  as unreliable.

## Estimates the twin makes itself

- **B-tree height** is estimated from sizes and key widths (ADR-0002). It is wrong by one level for 2 of 44 JOB indexes.
- **New indexes** the migration creates are sized from the table's tuple count, with a fixed 10% factor for partial
  indexes (ADR-0010). Findings that depend on them are labelled *estimated*.
- **Tables the migration changed** have stale or partly missing statistics. Findings on them are labelled *estimated*.
- **New tables** have no statistics at all. Findings on them are labelled *estimated* and should be read with care.

## Redaction costs

Redaction changes estimates for literal predicates on sensitive common values (see the table in
`docs/privacy-and-redaction.md`). A clean report under `hash` does not prove the same report under `keep`.

## Operational limits

- **Migrations must run on an empty database.** `sql` and `command` tools run against the empty twin. A migration that
  reads data (a backfill with `UPDATE`) does not change the twin's statistics, which is correct, but it may fail on an
  empty table.
- **Tool support is partial.** `sql` (plain files, Flyway-style names) is applied directly. Django, Rails, Prisma,
  Alembic and Atlas are supported through `command` on the **host**, which must have their runtime installed.
  Repeatable Flyway migrations (`R__`) are not supported.
- **PostgreSQL 18 only**, in every 1.x release. The statistics import functions are PostgreSQL 18 features in this design.
- **Docker is required** for `check`, `demo` and `doctor`. There is no daemon-less mode.
- **Git worktrees.** `check` reads the committed revisions. Use `--head WORKTREE` to test uncommitted changes.
- **Bundle freshness.** `doctor` warns when a bundle is older than 14 days. Statistics drift; re-export regularly.

## Security boundaries

- The `command` migration tool runs a command from `plantwin.yaml` on the machine running `check`. If a pull request
  from an untrusted fork can change that file, it can run commands there. Run `check` in CI on trusted refs only, or
  without secrets (see SECURITY.md).
- The extension's CI marker is a convention, not authentication. A superuser can set it. It prevents accidental loads.
