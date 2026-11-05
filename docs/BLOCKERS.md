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
    and runs the example against the tagged release; the result is recorded in `docs/STATUS.md` once the job has run.

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
