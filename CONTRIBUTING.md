# Contributing

Thanks for looking at PlanTwin. The most useful contributions right now are real migrations that PlanTwin gets wrong
(false positives or misses), plan shapes where the twin disagrees with production, and reports from `plantwin doctor`
on setups we have not seen.

## Ground rules

- **Measure before you claim.** Every number in the docs must come from a script in `bench/` with its raw output saved.
  If a result is bad, publish it with the explanation. Do not tune a benchmark to make a number better.
- **Record decisions.** A non-trivial design choice gets an ADR in `docs/decisions/`.
- **Keep the extension guarded.** Nothing may make the C extension load without `plantwin.ci_marker`, or run outside CI.
- **No production data in the repository.** Fixtures are synthetic or public (TPC-H, JOB). Do not commit bundles from real systems.

## Development setup

You need Docker and make. Go runs in a pinned container, so you do not need Go installed (ADR-0006).

```bash
make image           # the twin image (postgres:18 + plantwin.so)
make check           # gofmt, go vet, unit and golden tests
make test-ext        # pg_regress tests for the extension (Dockerfile test stage)
make test-integration  # real Docker: demo, redaction, least-privilege export role
make build && ./bin/plantwin demo
```

## Before you open a pull request

1. `make check` passes.
2. If you touched `ext/plantwin`, `make test-ext` passes.
3. If you touched export, twin or check, `make test-integration` passes.
4. If you changed a rule, update `docs/rules.md`, the golden files (`go test ./internal/rules -run Golden -update`,
   then review the diff), and explain the change in the pull request.
5. If you changed a measurement, re-run the benchmark and commit the regenerated results.
6. If the change is visible to users (a command, flag, configuration key, report field, default, or message), add a line
   under `## [Unreleased]` in `CHANGELOG.md`. A release is refused without a CHANGELOG section for its tag.
7. From 1.0.0, do not break anything listed as stable in [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) within 1.x. A
   change that needs it goes to the next major version.

## Commit messages

Conventional commits: `feat(scope): …`, `fix(scope): …`, `bench(scope): …`, `docs: …`. Describe the why in the body.

## Code style

- `gofmt`, `go vet`. Keep dependencies to the two listed in ADR-0009 unless an ADR says otherwise.
- Errors for people go through `internal/errs`: what happened, why, and what to do next.
- C: check memory contexts and hook chaining; use `elog` levels deliberately; keep the hook surface small.

## Licence

By contributing you agree that your contribution is licensed under the licence of the component you change
(Apache-2.0 for the CLI and Action; PostgreSQL License for `ext/plantwin`).
