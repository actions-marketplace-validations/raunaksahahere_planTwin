# Compatibility promise (from 1.0.0)

This page says what PlanTwin 1.x keeps stable, what it does not promise, and how versions change. It is the reference
for pinning a version in CI. The change history is in [CHANGELOG.md](../CHANGELOG.md).

## Versions

- PlanTwin uses semantic versioning. Releases are tags `vMAJOR.MINOR.PATCH`.
- A tag is never moved or deleted. A bad release is fixed by the next patch version.
- Every release has a section in `CHANGELOG.md`. The release is refused without one.
- Security fixes go to the latest 1.x minor release.

## What 1.x keeps stable

| Surface | Stable within 1.x | Changing it needs 2.0 |
|---|---|---|
| CLI commands: `init`, `export`, `keygen`, `check`, `explain`, `accept`, `doctor`, `demo`, `version` | Each command exists and keeps its meaning. New flags can be added. | Removing or renaming a command or a flag. |
| Exit codes: `0` passed, `1` findings failed the check, `2` usage or configuration error, `3` Docker or Git unavailable | The meaning of each code. | Changing a code's meaning. |
| `plantwin.yaml` keys: `version`, `image`, `migrations`, `queries`, `thresholds`, `redaction`, `fail_on`, `bundle` | Every key in 1.0.0 keeps its meaning. New keys can be added. A file that loads in 1.0.0 loads in every 1.x. | Removing a key or changing what it means. |
| Unknown keys in `plantwin.yaml` | Rejected with an error that names the line. This is what keeps a misspelled key from being ignored. | Accepting unknown keys again. |
| Redaction modes: `keep`, `hash`, `drop-mcv`, `drop-histogram` | The four names and what they export. | Removing a mode. New modes can be added. |
| JSON report | `schema_version` is `1`. Adding a field is allowed and does not change the version. | Removing or renaming a field, or changing its type. The version then becomes `2`. |
| SARIF output | SARIF 2.1.0. | Changing the SARIF version. |
| Statistics bundle | JSON `schema_version` `1`, with a detached Ed25519 signature in `<bundle>.sig`. A bundle exported by 1.0.0 is readable by every 1.x. | A bundle format that 1.x cannot read. |
| Release files | Names `plantwin_linux_amd64.tar.gz`, `plantwin_linux_arm64.tar.gz`, the raw binaries, and `checksums.txt`. The GitHub Action relies on these names. | Renaming them. |
| Container image | `ghcr.io/<owner>/plantwin-postgres:18`, multi-architecture, keyless-signed by digest. | Dropping a tag or an architecture. |
| Action inputs | The inputs named in `action.yml`. | Removing or renaming an input. |
| C extension GUCs | `plantwin.ci_marker` (refuses to load without the CI marker) and `plantwin.enabled` (default off). | Removing or renaming a GUC. |

## What 1.x does not promise

- **PostgreSQL versions other than 18.** Only 18 is tested. See [limitations](limitations.md).
- **Accuracy on your workload.** The figures in [bench/results](../bench/results) come from a synthetic corpus and
  public benchmark schemas. They are not a guarantee for your queries or data.
- **Scale.** No maximum number of queries, tables or database size is promised. The check runs one twin per revision and
  its time grows with the work it does. Measured timings for your workload are the only reliable answer.
- **Rule outcomes and default thresholds.** A minor release can change a rule or a default threshold, because that is
  how accuracy improves. Such a change is listed under `Changed` in the CHANGELOG and never ships in a patch release.
- **Wording.** The text of Markdown reports, terminal messages and rule explanations can change in any release. The
  fields of the JSON report do not.
- **Packages under `internal/`.** They are not an API.
- **Timings and performance.** Not part of the promise.

## Deprecation

A deprecated key, flag or field keeps working until 2.0.0. It is named in the CHANGELOG under `Deprecated` in the release
that deprecates it.

## How to pin

- For CI, pin the Action by tag, for example `raunaksahahere/planTwin@v1.0.0`, and set `version:` to the same tag.
- For the image, pin by digest. The release notes give the digest and the signing identity to verify it with.
- A consumer that reads the JSON report should check `schema_version` and refuse a value it does not know.
