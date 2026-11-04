# ADR-0001: CI-only guard for the planner extension

**Context.** The extension changes planner estimates. Loaded in production by mistake, it would
silently alter plans. The blueprint requires that it refuse to load outside CI.

**Decision.**
- `_PG_init()` reads `plantwin.ci_marker` with `GetConfigOption(..., missing_ok)` before defining
  any GUC. If it is empty, the load fails with `ERROR`.
- The check runs before `DefineCustomStringVariable`. Defining first and then failing left the
  GUC defined, so a second `LOAD` in the same session failed with "attempt to redefine parameter".
  The regression test caught this.
- `plantwin.enabled` defaults to `off`. Both GUCs are `PGC_SUSET`, so ordinary roles cannot change them.
- The hook writes nothing to disk and changes no catalog. It only changes the planner's in-memory
  size fields for the current query.

**Verified.** `plantwin_guard` pg_regress test (load without marker fails; with marker it loads).
`shared_preload_libraries = plantwin` makes `pg_ctl start` fail with the same error, so a
production server cannot start with the library preloaded.

**Consequences.** A marker is a convention, not a secret: anyone with superuser rights can set it.
It prevents accidents, not deliberate misuse. The docs say so.
