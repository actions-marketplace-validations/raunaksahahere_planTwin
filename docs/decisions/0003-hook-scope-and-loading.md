# ADR-0003: Hook scope and per-session loading

**Context.** The design note listed four open questions. This ADR records the answers for the hook.

**Decision.**
1. Load per session with `LOAD`, never via `shared_preload_libraries`. Preloading is also refused by ADR-0001.
2. The hook is called for each relation, including partitions. Each partition is sized from its own `pg_class`
   row. Partitioned parents (no storage, `relpages = 0`) are skipped by the `relpages > 0` test.
3. Overridden fields: `rel->pages`, `rel->tuples`, `rel->allvisfrac`; for each index `pages`,
   `tuples` (mirroring `plancat.c`: non-partial indexes take the table's tuples, partial indexes take
   their own capped at the table's) and, for B-tree, `tree_height` (ADR-0002).
4. Overrides apply only when the catalog holds imported stats (`relpages > 0` and `reltuples >= 0`).
   Otherwise the planner's own estimate is kept.
5. The previous hook in the chain runs first, so this extension's values win.

**Why.** Overriding only what the spike showed was needed keeps the change small and the reasoning
checkable. `tree_height` was needed: with the height at 0 on the empty clone, index-scan costs
dropped (see `docs/why-a-hook.md`).

**Consequences.**
- The twin must never be `ANALYZE`d or vacuumed. Either would overwrite imported stats. `build_clone.sh`
  turns off autovacuum on the twin's tables. The CLI must do the same and must not run `ANALYZE`.
- Partition behaviour is not yet covered by a regression test. It is a documented limitation.
