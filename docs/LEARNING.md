# Learning guide: how PlanTwin works, and why

This guide is for defending the project in an interview. It explains each component in plain language, the PostgreSQL
planner ideas the project depends on, where to look in the code, and 25 questions with answers grounded in this
repository. Where a claim rests on a measurement, the file that holds the measurement is named.

## 1. The problem in one paragraph

A PostgreSQL planner chooses a plan (an index scan, a hash join, a sequential scan) from **estimates**. Estimates come
from statistics that `ANALYZE` collects. A CI database is empty or tiny, so its statistics are wrong, and the planner
picks a different plan than production does. A migration can look harmless in CI and then turn an index scan into a
sequential scan in production. PlanTwin recreates production's *statistics* in CI, without production rows, and
compares the plans before and after the change.

## 2. The planner concepts the project uses

### 2.1 Cost model
Every plan node has a startup cost and a total cost, in abstract units. The units come from a few settings:
`seq_page_cost` (1.0 by default), `random_page_cost` (4.0), `cpu_tuple_cost` (0.01), `cpu_index_tuple_cost` (0.005),
and `cpu_operator_cost` (0.0025). These are estimates, not seconds. PlanTwin therefore uses cost as the *secondary*
signal (the `cost_ratio` rule is info only) and the shape of the plan as the primary one.

### 2.2 Relation size: where the planner gets it
The planner sizes a table from its **physical** block count (`RelationGetNumberOfBlocks`) and then uses the density
of tuples per block that was last recorded in `pg_class`. In `src/backend/access/table/tableam.c`, around line 703,
`table_block_relation_estimate_size()` has a quick exit: if the table has zero blocks, it returns zero tuples, whatever
`pg_class.reltuples` says. This is the core fact the project relies on. An empty CI table therefore plans as if it had
no rows. The reproduction is in `bench/spike/synthetic_repro.sh` and in `docs/why-a-hook.md`.

### 2.3 `pg_statistic`, `pg_stats`, and what they contain
`ANALYZE` writes one row per column into `pg_statistic`. The important fields (decoded by the `pg_stats` view):

- `stanullfrac`: fraction of NULLs.
- `stawidth`: average width in bytes (used for memory estimates).
- `stadistinct`: number of distinct values (negative means a fraction of the row count).
- **Statistic kinds** (`stakind`): 1 = most common values (MCV) with frequencies, 2 = histogram bounds, 3 = correlation
  between physical order and column order, and others for arrays and ranges.

PlanTwin reads `pg_statistic` with the same `CASE` decoding as the view (`internal/export/export.go`). MCVs and
histogram bounds contain **real values**, which is why redaction exists.

**Equality estimates.** For `col = 'x'`: if `'x'` is in the MCV list, the selectivity is its frequency. If not, the
planner spreads the remaining probability over the other distinct values. The measured consequence is in
`docs/privacy-and-redaction.md`: when every common value is replaced by a surrogate, an unseen literal gets an
estimate of one row (25,150 → 1 on the demo).

### 2.4 Generic and custom plans
A prepared statement with `$1` can use a **custom** plan (built for the actual value) or a **generic** plan (built
without knowing the value, so it uses average selectivity). PostgreSQL chooses between them. PlanTwin explains
parameterised queries with `EXPLAIN (FORMAT JSON, GENERIC_PLAN)`, which is available from PostgreSQL 16. Literal
queries are explained as written. The limit (a custom plan for a specific literal can differ) is in
`docs/limitations.md`.

### 2.5 GEQO
For a join of 12 or more FROM items (`geqo_threshold`, default 12 in `src/backend/utils/misc/guc_tables.c`), PostgreSQL searches join orders with a genetic
algorithm instead of exhaustive dynamic programming. The result depends on a random seed, `geqo_seed`. I checked in
PostgreSQL 18's source (`src/backend/optimizer/geqo/geqo_main.c`, `geqo()`): the generator is reseeded at the start of
every planning call with `geqo_set_seed(root, Geqo_seed)`. So a given query with given statistics always gets the same
plan. The JOB queries with 24–28 FROM items use GEQO, and the fidelity results include them.

### 2.6 Hooks and chaining
`get_relation_info_hook` (declared in `src/backend/optimizer/util/plancat.c`, called at line 574) lets an extension edit
the planner's view of a relation after the built-in code has filled it in. HypoPG uses the same mechanism for
hypothetical indexes. Hooks are **chained**: each extension saves the previous hook pointer and calls it first. In
`ext/plantwin/plantwin.c`, `plantwin_get_relation_info` calls `prev_get_relation_info_hook` before it changes anything,
so other extensions keep their behaviour.

### 2.7 B-tree height
The planner charges an index scan a descent cost that grows with the tree height. In PostgreSQL 18,
`btcostestimate` (`src/backend/utils/adt/selfuncs.c`, line 7805) computes it as
`(tree_height + 1) × DEFAULT_PAGE_CPU_MULTIPLIER × cpu_operator_cost`, with the multiplier set to 50 (line 145). The
height is read from the physical metapage. An empty index has no root, so its height is 0 and the descent cost disappears. The
spike showed index scans on the empty clone costing 0.00 (`docs/why-a-hook.md`). PostgreSQL does not store the height
in `pg_class`, so the extension estimates it (ADR-0002, `estimate_btree_height` in `plantwin.c`). The estimate uses the
70% fill that PostgreSQL uses for internal pages (`BTREE_NONLEAF_FILLFACTOR`, `nbtsort.c`). It matches the real level
for 42 of 44 JOB indexes and 8 of 8 TPC-H indexes.

