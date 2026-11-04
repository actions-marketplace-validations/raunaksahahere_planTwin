# ADR-0002: Estimating B-tree height from imported sizes

**Context.** `get_relation_info()` reads a B-tree's `tree_height` from the physical metapage
(`_bt_getrootheight`). An empty CI index has no root, so the height is 0. `btcostestimate()` then charges
no descent cost, so index scans look cheaper than in production. On the TPC-H clone this showed as
index-scan nodes with cost 0.00 (`docs/why-a-hook.md`). The height is not stored in `pg_class`, so the
hook has to estimate it.

**Decision.** Estimate the level from `pg_class` pages and per-column widths:

    leaves   = pages - 1                        (metapage)
    itemsize = MAXALIGN(sizeof(IndexTupleData) + key width) + sizeof(ItemIdData)
    fanout   = floor((BLCKSZ - SizeOfPageHeaderData - MAXALIGN(sizeof(BTPageOpaqueData))) * 0.70 / itemsize)
    level    = ceil(log(leaves) / log(fanout))   for leaves > 1, else 0

- The 0.70 is `BTREE_NONLEAF_FILLFACTOR` (`src/include/access/nbtree.h`, line 202, PG18). Internal pages
  are built to 70% full (`nbtsort.c`, `btps_full`).
- Key width is the sum of `pg_statistic.stawidth` for the key columns (8 bytes for an expression
  column or a missing stat), so the imported stats supply it.

**Evidence.** Checked against `pageinspect.bt_metap().level` on every B-tree index:
- TPC-H SF1: 8/8 correct.
- JOB: 42/44 correct. Misses: `complete_cast_pkey` (true 1, estimated 2) and `cast_info_pkey` (true 2, estimated 3).
  Both are close to a fanout boundary. The true level depends on how the index was built (sorted bulk build
  versus insert-order splits), which the catalog does not record. Neither index is used by the mismatched JOB queries.

**Disclosure (honest order of events).** The first version, `fanout = tuples / leaves`, got 40/44 on JOB. I then
switched to the key-width model (42/44) after seeing JOB failures. The constants come from the PG18 source,
not from a fit, and TPC-H (8/8) was correct under both versions. Still, the model choice was informed by
the JOB results, and the JOB numbers are not a held-out test of it. A held-out check would need a third
schema, which is not in the benchmark yet.

**Consequences.** The hook can be off by one level for some indexes. This is in the limitations and in
`bench/results/fidelity.md`. Tuning the 0.70 to fit JOB was rejected: it would be overfitting.
