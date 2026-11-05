# ADR-0014: What 1.0.0 means, and the gates it must pass

**Status**: accepted, 2026-11-06. v1.0.0 was tagged on `7c0de91` after the gates below passed; the results are in
[STATUS](../STATUS.md).

**Context.** Through 0.1.x PlanTwin said it was pre-1.0 and could change its flags, keys and report at any release. Teams
that run it in CI need to know what will break and when. Two behaviours in 0.1.x were also unsafe for production use:
a misspelled key in `plantwin.yaml` was silently ignored, so the check ran with defaults; and the statistics export
against a production replica had no server-side guarantee of read-only access and no timeouts.

**Decisions.**

1. **The compatibility promise is written down.** [COMPATIBILITY.md](../COMPATIBILITY.md) lists what 1.x keeps stable
   (commands, exit codes, configuration keys, the JSON report's `schema_version` 1, the bundle format, release file
   names, the image tag) and what it does not promise (other PostgreSQL versions, accuracy on a given workload, scale,
   wording, rule thresholds between minor releases).

2. **Unknown configuration keys are an error.** The loader uses a strict decoder. A misspelled key fails the run and
   names the line. This is a behaviour change for any file with a typo. The alternative, ignoring unknown keys, means a
   CI check can pass while measuring nothing the author intended. All shipped examples and the `init` template use only
   known keys, and tests cover misspelled sections and an empty file.

3. **The export is read-only on the server.** The session sets `default_transaction_read_only`, a five-minute
   `statement_timeout`, a five-second `lock_timeout`, and `application_name=plantwin-export`. The export checks that
   read-only mode is in effect and refuses to run if it is not. This is the case when a connection pooler drops startup
   parameters. The reason is that the export runs on production, and the documented grants are only part of the
   guarantee. The statement timeout and the lock timeout are defaults chosen for a catalog read, and they are not tuned
   against any measured production replica.

4. **A parse error for the DSN does not echo the DSN.** A URL parse error can contain the password. The message says the
   DSN cannot be parsed and gives the expected form.

5. **`plantwin check` has a default timeout of 30 minutes** (`--timeout`). A check that does not finish stops and says so,
   rather than holding a CI job until the platform kills it.

6. **Platforms.** Release binaries are built for linux/amd64, linux/arm64, darwin/amd64 and darwin/arm64. The GitHub
   Action installs the Linux binary that matches the runner, and the image is built for linux/amd64 and linux/arm64. The
   Action does not run on macOS.

7. **Claims that 1.0.0 does not make.** No scale figure, no accuracy figure for any workload other than the published
   benchmarks, no support for PostgreSQL versions other than 18, and no measurement on a streaming standby beyond what is
   recorded in [STATUS](../STATUS.md). A claim becomes a promise only when a recorded run supports it.

8. **A regression test joins the suite only with a reviewed expected output.** `ext/plantwin/sql/plantwin_partition.sql`
   sizes two partitions through the hook. Its expected output was generated on PostgreSQL 18 with the same `pg_regress`
   harness as CI, then checked by hand: the parent has no scan of its own, and each child is sized from its imported
   statistics. A test added to `REGRESS` without an `expected/` file fails the extension job.

**Gates for tagging v1.0.0.** All of these must hold, on the commit being tagged:

- `make check` (format, `go vet` with and without `-tags integration`, `staticcheck`, unit and golden tests) passes.
- `make test-ext` passes on PostgreSQL 18.6 and 18.4.
- `make test-integration` passes. The strict-config tests are unit tests and run in `make check`. The read-only refusal
  in the export has no test yet; adding one against a live server is open work, recorded in [STATUS](../STATUS.md).
- CI is green on `main`, including the quickstart job on both `ubuntu-24.04` and `ubuntu-24.04-arm`.
- The release workflow publishes the tag, and the release file names match what the Action downloads.
- The examples workflow passes against `v1.0.0` on both architectures, for the SQL and the Django examples.
- The release files verify from outside the build: `cosign verify-blob` on `checksums.txt`, `sha256sum -c`, and a
  digest-pinned pull with `cosign verify` on the image.
- The CHANGELOG section for 1.0.0 matches what the commit does, and [STATUS](../STATUS.md) records each gate with its
  run.

**Consequences.**

- Users of 0.1.x configuration files with typos will see a clear error on upgrade. The CHANGELOG says so first.
- A later change to a default threshold can change check results. It is a minor release with a CHANGELOG entry, never a
  patch release.
- Any gate that cannot be run is recorded as not run. The release is not tagged on an assumption.

**Alternatives considered.**

- *Stay at 0.x until more production use.* Rejected: teams need a compatibility promise to adopt a tool, and the
  promise can be narrow and honest about what is not claimed.
- *Ignore unknown keys and warn.* Rejected: the check's output would still come from a configuration the author did not
  mean. A warning in a CI log is easy to miss.
- *Promise scale.* Rejected: there is no measurement that supports a number. The benchmarks in `bench/` measure
  fidelity on a small corpus.