## 3. Components, in plain language

| Component | What it does, in one sentence | Where |
|---|---|---|
| The extension | When asked, it tells the planner the table and index sizes that production had, and it refuses to load outside CI. | `ext/plantwin/plantwin.c` |
| Export | Reads planner statistics from a replica with a role that cannot read rows, redacts common values, and writes a signed file. | `internal/export/export.go` |
| Bundle | The file that leaves production: format, redaction, and an ed25519 signature. | `internal/bundle/` |
| Twin | A throwaway PostgreSQL 18 container with the migrations applied and the statistics loaded. | `internal/twin/twin.go` |
| Estimator | Sizes indexes that the migration creates and production has never analysed. | `internal/twin/estimate_new_indexes.sql` |
| Plan parser | Reads `EXPLAIN (FORMAT JSON)` output in three forms. | `internal/plan/plan.go` |
| Shape | Reduces a plan to what it does: operators, relations, indexes, join order. Estimates are dropped. | `internal/shape/shape.go` |
| Differ | Finds the smallest set of changes between two shapes. | `internal/diff/diff.go` |
| Rules | Turns changes into findings, each with a severity, a reason and a fix. | `internal/rules/rules.go` |
| Report | Markdown for a PR comment, JSON with a `schema_version`, and SARIF. | `internal/report/report.go` |
| Check | Puts it together: worktrees, two twins, import, explain, rules, report. | `internal/check/check.go` |
| CLI | Commands, exit codes, and plain-language errors. | `cmd/plantwin/` |

## 4. Where to look in the code

Start with the path a user takes:

1. `cmd/plantwin/main.go` (dispatch, exit codes) and `commands.go` (one function per command).
2. `internal/check/check.go`, `func Run`: read it top to bottom. It is the whole pipeline.
3. `internal/twin/twin.go`: `ImportBundle` and `attributeCall` show how statistics go into PostgreSQL.
4. `internal/rules/rules.go`, `func Analyse`: the rules, and why each change is checked against every rule.
5. `ext/plantwin/plantwin.c`: `_PG_init` (the guard) and `plantwin_get_relation_info` (the hook).
6. The evidence: `bench/spike/` (fidelity), `bench/corpus/` (migration corpus), `bench/redaction/` (redaction).

## 5. Decisions and the reasons for them

- **Two dependencies** (pgx and yaml) and the docker command-line tool instead of the Docker SDK (ADR-0009).
- **The cache stores the schema, not the container**: the official image declares its data directory as a volume,
  which `docker commit` does not capture (ADR-0012).
- **Shapes first, cost second** (ADR-0007, ADR-0008).
- **A 10% selectivity cut** decides whether losing an index is a regression (ADR-0008). It was found by the first
  corpus run, which is why the corpus is not a held-out test.
- **Redaction default `hash`**, with its measured cost (`docs/privacy-and-redaction.md`).

## 6. Twenty-five interview questions, with answers

**1. Why doesn't importing statistics into an empty database reproduce production plans?**
Because the planner sizes a table from its physical block count. With zero blocks it takes the quick exit in
`table_block_relation_estimate_size` (tableam.c, around line 703) and returns zero tuples, whatever `pg_class` says.
The imported statistics are present but unused until the extension overrides the size (`docs/why-a-hook.md`).

**2. Why a hook and not a change to the relation files?**
The hook changes the planner's in-memory view and writes nothing to disk. Padding relation files with zero pages was
considered and rejected: it writes behind the server's back and depends on the storage layout.

