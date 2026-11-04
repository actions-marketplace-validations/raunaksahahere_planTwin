# Design note: the `plantwin` planner hook (DRAFT, for review, no code yet)

Status: **approved** (owner instruction, autonomous completion). Open questions resolved below. Implemented in `ext/plantwin/plantwin.c`.

## Problem
An empty CI database with imported statistics gets 0-row estimates, because the planner
sizes a table from its physical block count (`table_block_relation_estimate_size()`,
`src/backend/access/table/tableam.c:703-709`: 0 blocks means "clearly empty", so 0 tuples).
Reproduced on PG 18.6 with a 1M-row table (`bench/spike/synthetic_repro.sh`) and on TPC-H SF1 with 0/22 plan shapes matching (`docs/why-a-hook.md`).

## Proposed mechanism
Install a `get_relation_info_hook` (declared in `plancat.c`, called at `plancat.c:574`).
The hook runs after the planner's own estimate (`plancat.c:204-205`) and index sizing
(`plancat.c:476-485`), so it can overwrite the result. For each relation it would:

1. Look up imported `pg_class.relpages` / `reltuples` for the relation (and for each index).
2. Set `rel->pages`, `rel->tuples`, `rel->allvisfrac`, and the `IndexOptInfo` page/tuple fields.
3. Call the previous hook first (chaining), so other extensions keep working.

## Safety rules
- Off by default: `plantwin.enabled = off`.
- Refuses to activate unless a CI marker GUC is set (for example `plantwin.ci_marker`
  must equal a per-run token). Otherwise it logs a WARNING and does nothing.
- Writes nothing to disk and never changes data. It only affects planner estimates
  for the current backend.
- Loads only in CI sessions (explicit `LOAD`, not `shared_preload_libraries`).

## Resolved questions (decisions, see ADRs)
1. **Loading.** Per-session `LOAD` only, never `shared_preload_libraries`. The guard also
   makes a `shared_preload_libraries` entry fail server startup (verified: `pg_ctl start`
   exits with "plantwin refuses to load outside CI"). ADR-0001.
2. **Inheritance and partitions.** The hook runs once per relation, so each partition is
   sized from its own `pg_class` row. Partitioned parents have no storage (`relpages = 0`) and
   are skipped. The CLI must export stats for every partition, which `pg_dump` already does.
   Not yet covered by a test; listed as a limitation until one exists.
3. **Fields beyond pages/tuples/allvisfrac.** Index `tuples` (mirroring `plancat.c`) and B-tree
   `tree_height` are also overridden, because the spike showed `tree_height` (physical
   metapage) changes index-scan cost. Height uses the model in ADR-0002. Other fields are
   overridden only when the evidence requires it.
4. **Extended statistics.** Not restored. The CLI lists `CREATE STATISTICS` objects at export and
   warns that multi-column estimates will differ. It does not re-create them (that would
   require ANALYZE on synthetic data, which the twin must never run). ADR-0004. The PG18
   release note wording (pg_upgrade item) is cited in `docs/why-a-hook.md`, and the dump-path
   behaviour was checked on TPC-H.

## Explicit non-goals
- No edits to relation files (rejected in the blueprint: unsafe, version-brittle).
- No effect on the production server.
