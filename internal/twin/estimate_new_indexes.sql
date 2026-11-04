-- Estimate size statistics for indexes that have no imported statistics.
--
-- Used on the twin after the stats bundle is imported. An index is "new" when it
-- has no imported pages (relpages = 0 after import). Its size is estimated from the
-- table's imported tuple count and the key column widths (pg_statistic.stawidth).
--
--   pages  = ceil(tuples_est * (key_width + 12) / (8192 * 0.90))   -- 12 bytes index tuple overhead, 90% fill
--   tuples = table reltuples, times 0.10 for a partial index (fixed guess, see ADR-0010)
--
-- Every estimated index is listed on stdout as "estimated: <index>", so the caller can
-- label findings that depend on it. Run as a superuser on the twin only, never on production.
DO $$
DECLARE
  r record;
  key_width int;
  tuples double precision;
  pages int;
  est_tuples double precision;
BEGIN
  FOR r IN
    SELECT ic.oid AS idx_oid, ic.relname AS idx_name, t.oid AS tbl_oid, t.relname AS tbl_name,
           t.reltuples AS tbl_tuples, i.indkey::int2[] AS keys, (i.indpred IS NOT NULL) AS is_partial
    FROM pg_index i
    JOIN pg_class ic ON ic.oid = i.indexrelid
    JOIN pg_class t  ON t.oid = i.indrelid
    JOIN pg_am am   ON am.oid = ic.relam
    WHERE ic.relnamespace = 'public'::regnamespace
      AND ic.relpages = 0
      AND am.amname = 'btree'
  LOOP
    SELECT COALESCE(SUM(COALESCE(s.stawidth, 8)), 8)::int INTO key_width
    FROM unnest(r.keys) WITH ORDINALITY k(attnum, ord)
    LEFT JOIN pg_statistic s
      ON s.starelid = r.tbl_oid AND s.staattnum = k.attnum AND NOT s.stainherit
    WHERE k.attnum > 0;

    tuples := GREATEST(r.tbl_tuples, 0);
    est_tuples := CASE WHEN r.is_partial THEN tuples * 0.10 ELSE tuples END;
    pages := GREATEST(1, CEIL(est_tuples * (key_width + 12) / (8192 * 0.90)))::int;

    PERFORM pg_catalog.pg_restore_relation_stats(
      'version', 180006,
      'schemaname', 'public',
      'relname', r.idx_name,
      'relpages', pages,
      'reltuples', est_tuples::real,
      'relallvisible', 0,
      'relallfrozen', 0);
    RAISE NOTICE 'estimated: %', r.idx_name;
  END LOOP;
END
$$;
