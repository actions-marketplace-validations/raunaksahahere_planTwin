# ADR-0013: CI, release and published-artifact verification

**Status**: accepted, 2026-11-05.

**Context.** The first CI run on `main` failed in two jobs for two root causes (see [ci-triage](../ci-triage.md)). The
release and examples workflows had never run. The project needs a pipeline that (a) catches the failures that a dev
machine hides, (b) publishes artifacts that can be verified from outside the repository, and (c) does not weaken checks
to get green.

**Decisions.**

1. **Go runs in pinned containers with a host cache.** Module and build caches are host directories owned by the user
   that runs the container. A named Docker volume is created root-owned, and a non-root CI user cannot write to it. This
   is the cause of CI failure 1.

2. **Image tags are verified before use.** A matrix entry names a tag only after it appears in the registry's tag list
   (Docker Hub, checked with its API). `postgres:18.5` did not exist and failed the job, so the matrix now uses `18.4`.

3. **Every third-party action is pinned to a commit SHA**, with the version in a comment. Tags can move. The SHA is the
   value resolved from the tag at the time of the change.

4. **Least-privilege tokens.** Each workflow sets `contents: read` at the top. Jobs that publish raise only the scopes
   they need (`contents: write` to create the release; `packages: write` to push the image; `id-token: write` only for
   keyless signing). `persist-credentials: false` on every checkout. No `pull_request_target`.

5. **Keyless signing, no stored keys.** The checksum file and the image digest are signed with Sigstore cosign using the
   job's OIDC identity. The signature records the workflow path and the tag, so verification can name the exact release
   workflow and tag as the identity.

6. **A `quickstart` job on a clean runner.** It runs the README's commands on a fresh checkout and times each one. It
   requires the report to contain the planted `lost_index` finding, so a report with no findings fails the job.

7. **Examples test the published artifacts at the tag.** The `examples` workflow (manual, `workflow_dispatch`) pulls the
   published image and verifies its signature; downloads the release binary and verifies its checksum; exports with the
   documented least-privilege role; and runs the published GitHub Action at the tag against a two-commit repository that
   contains a planted regression, for the SQL and the Django examples.

8. **The expected-failure assertion.** The planted migration must make the Action fail. The Action step is marked
   `continue-on-error: true` so the job can go on, and the next step asserts that the step's outcome is `failure` *and*
   that the report names `lost_index`. A passing check, or a failure for another reason, turns the job red. This is an
   assertion of the expected result, not a way to hide a failure: nothing else is allowed to be red without the job being
   red.

9. **Lint is part of the check.** `staticcheck` is pinned (`2025.1.1`) and runs in `make check`, in the same pinned
   container as the rest of the Go tooling. `go vet` also covers the integration-tagged files, which the default build
   does not compile.

10. **Release notes come from the CHANGELOG.** The release job extracts the section for the tag and fails if there is
    none. The CHANGELOG is therefore part of the release gate.

11. **No credentials in code or fixtures.** Twin containers get a random password per container, and the containers no
    longer use `trust` authentication. The least-privilege test role's password is generated per run. The Django
    example reads `SECRET_KEY` from the environment and generates one otherwise.

**Deliberate exception to pinning.** The `examples` workflow references this project's own Action at its tag
(`raunaksahahere/planTwin@v0.1.0`), not at a commit SHA. That is the reference consumers use, and testing it is the point
of the job. The tag is the only mutable reference in the workflows and is checked by hand at release time. Container
images in the Makefile and workflows are referenced by tag, not digest; the release pins the published image by digest
when it signs it.

**Consequences.**

- The `examples` workflow names the Action at a literal tag (`raunaksahahere/planTwin@v0.1.0`), because GitHub does not
  allow an expression in `uses:`. Each release bumps that literal and the default of the `tag` input together.
- The examples job needs the GHCR package to be pullable with the job's token. If the package is private, the examples
  job cannot pull it. Making the package public is a web-UI step (see [BLOCKERS](../BLOCKERS.md)).
- Tags are never moved or deleted. A failing release is fixed forward with the next patch version.
- A signature is only as useful as its identity check. Verification commands in [STATUS](../STATUS.md) and the release
  notes name the identity explicitly.

**Alternatives considered.**

- *Build the image in the examples job instead of pulling it.* Rejected: it would test a build, not the published image.
- *Use `workflow_run` to trigger the examples after the release.* Rejected for now: a manual trigger is easier to read and
  to rerun, and the release gate is already the tag.
- *Mark the expected-failure job `continue-on-error` for the whole job.* Rejected: it would hide any failure. The
  assertion step is narrower.
