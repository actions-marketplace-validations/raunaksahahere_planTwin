# ADR-0012: Caching by schema hash stores the schema, not the container

**Context.** The blueprint asks for "container caching keyed by schema hash". The official `postgres` image declares
its data directory as a `VOLUME`. `docker commit` does not capture volume contents, so a committed image would
restart with an empty database. Caching the container image therefore does not work with this image.

**Decision.** Cache the **schema**. After migrations are applied to a twin, `pg_dump --schema-only` writes
`.plantwin/cache/<hash>.sql`. The hash is SHA-256 over the image name and every migration file in order (name and
content). A later run with the same hash replays the dump instead of running the migrations. Psql meta-commands in
the dump (`\restrict` and similar) are removed first, so pgx can run it.

- A cache entry that fails to replay is ignored and the migrations run again.
- The cache holds schema only: no rows and no statistics. Statistics are always imported fresh from the bundle.
- The base and head twins are cached separately, because their migrations differ.

**Verified.** The demo integration test checks that two cache entries exist after one run (base and head).

**Consequences.** For plain SQL migrations the saving is small (the migrations are fast). It matters more for slow
command-based tools (Django, Rails). The blueprint wording "container caching" is replaced by this, and the change is
recorded here. Container start-up still happens on every run.