**3. What exactly does the hook override?**
Table pages, tuples and allvisfrac; index pages and tuples, with the planner's own rule for non-partial indexes
(`plancat.c` takes the table's tuples); and the B-tree height (`plantwin.c`). Overrides apply only when the catalog has
imported stats (`relpages > 0`).

**4. Why do you call the previous hook first?**
So that other extensions in the chain keep their behaviour. PlanTwin's values then win for the fields it sets.

**5. How do you guarantee the extension never loads in production?**
`_PG_init` checks `plantwin.ci_marker` before it defines any setting, and fails if it is empty. The check runs before
the setting is defined, because defining first and then failing made a second `LOAD` in the same session fail with
"attempt to redefine parameter". With `shared_preload_libraries`, the postmaster refuses to start (verified).

**6. What is a plan "shape", and why ignore estimates?**
The operators, relations, indexes and join order, in order. Estimates change with every `ANALYZE`, so a shape that
includes them would flag noise. The shape is what the migration changed.

**7. How does the differ avoid cascading differences?**
It aligns children with a longest common subsequence over subtree signatures. A single inserted child is one
addition, not a change to every later sibling (a unit test checks it).

**8. A parallel plan put the sequential scan under `Gather`. What went wrong?**
The lost-index rule looked only at the top of a replaced subtree, so it missed the scan. The corpus measured two
regressions of 150× and 340× that the tool missed. Rules now search the whole subtree (`seqScansIn`).

**9. Why is losing an index not always a regression?**
If the index returned most of the table, a sequential scan is the right plan, and it can be faster. The rule uses a
10% cut. A 25% index scan on `customers` becoming a faster sequential scan was a false positive before the cut.

**10. What is the difference between a generic and a custom plan?**
A custom plan uses the actual parameter value to estimate selectivity. A generic plan averages over values. PlanTwin
explains with `GENERIC_PLAN` for parameterised queries, so the twin sees what the application sees for a prepared
statement.

**11. What is GEQO and why is it relevant?**
A genetic algorithm for join order, used from 12 FROM items (`geqo_threshold`). It is deterministic here because
`geqo()` reseeds per planning call (`geqo_main.c`). JOB has queries with 24–28 FROM items, so it is in the results.

**12. How do MCVs affect an equality estimate?**
A common value's estimate is its stored frequency. An unseen value gets the leftover probability, spread over the
other distinct values. If the surrogates cover every value, the leftover is about zero, which gives one row. The
`hash` mode measurement shows this (25,150 → 1 on the demo).

**13. Why did you choose hash redaction as the default?**
Privacy first: the bundle keeps frequencies and not literal values. The measured cost is that literal predicates on
common values lose their estimate, which can change a plan (`docs/privacy-and-redaction.md`). Per-column `keep` is the
documented escape for low-cardinality columns the team accepts exposing.

**14. How do you know the surrogates cannot be reversed?**
Each export draws a fresh 32-byte salt that is never written to the bundle. Without the salt, a dictionary of values
cannot be matched to the surrogates. The surrogates keep frequencies, so this is pseudonymisation of values, not
encryption, and the docs say so.

**15. Why does the export role need `SELECT` on `pg_statistic`?**
Because `pg_read_all_stats` does not cover `pg_statistic`, and the `pg_stats` view filters columns by table `SELECT`.
I checked both on PostgreSQL 18. The role then reads statistics but no rows, and an integration test verifies that.
The trade-off is that the role can see common values in the catalog, so it must be protected.

**16. How do you verify the bundle has not been changed?**
ed25519. `check` and `doctor` verify the signature with the committed public key, and stop on mismatch. Signing covers
the exact bytes, so a change to any byte fails verification.

**17. What is the schema cache, and why is it not a container cache?**
After migrations are applied, `pg_dump --schema-only` is stored under `.plantwin/cache/<hash>.sql`, keyed by the image
and the migration files. The official image declares its data directory as a volume, which `docker commit` does not
capture, so caching the container does not work (ADR-0012).

**18. How do you estimate the size of an index the migration creates?**
From the table's tuple count and the key widths in `pg_statistic`: pages = tuples × (key width + 12) / (8192 × 0.9).
A partial index gets a fixed 10% factor, which is a guess. Findings that depend on it are labelled *estimated*
(ADR-0010).

**19. Why are B-tree heights estimated, and how good is the estimate?**
The true height is read from the metapage, which an empty index does not have. PostgreSQL does not store it in
`pg_class`. The estimate uses the 70% fill for internal pages and the key width. It matches 42 of 44 JOB indexes and
8 of 8 TPC-H indexes. The two misses are close to a fanout boundary (`bench/results/`).

**20. How reliable is the fidelity result?**
On TPC-H, 22 of 22 plan shapes match with the hook, and 21 of 22 full trees match. On JOB, 112 and 113 of 113 shapes
match. But the populated database's own plans change between two `ANALYZE` runs for 31 of 113 JOB queries, and the twin
moves with them. So the result describes agreement with one statistics sample. The one unexplained residual is named.

**21. What did the corpus show, and what would you change?**
Recall 5/5 and no false positives at a 2× runtime bar, on 18 synthetic cases. But the rules were developed on the first
12, so it is not held out. I would build a held-out set with a separate query workload, and report the runtime bar's
sensitivity (1.5× recall drops to 5/8).

**22. Why did the first benchmark score look wrong?**
The scorer applied its absolute-time floor to the base time, so a query that went from 0.1 ms to 22 ms was ignored.
The floor now applies to the head time. A cold cache also produced a false positive, so the benchmark now takes a
warm-up run, and the report says so.

**23. Why is the Go toolchain run in a container?**
Go is not installed on the development host, and installing it would run downloaded code outside a reviewed container.
ADR-0006 pins `golang:1.24` and runs commands as the current user.

**24. How do you keep the CLI's dependency surface small?**
Two dependencies, both pinned: pgx for the PostgreSQL protocol, and yaml.v3. Everything else is the standard library.
Docker is driven through the `docker` command, so the Docker SDK is not linked (ADR-0009).

**25. What would you do differently next?**
Build the held-out corpus first; measure redaction on a schema with sensitive categorical columns; run the workflows
(actionlint, a real CI run) that are written but have not run; and decide the `hash` default with the measured cost in
front of the team.
