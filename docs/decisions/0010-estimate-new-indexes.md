# ADR-0010: Estimating statistics for indexes the PR creates

**Context.** A migration can add an index that production has never analysed. The twin has no imported size for it,
so the planner would see an empty index. The blueprint asks for estimated sizes and labelled findings.

**Decision.** `internal/twin/estimate_new_indexes.sql` (embedded in the CLI and used by the corpus benchmark, so the two
cannot drift) sizes every B-tree index whose imported `relpages` is 0:

    pages  = ceil(tuples_est * (key_width + 12) / (8192 * 0.90))
    tuples = table reltuples, times 0.10 for a partial index

- `key_width` is the sum of `pg_statistic.stawidth` for the key columns (8 bytes for an expression or a missing stat).
- 12 bytes is index-tuple overhead. 90% is the default B-tree leaf fill (`BTREE_DEFAULT_FILLFACTOR`, PG18 source).
- The 0.10 factor for partial indexes is a fixed guess, not a measurement. The estimate is labelled
  *estimated* in every finding that depends on it.
- The estimator writes notices `estimated: <index>`. The CLI reads them and uses the catalog signature comparison
  (`TableSignatures`) to label the tables the migration changed.

**Limit.** The partial-index factor is a guess. A partial index on a rare value is over-estimated. Findings labelled
*estimated* should be read with that in mind. The PR comment says so.

**Consequences.** The estimator runs only on the twin, never on production (it writes stats through
`pg_restore_relation_stats`, which needs superuser on the twin).
