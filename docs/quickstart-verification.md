# Quickstart verification (the 10-minute rule)

**Requirement.** A stranger with Docker reaches a first report from the README alone, in under ten minutes.

**What was run.** The three quickstart commands from `README.md`, in a **fresh clone** of the repository at commit
`4913f8b`, in a new directory. Nothing was copied from the working tree. The clone was built as follows.

| Step | Command (from README) | Time | Result |
|---|---|---:|---|
| 1 | `git clone` of the committed repository | < 1 s | clean clone |
| 2 | `make image` | 72 s | image built from the clone's Dockerfile (build cache pruned first) |
| 3 | `make build` | 13 s | `bin/plantwin` built in the pinned golang container |
| 4 | `./bin/plantwin demo` | 9 s | report produced, exit code 1 (the demo migration is flagged, as designed) |
| | **Total** | **about 94 s** | |

**Conditions, stated plainly.**

- Same machine as development: Linux, Docker 29.9.2, 12 cores, 7 GB RAM. This is not a clean machine. It is a fresh
  clone with no shared build cache. The base image `postgres:18` and the `golang:1.24` image were already pulled, and
  the Go module cache volume (`plantwin-gomod`) was already populated. A machine without those images adds the pull
  time to step 2 and step 3.
- Build cache was pruned before step 2 (`docker builder prune -af`), so the image build did real work.
- The demo creates a synthetic production database with 500,000 rows, so its time depends on the machine.

**The first report, as produced in step 4** (`plantwin demo`, Markdown format):

```
## PlanTwin: HEAD~1 → HEAD

**Result: failing.** 1 error(s), 0 warning(s), 1 info.

> **Estimated:** statistics for orders were estimated, not imported. Findings on them are labelled.

> **Note:** 1 statistics object(s) on HEAD could not be restored (usually expected for objects the migration changed); they are estimated. First: relation public.orders_customer_idx: ERROR: relation "public.orders_customer_idx" does not exist (SQLSTATE 42P01)

### `customer_lookup`

- **ERROR** `lost_index` *(estimated)*: Index use lost on orders: a selective index is now unused, and the table is read by a sequential scan (about 500K rows).
  - Why this matters: Queries on this table used a selective index. Without it, PostgreSQL reads every row of the table.
  - Suggested fix: Recreate the index, or add it back in this migration. To accept the change, run `plantwin accept`.
- **INFO** `cost_ratio`: Estimated total cost rose 70.6x (110 to 7774).
  - Why this matters: Cost is an estimate. A large rise is a signal to look, not proof of a slowdown.
  - Suggested fix: Compare the two plans below and verify on a representative workload.

<details><summary>Plans, side by side</summary>

| HEAD~1 | HEAD |
|---|---|
| `Bitmap Heap Scan on orders  rows=28 cost=110` | `Gather  rows=28 cost=7774` |
| `  Bitmap Index Scan using orders_customer_idx  rows=28 cost=5` | `  Seq Scan on orders  rows=12 cost=6771` |

</details>

<sub>Timing: load 43ms · twins 2.442s · import 38ms · explain 16ms · total 2.541s</sub>
```

**What the report does not show yet, and what a real repository needs.** The demo supplies its own statistics and
signing key. A real repository needs a read replica, a role from `docs/privacy-and-redaction.md`, `plantwin init`, and a
query catalog. Those steps are documented in the README and were not timed here.

**Known rough edge.** The note about the statistics object that could not be restored is accurate but wordy. It says
that the index was dropped, which is the expected outcome, so it could read more plainly.

**Reproduce this record.** From a clean clone: `make image && make build && ./bin/plantwin demo`, and time each step.
