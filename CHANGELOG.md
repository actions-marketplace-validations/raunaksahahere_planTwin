# Changelog

All notable changes are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
PlanTwin follows semantic versioning from v0.1.0. The JSON report carries its own `schema_version`.

## [0.1.0] - 2026-11-05

First release. PlanTwin is a pre-1.0 tool; the CLI flags, the configuration keys and the JSON report may still change
before 1.0. The JSON report carries `schema_version: 1`.


### Added
- C extension `plantwin` (PostgreSQL 18): chained `get_relation_info_hook`, CI-marker guard, default-off switch,
  B-tree height estimate, pg_regress tests.
- CLI `plantwin`: `init`, `export`, `keygen`, `check`, `explain`, `accept`, `doctor`, `demo`, `version`.
- Stats bundle with signing (ed25519) and four redaction modes.
- Shape trees, LCS differ, and seven rules with documented thresholds.
- Reports: Markdown with side-by-side plans, JSON (`schema_version`), SARIF 2.1.0.
- Benchmarks: TPC-H and JOB fidelity; 18-case migration corpus with measured ground truth; redaction fidelity.
- Twin image `plantwin-postgres:18` (built locally from the Dockerfile).

### Fixed during development (recorded for transparency)
- The export role cannot use `pg_read_all_stats` for `pg_stats`; it reads `pg_statistic` directly (verified on PG18).
- Parallel `Gather` plans hid sequential scans from the lost-index rule.
- `lost_index` fired on low-selectivity indexes; now selectivity-aware (ADR-0008).
- Cold-cache timings produced a false positive; the benchmark now takes a warm-up run.
- The first benchmark scorer ignored fast-to-slow regressions (the floor applied to the base time).

### Continuous integration (found and fixed before this release)
- Go module cache: a root-owned Docker volume made `go` unable to write its cache on the runner. Now a host directory.
- Extension matrix: `postgres:18.5` does not exist on Docker Hub; the matrix uses `postgres:18.4`.
- Release workflow: tag filter, quoting, action pinning and release notes were corrected before any tag was pushed.
- Lint: `staticcheck` (pinned) runs in `make check`; dead code it reported was removed.
- A `quickstart` job follows the README on a clean runner and requires the planted finding in the report.
- See [docs/ci-triage.md](docs/ci-triage.md) for each root cause.

### Known limits
- Extended statistics are reported but not restored. Multi-column estimates will differ from production.
- Parameter-sensitive custom plans are not compared; parameterised queries use the generic plan.
- Only PostgreSQL 18 is supported.
- The `hash` redaction default changes plans for common literals on categorical columns (measured; see the privacy guide).
- Benchmark recall and false-positive numbers come from a synthetic corpus whose rules were developed on part of it.

## [Unreleased]

