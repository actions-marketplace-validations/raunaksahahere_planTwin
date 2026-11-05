# Changelog

All notable changes are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
PlanTwin follows semantic versioning from v0.1.0. The JSON report carries its own `schema_version`.

## [1.0.0] - 2026-11-05

First production release. From 1.0.0 the compatibility promise in [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) applies:
CLI flags, exit codes, configuration keys and the JSON report's `schema_version` 1 do not change incompatibly within 1.x.

### Changed
- `plantwin.yaml` with an unknown key is now an error that names the file and line. Before, a misspelled key such as
  `threshold:` was ignored and the check ran with defaults. Check your file: every example and the `plantwin init`
  template use only known keys.
- The statistics export runs in a read-only session with a five-minute statement timeout and a five-second lock timeout.
  It identifies itself as `plantwin-export` in `pg_stat_activity`. If the server does not apply read-only mode (for
  example, a connection pooler drops startup parameters), the export refuses to run.
- A DSN that cannot be parsed is reported without its details, because the parser's message can contain the password.
- Release binaries are built for linux/amd64, linux/arm64, darwin/amd64 and darwin/arm64.
- The GitHub Action's default `version` is `v1.0.0`.

### Added
- `plantwin check --timeout` (default 30 minutes). A check that does not finish in time stops with a clear message.
- The GitHub Action detects an x86-64 or arm64 Linux runner and installs the matching binary.
- The quickstart and the examples workflow run on Linux x86-64 and arm64.
- An extension regression test for partitioned tables: each partition is sized from its own imported statistics, and
  the partitioned parent adds no scan of its own.

### Known limits
These are unchanged from 0.1.x and are stated in [docs/limitations.md](docs/limitations.md):
- Only PostgreSQL 18 is supported.
- Extended statistics are reported but not restored.
- Parameter-sensitive custom plans are not compared; parameterised queries use the generic plan.
- Accuracy figures come from a synthetic corpus. PlanTwin is not measured on your workload until you run it on it.

## [0.1.3] - 2026-11-05

### Added
- GitHub Marketplace metadata for the Action: a `branding` block (icon and colour). No change to the CLI, the image or
  the checks.

## [0.1.2] - 2026-11-05

### Fixed
- Migration tools that run a command (Django, Rails, Prisma, Alembic, Atlas) ran in the `migrations/` directory, which
  those projects do not have, so the twin failed to migrate. A command tool now runs from the revision root. Found by
  the examples workflow against `v0.1.1` (the Django example).
- A failed migration command now shows the last lines of its own output, so the cause is visible without `--verbose`.

## [0.1.1] - 2026-11-05

### Fixed
- The GitHub Action could not install the CLI from a release: it saved the tarball under a different name from the one
  `checksums.txt` lists, so the checksum check failed before any PlanTwin code ran. Found by the `examples` workflow
  against `v0.1.0`. The release notes for `v0.1.0` carry the same warning. The CLI and the image are unchanged.
- The examples workflow now references the Action at `v0.1.1`.

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

