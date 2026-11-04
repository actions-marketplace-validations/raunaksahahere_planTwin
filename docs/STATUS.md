# Status

**Release decision: v0.1.0 is not tagged.** One gate cannot be passed in this environment, and several others are
only partly met. The rule for tagging is that every gate passes. The gates and their status are below, with the
evidence for each.

Last updated: 2026-11-04, at commit `94e411a` (or later on `main`; see `git log`).

## Gates

| Gate | Status | Evidence |
|---|---|---|
| **Step 0**: the gap is shown on TPC-H with the hook | Met | `docs/why-a-hook.md`, `bench/results/step0/` |
| **Phase 1**: extension builds on PG18, guard works, pg_regress passes | Met | `make test-ext` (clean build); `shared_preload_libraries` refusal verified |
| **Phase 2**: fidelity on TPC-H (22) and JOB (113), every mismatch explained | Met, with residuals named | `bench/results/fidelity.md` |
| **Phase 3**: canonicaliser, differ, rules, golden, planted and benign tests; recall and FP measured | Met, **not held out** | `bench/results/corpus/summary.md`; golden tests in `internal/rules` |
| **Phase 4**: CLI commands, redaction modes, caching, parallel EXPLAIN, DX requirements | Met | `make test-integration`; `docs/quickstart-verification.md` |
| **10-minute rule**: fresh clone to first report from the README | Met (about 94 s, on the development machine) | `docs/quickstart-verification.md` |
| **Phase 5**: reporters, Action, GitLab template, image, examples, CI and release workflows | Partly met (see below) | files exist; not executed in CI |
| **Phase 6**: docs, LEARNING, licences, templates | Met | `docs/`, `LICENSE`, `ext/plantwin/LICENSE` |
| **Quality**: unit, golden, pg_regress, integration all green | Met | `make check`, `make test-ext`, `make test-integration` |
| **Quality**: `go vet` and lint clean | Partly met | `go vet` and `gofmt` clean; no linter beyond vet was run (see blockers) |
| **Security review** | Met for the items checked; findings fixed | see the list below |
| **CI green on a fresh clone** | **Not met** | the workflows were never run; pushing is not allowed for this work |
| **Multi-architecture image** | **Not met** | only `linux/amd64` was built locally; buildx was not run |
| **Signed release** | **Not met** | cosign is not installed here; the release workflow was not run |

## Why the tag is withheld

1. CI has not run. The gate says "CI green on a fresh clone". That needs a push to GitHub, and pushing is not allowed
   for this work, so the gate cannot be passed from here.
2. The image is built for `linux/amd64` only. The multi-architecture build is in `release.yml` and was not run.
3. The release pipeline (checksums, keyless signing, GHCR) was written but has not run.
4. Recall and false-positive numbers come from a corpus whose rules were developed on part of it. A held-out result
   is still missing.

Tagging can happen after these are resolved. Until then the tree is at a development state, and nothing has been
published.

## Security review: what was checked, and what was fixed

Checked and fixed:

- **Export privilege.** The documented role was wrong: `pg_read_all_stats` does not cover `pg_statistic`. The export
  now reads `pg_statistic` directly, the precheck tests the privilege it needs, and the documentation names the exact
  GRANT. An integration test runs the export as that role and confirms it cannot read rows.
- **Git refs.** A ref beginning with `-` would have been read as a git option. It is now refused.
- **Redaction.** An integration test confirms that a common value does not appear in a `hash` or `drop-mcv` bundle, and
  that it does appear in `keep` (so the test cannot pass vacuously). A keyless HMAC was a risk during development;
  the export now passes a per-export random salt to every redaction call.
- **Extension guard.** Tested: loading without the marker fails; the marker is checked before any setting is defined;
  `shared_preload_libraries` stops the server from starting.
- **Signing.** Tested: a changed bundle fails verification; the private key is written with mode 0600.
- **Workflows.** Read-only token permissions; `pull_request` only; no secrets in the CI file; the Action verifies the
  checksum before running the binary.
- **Secrets in output.** The CLI does not print the export DSN. The demo's signing key is ephemeral and deleted with the
  temporary directory unless `--keep` is given.

Open, and documented:

- The export role can read `pg_statistic`, which holds common values (`docs/privacy-and-redaction.md`).
- The `command` migration tool runs a command from `plantwin.yaml` on the host (`docs/limitations.md`).
- The CI marker is a convention. A superuser can set it.

## What is complete

- C extension: chained hook, CI guard, default-off switch, B-tree height estimate, pg_regress tests (2 tests).
- CLI: `init`, `export` (four redaction modes), `keygen`, `check`, `explain`, `accept`, `doctor`, `demo`, `version`.
- Reports: Markdown with side-by-side plans and "estimated" labels, JSON with `schema_version`, SARIF 2.1.0.
- Exit codes: 0 passed, 1 findings, 2 usage or config, 3 Docker or Git unavailable.
- Benchmarks, each with a script and raw output: TPC-H and JOB fidelity; a migration corpus with measured ground truth;
  redaction fidelity.
- Documentation: README, architecture, privacy and redaction, limitations, rules, security, contributing, code of
  conduct, changelog, ADRs 0001–0012, LEARNING, quickstart verification.

## Known gaps

- The corpus is not held out (see above). A second corpus with an independent query workload is needed.
- `plantwin doctor` has not been exercised against a real replica. Its checks run against the demo and against missing
  files only.
- The Django example has not been run; Django is not installed in this environment.
- Partition behaviour in the extension is not covered by a regression test.
- The `hash` redaction default has a measured cost on categorical filters (`docs/privacy-and-redaction.md`). Whether to
  keep it as the default is an open decision for the maintainers.
