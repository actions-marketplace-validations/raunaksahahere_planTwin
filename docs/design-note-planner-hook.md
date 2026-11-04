# Design note: the `plantwin` planner hook (DRAFT, for review, no code yet)

Status: proposal. Implementation waits for review. Step 0 is partial: the gap is shown on TPC-H, but the hook's effect is not yet shown.

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

## Open questions for review
1. Is `shared_preload_libraries` ever needed, or is per-session `LOAD` enough? (Proposed: per-session.)
2. Inheritance and partitions: the hook receives child relations too. Does each child need
   its own imported stats? (To verify against `plancat.c` before deciding.)
3. Which fields beyond `pages`/`tuples`/`allvisfrac` affect the plans in the 22 TPC-H queries?
   Decide after the spike.
4. Extended statistics are not preserved by PG18 export. Status: the PG18 release notes state
   "Extended statistics are not preserved" under the pg_upgrade item
   (https://www.postgresql.org/docs/18/release-18.html, section E.6.3.7.2). A test on TPC-H
   confirmed that `pg_dump --statistics-only` also carries no extended-statistics restore
   calls. See `docs/why-a-hook.md`.

## Explicit non-goals
- No edits to relation files (rejected in the blueprint: unsafe, version-brittle).
- No effect on the production server.
