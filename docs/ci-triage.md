# CI triage

This page records every failure found in the project's GitHub Actions runs, its root cause, the fix, and the commit that
fixed it. It separates failures seen in real runs from problems found by review before a run could expose them.

## Runs examined

| Run | Workflow | Trigger | Result |
|---|---|---|---|
| [41898341129](https://github.com/raunaksahahere/planTwin/actions/runs/41898341129) | ci | push to `main` (commit `d4f1c23`) | failure: 2 jobs failed, 1 skipped |
| [41968556607](https://github.com/raunaksahahere/planTwin/actions/runs/41968556607) | ci | pull request #1 (branch `ci/fix-runner-failures`) | success: 4 jobs green |

Only one run had failed when this triage began. `gh run list --limit 50` showed nothing else.

## Failures seen in real runs

| # | Workflow / job | Step | Exact error | Root cause | Fix | Status |
|---|---|---|---|---|---|---|
| 1 | ci / Go vet, format and unit tests | `make check` → `go vet` (inside `golang:1.24`) | `go: writing go.mod cache: mkdir /go/pkg/mod/cache: permission denied`; `internal/check/check.go:19:2: mkdir /go/pkg/mod/cache: permission denied` | The Makefile mounted a **named Docker volume** for the Go module cache. Docker creates a named volume owned by root. The runner runs containers as UID 1001, so the module cache was not writable. | The module cache is a host directory (`$HOME/.cache/plantwin-go`), owned by the runner user, mounted at `GOPATH=/tmp/go`. | Fixed. Green in run 41968556607. |
| 2 | ci / Extension (PostgreSQL postgres:18.5) | `docker build --build-arg PG_IMAGE=postgres:18.5 …` | `ERROR: docker.io/library/postgres:18.5: not found` | The matrix named a tag that does not exist. Checked against the Docker Hub tag list, which has `18.6`, `18.4`, `18.3`, and so on, but no `18.5`. | Matrix uses `postgres:18.4`, which exists. The comment in `ci.yml` records the check. | Fixed. Green in run 41968556607. |
| 3 | ci / Integration | not run | skipped, because it depends on the two jobs above | Dependency, not a separate fault. | none needed | Ran and passed in run 41968556607 (demo, redaction, least-privilege role). |

Failures 1 and 2 are independent. Each is a root cause in its own right, and each job has one step. Fixes were applied in one
commit (`ci: fix Go module cache permissions and an unpublished PostgreSQL tag`) and checked on a pull request.

## Latent problems found by review before a run exposed them

These were not in any failing log. They were found by running `actionlint` and `staticcheck` locally, and by reading the
workflows against the same root causes as failure 1. Each would have failed or weakened a later run.

| # | Where | Problem | Why it matters | Fix |
|---|---|---|---|---|
| L1 | `release.yml`, all Go builds | Same named-volume cache pattern as failure 1 (`GOPATH` not writable). | The release binaries job would fail the same way as the Go job. | Cache is a host directory; `GOPATH=/tmp/go`. |
| L2 | `release.yml` | Tag filter `v[0-9]+.[0-9]+.[0-9]+` is a glob, where `+` is literal. | No real tag such as `v0.1.0` would match, so the release would never run. | Filter is `v*.*.*`. |
| L3 | `release.yml` | Shell variables unquoted in `run:` (ShellCheck SC2086, reported by `actionlint`). | Word splitting on paths with spaces. | Quoted; `set -euo pipefail`. |
| L4 | `ci.yml`, `release.yml` | Third-party actions referenced by tag, not commit SHA. | Tags can move; the security checklist requires pinning. | Pinned to commit SHAs, with the version in a comment. |
| L5 | `release.yml` | Release notes were not taken from `CHANGELOG.md`. | A release with no notes, or notes that do not match the tag. | Notes come from the section for the version; the job fails if there is none. |
| L6 | `release.yml`, image job | Image digest was not signed. | Signing the tag or the release files does not cover the image itself. | `cosign sign` on `image@digest`, keyless. |
| L7 | `staticcheck` (not run in CI before) | Unused functions `relationRows`, `hasIndex`, `indexName` left over from the rule refactor; `removeAll` used only by an integration-tagged test. | Dead code, and a lint gate the project claimed but did not run. | Dead code removed; integration test calls `os.RemoveAll` directly; `staticcheck@2025.1.1` runs in `make check`. |
| L8 | `ci.yml` | No job followed the README quickstart on a clean runner. | The 10-minute rule was only measured on the development machine. | Added `quickstart` job: README commands, timed, and the report must contain the planted `lost_index` finding. |

## Found by the release and the examples (fixed forward)

These were not visible on `main`. The release workflow passed, and the problems appeared only when the published
artifacts were used from outside, which is what the `examples` workflow does. None was fixed by moving a tag. Each fix
was released as the next patch version.

| # | Found by | Problem | Root cause | Fix | Release |
|---|---|---|---|---|---|
| R1 | examples, SQL and Django, run [41973503679](https://github.com/raunaksahahere/planTwin/actions/runs/41973503679) against `v0.1.0` | The Action failed before PlanTwin started: `sha256sum: plantwin_linux_amd64.tar.gz: No such file or directory`. | The install step saved the tarball as `plantwin.tar.gz`, but `checksums.txt` lists `plantwin_linux_amd64.tar.gz`. `sha256sum -c` looks files up by name. Found because the Action had never been run from outside. | Keep the listed file name in both `action.yml` and the examples job. The install step was reproduced locally against the published `v0.1.0` assets before the fix. | `v0.1.1` (commit `4ae4b25`). The `v0.1.0` release notes now carry a known-issue note. |
| R2 | examples, Django, run [41976074760](https://github.com/raunaksahahere/planTwin/actions/runs/41976074760) against `v0.1.1` | `The migration command failed on the twin.` (the cause was not shown). | `migrationDir` joined `migrations.path` onto the revision for every tool. A command tool (`python manage.py migrate`) ran in a directory that a Django project does not have. The failure also hid the command's output unless `--verbose` was given. | A command tool runs from the revision root; a plain-SQL tool still reads `migrations.path`. A unit test covers both. The last lines of the command's output are now in the error. | `v0.1.2` (commit `5c68343`). |
| R3 | examples, Django, after R2 | The Action succeeded, so the planted regression was not reported. | Not a PlanTwin fault. Django creates its own index for every `ForeignKey`. Dropping the custom `orders_customer_idx` left an equivalent index on `customer_id`, so the example did not regress and PlanTwin correctly reported nothing. | The example's foreign key sets `db_index=False`, so the custom index is the only index on that column. Reproduced locally: the check reports `lost_index` on `shop_order`. | Example change only (no release needed). Merged in `examples: Django foreign key …`. |

Two latent problems were found while working on the release, and fixed in the same change:

| # | Where | Problem | Fix |
|---|---|---|---|
| L9 | `examples.yml`, the production database start | A socket readiness check (`pg_isready` over the Unix socket) can succeed while the image's initialisation server is still running and not yet listening on TCP. A migration that connects over TCP then fails. | Readiness is checked over TCP inside the container. |
| L10 | `Makefile`, `make build` | The version was hardcoded as `0.1.0` on the link line, so a development build reported a different version from the source. | Removed. The version comes from `cmd/plantwin/main.go`, which the release build sets explicitly. |

## Not reproduced locally

- **Runner-only behaviour.** The permission failure in #1 happens only because the runner's UID differs from the
  development machine's. It was reproduced by reading the log, not by running the job, and it was confirmed by the fix:
  the same job passed on the runner after the change.
- **`act`** (running Actions locally) is not installed. Running the workflows locally was not done.
- **Multi-architecture build.** The release built `linux/amd64` and `linux/arm64`, and the manifest lists both. No arm64
  container was started on an arm64 machine. See `docs/STATUS.md`.

## Re-running

```bash
gh run list --limit 50
gh run view <id> --log-failed
gh api repos/raunaksahahere/planTwin/actions/jobs/<job-id>/logs > job.log
```

Logs contain no secrets. The workflows use only the built-in `GITHUB_TOKEN`, scoped per job.
