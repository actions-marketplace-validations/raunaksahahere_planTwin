# Security policy

## Supported versions

Only the latest release receives security fixes. Before v0.1.0 is tagged, only the `main` branch is maintained.

## Reporting a vulnerability

Do not open a public issue for a security problem. Report it privately to the maintainers through the repository's
security advisory form (GitHub: *Security* → *Report a vulnerability*), or by email to the address in the repository
profile. Include the version, the steps, and the output with any secrets removed. We aim to acknowledge within five
working days.

## What PlanTwin is designed to protect

- **Production data.** The export reads catalogs and `pg_statistic` only. The role documented in
  `docs/privacy-and-redaction.md` cannot read table rows (tested against PostgreSQL 18).
- **Values that leave the replica.** Redaction (`hash` by default) is applied before the bundle is written. The salt
  is never stored. A common value in a `hash` or `drop-mcv` bundle is not present in the file (tested).
- **Bundle integrity.** Bundles are signed with ed25519. `check` and `doctor` refuse an unsigned or modified bundle.
- **The planner extension in production.** The extension refuses to load without `plantwin.ci_marker`. A
  `shared_preload_libraries` entry stops the server from starting (tested). It is not preloaded in the image.
- **Local twins.** Containers bind to `127.0.0.1` on a random port, and use a fixed password that exists only for the
  life of the container.
- **Secrets in logs.** The CLI does not print the export DSN. Errors show the underlying cause; `--verbose` adds
  detail from the database driver, which does not include passwords.

## Known limits (see docs/limitations.md)

- The `command` migration tool runs a command from `plantwin.yaml` on the machine that runs `check`. A pull request can
  change that file. Do not run `check` with secrets on untrusted refs.
- The CI marker is a convention. A superuser can set it. It protects against accidents, not against deliberate misuse.
- The export role can read `pg_statistic`, which holds common values. Restrict who holds it.
- The per-export salt makes surrogates hard to match against a dictionary, but `hash` is not an encryption scheme and
  its surrogates keep the frequencies of the original values.

## CI guidance

- Trigger checks with `pull_request`. Never use `pull_request_target` to run pull-request code.
- Give workflows the minimum token permissions (`permissions: contents: read`).
- Do not expose the export DSN to forks. The bundle and public key are enough for checks.
- Pin actions and images by digest in production pipelines.
