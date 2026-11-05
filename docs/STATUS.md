# Status

Last updated: 2026-11-05.

## Pre-tag release gate: passed

The release gate is applied to the exact commit to be tagged. Each row names its evidence.

| Gate | Result | Evidence |
|---|---|---|
| CI fully green on `main` (all jobs) | Passed | [run 41971615328](https://github.com/raunaksahahere/planTwin/actions/runs/41971615328) on `main` at `b8fa420`: Go, extension on PostgreSQL 18.6 and 18.4, quickstart, integration |
| `make check` | Passed | run locally on `b8fa420`; includes `gofmt`, `go vet` (also with `-tags integration`), pinned `staticcheck 2025.1.1`, unit and golden tests |
| `make test-ext` (pg_regress, guard, hook) | Passed | run locally; the test stage of the Dockerfile fails the build on any regression |
| `make test-integration` (demo, redaction leak, least-privilege role) | Passed | run locally; the same tests run in CI |
| Quickstart on a clean runner | Passed | `quickstart` job: 65 s total, the report contains the planted `lost_index` finding ([docs/quickstart-verification.md](quickstart-verification.md)) |
| Security checklist | Passed, with one documented exception | see below |
| Docs contain no unverified claim | Passed at this commit | claims about CI, the quickstart and the tests are linked to runs above. The release and the examples are described as pending until they run. |

### Security checklist

- **Least-privilege tokens.** Every workflow sets `contents: read`. Only the jobs that publish raise a scope:
  `contents: write` to create the release, `packages: write` to push the image, `id-token: write` for keyless signing.
  Every checkout uses `persist-credentials: false`. No workflow uses `pull_request_target`.
- **Pinned actions.** Every third-party action is pinned to a commit SHA. One exception, by design: the examples
  workflow references this project's own Action at its tag (`raunaksahahere/planTwin@v0.1.0`), because that reference is
  what consumers use and what the job tests. See [ADR-0013](decisions/0013-ci-and-release-pipeline.md).
- **No secrets in logs or fixtures.** Twin containers get a random password per container, and the containers do not use
  `trust` authentication. The least-privilege test role's password is generated per run. The Django example reads
  `SECRET_KEY` from the environment. Workflow secrets are masked with `::add-mask::`. The only credential the workflows
  read is the built-in `GITHUB_TOKEN`.
- **The extension loads only with the CI marker.** Tested by `plantwin_guard` in `make test-ext` and in CI. A
  `shared_preload_libraries` entry stops the server from starting (verified manually; see ADR-0001).

## Post-tag verification

These depend on the tagged release and are run after the tag exists. Results are recorded here when they are known.

| Item | Status |
|---|---|
| Release workflow publishes multi-architecture image (`linux/amd64`, `linux/arm64`) to GHCR | pending |
| Keyless signature on the checksum file verifies | pending |
| Keyless signature on the image digest verifies | pending |
| Image pulled by digest from outside and started | pending |
| Released binary: checksum verifies, `plantwin demo` runs from it | pending |
| Examples workflow (SQL and Django) passes against the tag, with the published Action | pending |

If any post-tag item fails, the tag is not moved or deleted. The fix is made on `main`, and the next patch version is
tagged.

## What is not verified

- **Lint beyond the pinned set.** `actionlint 1.7.7` and `staticcheck 2025.1.1` are clean. Other linters were not run.
- **Real replica.** `plantwin export` and `plantwin doctor` were tested against a container that plays the replica, not
  against a production replica.
- **Held-out benchmark.** The corpus rules were developed on the first 12 cases. A held-out workload is still missing.
- **Redaction default.** `hash` changes plans for common literals on categorical columns. Whether to keep it as the
  default is a product decision, not a verified fact.
- **Partition behaviour** in the extension is not covered by a regression test.

## What is complete

- C extension: chained hook, CI guard, default-off switch, B-tree height estimate, pg_regress tests.
- CLI: `init`, `export` (four redaction modes), `keygen`, `check`, `explain`, `accept`, `doctor`, `demo`, `version`.
- Reports: Markdown with side-by-side plans and "estimated" labels, JSON with `schema_version`, SARIF 2.1.0.
- Exit codes: 0 passed, 1 findings, 2 usage or configuration, 3 Docker or Git unavailable.
- Benchmarks, each with a script and raw output: TPC-H and JOB fidelity; a migration corpus with measured ground truth;
  redaction fidelity.
- CI and release: [ci-triage](ci-triage.md) records the failures and their fixes; [ADR-0013](decisions/0013-ci-and-release-pipeline.md)
  records the design.
- Documentation: README, architecture, usage, privacy and redaction, limitations, rules, security, contributing, code of
  conduct, changelog, ADRs 0001–0013, learning guide, quickstart verification.
