# ADR-0008: Rules, thresholds and the selectivity cut

**Context.** The first corpus run exposed two defects in the rules, both measured, not theorised:
1. A parallel plan (`Gather → Seq Scan`) hid the sequential scan from the lost-index rule, which only looked at
   the top of a replaced subtree. Two real 150–340× regressions were missed. Fixed: rules search the whole replaced subtree.
2. `lost_index` fired on any index loss. On `customers` (100k rows), a 25% index scan became a faster sequential scan,
   and the tool reported an error. Fixed: a lost index counts only when it was selective.

**Decision.**
- Every applicable rule runs for every change (no `switch` short-circuit). `shape_changed` (info) runs only when no
  specific rule applied. Found by a unit test where a sort rule was hidden by the sequential-scan rule.
- **Selectivity cut: 10%.** A scan that returns at least 10% of its table is not "selective". Losing such an index
  is not a regression, and a sequential scan that returns it is not `seq_scan_large`. The value follows the usual
  planner rule of thumb that index scans lose to sequential scans well before most of a table is returned. It was
  chosen once, before the widened corpus, and is not tuned per case.
- Table sizes come from the stats bundle (`reltuples`). If a table has no imported size, the rules fall back to the
  returned row count, which is weaker.
- Thresholds (defaults): `large_rows` 100k (lost index becomes an error), `seq_scan_rows` 10k, `join_rows` 10k,
  `work_mem` 4 MB for Sort/Hash, `cost_ratio` 2.0 (info only), `min_cost` 1000.
- Cost is the secondary signal: `cost_ratio` is info, never a failure on its own.

**Consequences.**
- Corpus result after these fixes (18 cases, 2× bar): recall 5/5, false positives 0/13. The rules were developed on the
  first 12 cases, so this is **not** a held-out result (see `bench/results/corpus/summary.md`).
- The earlier run showed a false positive (c04: a selective index dropped, measured 1.17×). It came from a cold-cache
  measurement. With the warm-up in place (`bench/corpus/measure_truth.py`), the same case measures 2× on one query,
  and it is a true positive. The sensitivity table in `summary.md` shows how the verdict moves with the bar.
