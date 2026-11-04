# ADR-0004: Extended statistics: detect and warn, do not restore

**Context.** PG18 does not preserve extended statistics (`CREATE STATISTICS`) in the stats export. The PG18
release notes state "Extended statistics are not preserved" under the pg_upgrade item
(https://www.postgresql.org/docs/18/release-18.html, section E.6.3.7.2). We checked the
`pg_dump --statistics-only` path on TPC-H: the dump has no extended-statistics restore call.

**Decision.** The export lists every `CREATE STATISTICS` object and writes a warning into the bundle manifest.
`check` prints the warning. Re-creating the objects would need `ANALYZE` on synthetic data, which the
twin must never run (ADR-0003). Multi-column estimates are therefore a known difference.

**Consequences.** Plans involving correlated multi-column predicates may differ. This is listed in the
limitations and reported with each run.
