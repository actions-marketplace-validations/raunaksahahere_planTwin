---
name: False positive or missed regression
about: PlanTwin flagged a migration that is fine, or missed one that caused a slowdown
labels: accuracy
---

**The migration** (SQL, with table and column names renamed if needed)

**The query** (renamed if needed) and its parameters

**What PlanTwin reported** (paste the finding; no bundle contents)

**What actually happened** (runtime before and after, and how you measured it; `EXPLAIN (ANALYZE, BUFFERS)` is ideal)

**Was the statistics bundle fresh?** (date of export, whether the table was analysed since)

We use these reports to extend the migration corpus. Please say whether we may add a synthetic version of the case to
`bench/corpus`.
