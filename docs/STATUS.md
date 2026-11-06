# Status

Last updated: 2026-11-06. Current release: **v1.0.0**, tagged on `7c0de91`. The previous release was v0.1.3.

## Releases

| Tag | Release run | What it is | Known problems |
|---|---|---|---|
| [v0.1.0](https://github.com/raunaksahahere/planTwin/releases/tag/v0.1.0) | [41972680758](https://github.com/raunaksahahere/planTwin/actions/runs/41972680758) | First release. CLI, extension, image, Action. | The Action could not install the CLI (R1 in [ci-triage](ci-triage.md)). Use `v0.1.2`. The CLI and image are unaffected. |
| [v0.1.1](https://github.com/raunaksahahere/planTwin/releases/tag/v0.1.1) | [41975627987](https://github.com/raunaksahahere/planTwin/actions/runs/41975627987) | Fix forward: the Action's install step. | Command migration tools (Django, Rails, ...) failed on the twin (R2). Use `v0.1.2`. |
| [v0.1.2](https://github.com/raunaksahahere/planTwin/releases/tag/v0.1.2) | [41977000854](https://github.com/raunaksahahere/planTwin/actions/runs/41977000854) | Fix forward: command migration tools run from the revision root. | None known. |
| [v0.1.3](https://github.com/raunaksahahere/planTwin/releases/tag/v0.1.3) | [41981013377](https://github.com/raunaksahahere/planTwin/actions/runs/41981013377) | Marketplace branding metadata for the Action. No change to the CLI or the image. | None known. |
| [v1.0.0](https://github.com/raunaksahahere/planTwin/releases/tag/v1.0.0) | [42085113513](https://github.com/raunaksahahere/planTwin/actions/runs/42085113513) | First production release. Strict configuration, read-only export session, `check --timeout`, linux/arm64 and darwin/amd64 binaries, arm64 Action support. See [COMPATIBILITY](COMPATIBILITY.md). | Node 20 deprecation warning from `actions/checkout` in CI. It is a warning; the jobs pass. Follow-up: move to a checkout version that runs on Node 24. |

Tags are never moved or deleted. Each release fixed the previous one's problem as the next patch version.

## v1.0.0: released

Pull request [#12](https://github.com/raunaksahahere/planTwin/pull/12) was merged as `7c0de91`, and the annotated tag
`v1.0.0` was pushed to that commit. The gates in [ADR-0014](decisions/0014-one-point-zero-readiness.md) were checked as
follows.

| Gate | Result | Evidence |
|---|---|---|
| CI on `main` | Passed | [run 42084539690](https://github.com/raunaksahahere/planTwin/actions/runs/42084539690) on `7c0de91`: Go vet and unit tests, extension on postgres:18.6 and 18.4, integration, and both quickstart jobs |
| Quickstart on `ubuntu-24.04` and `ubuntu-24.04-arm` | Passed | the same run. The arm64 quickstart ran on GitHub's hosted arm64 runner for the first time |
| Release workflow | Passed | [run 42085113513](https://github.com/raunaksahahere/planTwin/actions/runs/42085113513): binaries, signed multi-architecture image, GitHub Release |
| Release files, checked from outside the build | Verified | downloaded from the release: `sha256sum -c checksums.txt` OK for all six files; `cosign verify-blob` on `checksums.txt` with identity `…/release.yml@refs/tags/v1.0.0` → `Verified OK`; the Linux amd64 binary prints `plantwin 1.0.0` |
| Examples against `v1.0.0`, both architectures | Passed | [examples run 42086568667](https://github.com/raunaksahahere/planTwin/actions/runs/42086568667): SQL and Django examples on `ubuntu-24.04` and `ubuntu-24.04-arm`. Each job pulled the image, verified its signature at the tag, installed the release binary, ran the published Action, and asserted the planted `lost_index` regression |
| Release notes | Checked | taken from the 1.0.0 section of the CHANGELOG; no attribution text in the notes or the tag |

Gates run before the pull request, on the same contents: `make check`, `make test-ext` (3 of 3), `make test-integration`,
the x86-64 quickstart steps, `actionlint` 1.7.7, the Action's install block against the published `v0.1.3` release, a
1 ns `--timeout` check, and a check that a malformed DSN does not print its password. All passed.

## Pre-tag gate (applied to each release)

| Gate | Result | Evidence |
|---|---|---|
| CI fully green on `main` | Passed | [run 41978303259](https://github.com/raunaksahahere/planTwin/actions/runs/41978303259) (`main`, all jobs). Also green on the commit of each tag: runs [41972369112](https://github.com/raunaksahahere/planTwin/actions/runs/41972369112), [41975331192](https://github.com/raunaksahahere/planTwin/actions/runs/41975331192), [41976682833](https://github.com/raunaksahahere/planTwin/actions/runs/41976682833). |
| `make check` | Passed | run locally on the v0.1.0 commit, and in CI on the commit of each tag; includes `gofmt`, `go vet` (also with `-tags integration`), pinned `staticcheck 2025.1.1`, unit and golden tests |
| `make test-ext` | Passed | run locally on the v0.1.0 commit, and in CI on each tagged commit; the Dockerfile's test stage fails on any pg_regress failure |
| `make test-integration` | Passed | run locally on the v0.1.0 commit, and in CI on each tagged commit: demo, redaction leak, least-privilege role |
| Quickstart on a clean runner | Passed | `quickstart` job: 65 s total, the report contains the planted `lost_index` ([run 41969273883](https://github.com/raunaksahahere/planTwin/actions/runs/41969273883); [details](quickstart-verification.md)) |
| Security checklist | Passed | see below |
| Documents make no unverified claim | Passed at each tag | claims are linked to the runs that verify them. Items not verified are listed under "Not verified". |

### Security checklist

- **Least-privilege tokens.** Every workflow sets `contents: read`. Only publishing jobs raise a scope:
  `contents: write` to create the release, `packages: write` to push the image, `id-token: write` for keyless signing.
  Every checkout uses `persist-credentials: false`. No workflow uses `pull_request_target`.
- **Pinned actions.** Every third-party action is pinned to a commit SHA. One exception, by design: the examples workflow
  references this project's own Action at its release tag (for example `raunaksahahere/planTwin@v1.0.0`), because that is the reference
  consumers use and the thing the job tests. See [ADR-0013](decisions/0013-ci-and-release-pipeline.md).
- **No secrets in logs or fixtures.** Twin containers get a random password per container, and they do not use `trust`
  authentication. The least-privilege test role's password is generated per run. The Django example reads `SECRET_KEY`
  from the environment. The examples workflow masks its generated passwords. The only credential the workflows use is the
  built-in `GITHUB_TOKEN`.
- **Extension loads only with the CI marker.** Tested by `plantwin_guard` in `make test-ext` and in CI. A
  `shared_preload_libraries` entry stops the server from starting (verified manually; see ADR-0001).

## Post-tag verification (v0.1.2, from outside the build)

| Item | Result | Evidence |
|---|---|---|
| Release files signed with keyless cosign | Verified | `cosign verify-blob` on `checksums.txt` with the identity `…/release.yml@refs/tags/v0.1.2` → `Verified OK` |
| Every release file matches its checksum | Verified | `sha256sum -c checksums.txt`: all four binaries and the tarball OK |
| Image pulled anonymously by tag | Verified | the GHCR package is public; `docker pull ghcr.io/raunaksahahere/plantwin-postgres:18` succeeds without a login |
| Image digest signed by the release workflow | Verified | `sha256:a46bf7f5b7cef2bc724e1d03fc8466443484c20b1eafe7e64b2c002512c733f3`; cosign `verify` exits 0 for identity `release.yml@refs/tags/v0.1.2`; the entry is in the transparency log |
| Manifest covers both architectures | Verified (manifest only) | `docker buildx imagetools inspect` lists `linux/amd64` and `linux/arm64` |
| Released binary runs `plantwin demo` | Verified | `plantwin 0.1.2`; `plantwin demo` with the published image exits 0 and reports the planted `lost_index` |
| Action at the tag, SQL example | Verified in CI | [examples run 41978583686](https://github.com/raunaksahahere/planTwin/actions/runs/41978583686): the Action installs `v0.1.2`, the check runs, the planted finding is reported, the expected failure is asserted |
| Action at the tag, Django example | Verified in CI | same run: the Django migration runs in the twin, and the planted regression is reported |
| Action at `v0.1.0` | Failed, as recorded | [run 41973503679](https://github.com/raunaksahahere/planTwin/actions/runs/41973503679) (R1) |
| Action at `v0.1.3` (Marketplace branding) | Verified | `action.yml` at the tag has `branding`; [examples run 41981570894](https://github.com/raunaksahahere/planTwin/actions/runs/41981570894) passes for both examples |
| GitHub Marketplace listing | **Published, with a category** | listing [PlanTwin check](https://github.com/marketplace/actions/plantwin-check) is at release `v1.0.0`. The category was added through the steps in [BLOCKERS](BLOCKERS.md#github-marketplace-listing-needs-the-web-interface). |

## Not verified

- **arm64 execution.** Covered only by GitHub's hosted `ubuntu-24.04-arm` runners in CI (quickstart and examples). No
  other arm64 hardware has been used, and no arm64 replica has been tested.
- **The Action from another repository.** The examples workflow references the Action from this repository's examples.
  A consumer in a different repository has not been run.
- **Real replica.** `plantwin export` and `plantwin doctor` were tested against a container that plays the replica, not
  against a production replica.
- **Other linters.** `actionlint 1.7.7` and `staticcheck 2025.1.1` are clean. Other linters were not run.
- **Held-out benchmark.** The corpus rules were developed on the first 12 cases. A held-out workload is still missing.
- **Redaction default.** `hash` changes plans for common literals on categorical columns. Keeping it as the default is a
  product decision, not a verified fact.
- **The read-only export refusal** (a session where the server did not apply read-only mode) has no test. The code path
  exists; it has not been run against a server that drops startup parameters.

## What is complete

- C extension: chained hook, CI guard, default-off switch, B-tree height estimate, pg_regress tests.
- CLI: `init`, `export` (four redaction modes), `keygen`, `check`, `explain`, `accept`, `doctor`, `demo`, `version`.
- Reports: Markdown with side-by-side plans and "estimated" labels, JSON with `schema_version`, SARIF 2.1.0.
- Exit codes: 0 passed, 1 findings, 2 usage or configuration, 3 Docker or Git unavailable.
- Benchmarks, each with a script and raw output: TPC-H and JOB fidelity; a migration corpus with measured ground truth;
  redaction fidelity.
- CI, release and examples: [ci-triage](ci-triage.md) records each failure and fix; [ADR-0013](decisions/0013-ci-and-release-pipeline.md)
  records the design.
- Documentation: README, architecture, usage, compatibility promise, privacy and redaction, limitations, rules, security,
  contributing, code of conduct, changelog, ADRs 0001–0014, learning guide, quickstart verification.
