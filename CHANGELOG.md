# Changelog

All notable changes are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
PlanTwin follows semantic versioning from v0.1.0. The JSON report carries its own `schema_version`.

## [Unreleased]

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

### Not included in this release
- Published images, binaries, or GitHub releases. The release workflow is written but has not run.
