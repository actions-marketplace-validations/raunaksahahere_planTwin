# Rules: what each finding means, and its threshold

Every finding has a rule name, a severity (`error`, `warning` or `info`), a plain-language *why*, and a one-line fix.
Only `error` and `warning` fail a check (`fail_on: warning` by default). Thresholds are in `plantwin.yaml` under
`thresholds:`. Defaults are shown below. The design rationale is in ADR-0007 and ADR-0008.

| Rule | Severity | Fires when | Default threshold | Why it matters | Fix |
|---|---|---|---|---|---|
| `lost_index` | error if the table has ≥ `large_rows`, otherwise warning | a scan that used a selective index is replaced by a sequential scan on the same table | `large_rows` 100,000; selective means the index scan returned < 10% of the table (`IndexSelectivity`) | queries lose the index and read every row | recreate the index, or accept it with `plantwin accept` |
| `seq_scan_large` | warning | a new sequential scan reads a table of ≥ `seq_scan_rows` rows and returns < 10% of it | `seq_scan_rows` 10,000 | a sequential scan over a large table is expensive at production size | index the filter columns, or confirm the table is small in production |
| `join_method` | warning | the join method changes between the same relations, with ≥ `join_rows` output rows | `join_rows` 10,000 | a different join method can change the cost of a large join by orders of magnitude | check join keys and indexes; review statistics if estimates moved |
| `sort_hash_work_mem` | warning | a new Sort or Hash node is estimated above `work_mem` (rows × width) | `work_mem_bytes` 4 MB | the operator may spill to disk | add an index that gives the order, or raise `work_mem` for the query |
| `query_error` | error | the head query no longer plans (for example after a type change) | none | the application would fail | fix the migration or the query |
| `cost_ratio` | info | the head's top cost is ≥ `cost_ratio` times the base's, and ≥ `min_cost` | `cost_ratio` 2.0; `min_cost` 1000 | cost is an estimate; a rise is a signal to look | compare the plans; verify on a representative workload |
| `shape_changed` | info | a plan changed and no specific rule above applied | none | the plan now does its work in a different order or with different operators | decide whether the change is expected |

## Notes

- **Shape first, cost second.** The shape rules decide pass or fail. Cost is info only.
- **Every rule runs for every change.** A change can trigger several rules. Each (rule, relation) appears once per query.
- **Estimated findings** are labelled. They depend on statistics the twin estimated or could not import.
- **Selectivity is measured from the bundle's table size.** Without a table size the rules fall back to the returned row
  count, which is weaker. Check `plantwin doctor` for stats completeness.
- **The 10% cut is a planner rule of thumb,** not a fitted value (ADR-0008).

## Accepting a finding

`plantwin accept <query> <rule> [relation]` records the decision in `.plantwin/accepted.json`. The next check does not
count the matching finding, and the report notes how many were accepted. Commit the file, so the decision is reviewed.
