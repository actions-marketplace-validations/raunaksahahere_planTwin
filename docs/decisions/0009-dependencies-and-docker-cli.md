# ADR-0009: Dependencies, and driving Docker through the CLI

**Context.** The blueprint suggested testcontainers-go for integration tests and a Docker SDK for the CLI. Both add a
large dependency tree. Each dependency is code that will run on users' machines or in CI.

**Decision.**
- Two direct Go dependencies, both pinned: `github.com/jackc/pgx/v5 v5.7.2` (PostgreSQL protocol, for export,
  EXPLAIN and stats import) and `gopkg.in/yaml.v3 v3.0.1` (config). Everything else is the standard library.
- Docker is driven with the `docker` command-line tool through `os/exec`. The CLI does not link the Docker SDK.
- Integration tests use the real Docker CLI and the real image. They are not testcontainers tests. The integration
  test binaries are built in the pinned golang container and run on the host (`make test-integration`).
- Go toolchain: `golang:1.24` in a container (ADR-0006). `go mod tidy` does not run under Go 1.24 because a
  transitive test dependency declares Go 1.25; `go.mod` is edited by hand to list direct requirements.

**Why.** Fewer dependencies means fewer things to audit and fewer ways to break. The docker CLI is already required
on every machine that runs PlanTwin. A static binary (CGO disabled) needs only the standard library plus pgx and yaml.

**Consequences.** Docker errors are reported as text from the CLI, so the CLI parses only the exit status and the
first line of output. A testcontainers suite would be more idiomatic and is a possible later change.
