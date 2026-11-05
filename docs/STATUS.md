# Status

Last updated: 2026-11-05. Current release: **v0.1.3**.

## Releases

| Tag | Release run | What it is | Known problems |
|---|---|---|---|
| [v0.1.0](https://github.com/raunaksahahere/planTwin/releases/tag/v0.1.0) | [41972680758](https://github.com/raunaksahahere/planTwin/actions/runs/41972680758) | First release. CLI, extension, image, Action. | The Action could not install the CLI (R1 in [ci-triage](ci-triage.md)). Use `v0.1.2`. The CLI and image are unaffected. |
| [v0.1.1](https://github.com/raunaksahahere/planTwin/releases/tag/v0.1.1) | [41975627987](https://github.com/raunaksahahere/planTwin/actions/runs/41975627987) | Fix forward: the Action's install step. | Command migration tools (Django, Rails, ...) failed on the twin (R2). Use `v0.1.2`. |
| [v0.1.2](https://github.com/raunaksahahere/planTwin/releases/tag/v0.1.2) | [41977000854](https://github.com/raunaksahahere/planTwin/actions/runs/41977000854) | Fix forward: command migration tools run from the revision root. | None known. |
| [v0.1.3](https://github.com/raunaksahahere/planTwin/releases/tag/v0.1.3) | [41981013377](https://github.com/raunaksahahere/planTwin/actions/runs/41981013377) | Marketplace branding metadata for the Action. No change to the CLI or the image. | None known. |

Tags are never moved or deleted. Each release fixed the previous one's problem as the next patch version.

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
  references this project's own Action at its tag (`raunaksahahere/planTwin@v0.1.2`), because that is the reference
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
| GitHub Marketplace listing | **Published, without a category** | listing [PlanTwin check](https://github.com/marketplace/actions/plantwin-check) links to this repository at `v0.1.3`. The listing's category list is empty, so it is not shown under any category. Adding one is the Marketplace step in [BLOCKERS](BLOCKERS.md#github-marketplace-listing-needs-the-web-interface). |

## Not verified

- **arm64 execution.** The arm64 image is in the manifest and was signed with the same digest, but no arm64 container was
  started on arm64 hardware.
- **The Action from another repository.** The examples workflow references the Action from this repository's examples.
  A consumer in a different repository has not been run.
- **Real replica.** `plantwin export` and `plantwin doctor` were tested against a container that plays the replica, not
  against a production replica.
- **Other linters.** `actionlint 1.7.7` and `staticcheck 2025.1.1` are clean. Other linters were not run.
- **Held-out benchmark.** The corpus rules were developed on the first 12 cases. A held-out workload is still missing.
- **Redaction default.** `hash` changes plans for common literals on categorical columns. Keeping it as the default is a
  product decision, not a verified fact.
- **Partition behaviour** in the extension is not covered by a regression test.

## What is complete

- C extension: chained hook, CI guard, default-off switch, B-tree height estimate, pg_regress tests.
- CLI: `init`, `export` (four redaction modes), `keygen`, `check`, `explain`, `accept`, `doctor`, `demo`, `version`.
- Reports: Markdown with side-by-side plans and "estimated" labels, JSON with `schema_version`, SARIF 2.1.0.
- Exit codes: 0 passed, 1 findings, 2 usage or configuration, 3 Docker or Git unavailable.
- Benchmarks, each with a script and raw output: TPC-H and JOB fidelity; a migration corpus with measured ground truth;
  redaction fidelity.
- CI, release and examples: [ci-triage](ci-triage.md) records each failure and fix; [ADR-0013](decisions/0013-ci-and-release-pipeline.md)
  records the design.
- Documentation: README, architecture, usage, privacy and redaction, limitations, rules, security, contributing, code of
  conduct, changelog, ADRs 0001–0013, learning guide, quickstart verification.
