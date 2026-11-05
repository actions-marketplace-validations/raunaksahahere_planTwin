# Blockers and what was done about them

Each entry says what was blocked, what was tried, and what was done instead. Blockers that changed the work are listed
first.

## Blocked, and worked around

1. **TPC-H generator (tpch-kit) could not be built.** The sandbox refused to build `dbgen` from a downloaded repository,
   because that runs downloaded code. **Done instead:** DuckDB's official `tpch` extension generated SF1 (`bench/spike/gen_tpch_sf1.py`),
   and DuckDB was installed from PyPI at a pinned version. Row counts match SF1.

2. **Go was not installed on the host.** Installing a toolchain runs downloaded code outside a container. **Done instead:**
   every Go command runs in the pinned `golang:1.24` image as the current user (ADR-0006, `Makefile`).

3. **`go mod tidy` fails under Go 1.24.** A transitive test dependency declares Go 1.25. **Done instead:** `go.mod` is
   edited by hand for direct requirements; the build and tests do not need tidy (ADR-0009).

4. **`pg_read_all_stats` does not give the export role what the blueprint assumed.** Checked on PostgreSQL 18: it does
   not grant `pg_statistic`, and the `pg_stats` view filters by table `SELECT`. **Done instead:** the export reads
   `pg_statistic` directly and the documented role is granted `SELECT` on `pg_statistic` only. Tested with a real role.

5. **The JOB data URL on the original host returned 404.** **Done instead:** the official CWI mirror
   (`https://event.cwi.nl/da/job/imdb.tgz`) was used. The archive was listed before extraction (21 CSVs and one SQL file).

6. **The Bash safety check blocked some commands with no verdict** (transient, and once a heredoc containing `$()`).
   **Done instead:** files were written with the file tools, and commands were re-run once the check responded.

## Not done, and why

7. **CI had not run in the first development pass.** Now resolved: CI ran on pull requests and `main`, its two failures were
   fixed, and `main` is green ([docs/ci-triage.md](ci-triage.md)).

8. **Multi-architecture image.** Only `linux/amd64` was built locally. The buildx multi-arch build is in `release.yml`;
   it runs with the tagged release and is verified after the tag ([docs/STATUS.md](STATUS.md)).

9. **Signed release.** cosign is not installed on the development machine. Keyless signing runs in `release.yml` on the
   runner, and it is verified after the tag (`docs/STATUS.md`).

10. **Linters.** Now resolved: `actionlint 1.7.7` runs in its pinned container over the workflows, and `staticcheck 2025.1.1`
    runs in `make check`. Both are clean. Other linters (for example golangci-lint) have not been run.

11. **Django example.** Django is not installed on the development machine. The `examples` workflow installs it on the runner
    and runs the example against the tagged release. It passes at `v0.1.2` ([STATUS](STATUS.md)).

12. **Real replica for `plantwin export` and `doctor`.** Export is tested against a container that plays the replica,
    including the least-privilege role. A real replica's version, extensions and size were not available.

13. **Held-out corpus.** The rules were developed on the first 12 corpus cases. A second, independent workload was not
    built. The numbers are labelled accordingly.

14. **Redaction default.** The `hash` default has a measured effect on categorical filters
    (`docs/privacy-and-redaction.md`). Choosing a different default is a product decision for the maintainers, so the
    default was kept and the trade-off documented.

## Environment notes

- Docker 29.9.2 on Linux, 12 cores, 7 GB RAM. Some runs (the corpus, JOB) need several minutes and up to about 4 GB.
- Containers left running by earlier development steps were removed before the final checks (see `docs/STATUS.md`).

## GitHub Marketplace listing (needs the web interface)

**What is done.** The Action is public, Apache-2.0 licensed, has `action.yml` and `README.md` at the root, and has a
`branding` block (icon `database`, colour `blue`) at release `v0.1.3`. The examples pass against `v0.1.3`
([run 41981570894](https://github.com/raunaksahahere/planTwin/actions/runs/41981570894)).

**What cannot be done from the command line.** GitHub has no API for creating a Marketplace listing. The listing is
created by ticking a box on a release in the web interface. Doing it through automation would mean driving the browser
with the account's session, which I did not do.

**What could not be checked.** The account's two-factor setting reads as `null` through the API, and the email list
needs the `user` token scope, which this login does not have. Marketplace publishing requires two-factor authentication
and a verified email, so check both in Settings.

**Steps, in the GitHub web interface, signed in as `raunaksahahere`:**

1. **Check the account.** Settings → *Password and authentication*: two-factor authentication must be enabled.
   Settings → *Emails*: the primary email must be verified.
2. **Open the release.** Go to https://github.com/raunaksahahere/planTwin/releases, open `v0.1.3`, and click the pencil
   (*Edit release*).
3. **Tick the box.** Tick **Publish this Action to the GitHub Marketplace**. It appears because the repository has an
   `action.yml` with `branding`. If it does not appear, check that the release is not a draft and that `action.yml` is at
   the repository root at this tag.
4. **Choose categories.** Pick a primary category (for example *Continuous integration*), and optionally a secondary one.
5. **Check the preview.** The name, description, icon and colour come from `action.yml` (name *PlanTwin check*, icon
   `database`, colour `blue`). If GitHub reports that the name is taken, the fix is a new `name:` in `action.yml` and a
   new patch release. The tag itself is not moved.
6. **Accept the agreement.** If prompted, read and accept the GitHub Marketplace Developer Agreement.
7. **Publish.** Click **Update release**. The listing is live when the Marketplace page shows the Action.
8. **Confirm.** Search for *PlanTwin check* at https://github.com/marketplace?type=actions and open the listing. Then
   add the listing link to `README.md` and `docs/USAGE.md`.

The exact labels in GitHub's interface change from time to time. The steps describe what to do, and the
account requirements do not change.

