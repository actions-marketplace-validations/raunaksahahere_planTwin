#!/usr/bin/env bash
# Build an EMPTY clone of database $SRC with the same schema and only imported statistics.
# usage: build_clone.sh <container> <src_db> <clone_db>
set -euo pipefail
C="$1"; SRC="$2"; CLONE="$3"
docker exec "$C" psql -U postgres -q -c "DROP DATABASE IF EXISTS $CLONE" -c "CREATE DATABASE $CLONE"
docker exec "$C" pg_dump -U postgres --schema-only "$SRC" | docker exec -i "$C" psql -U postgres -d "$CLONE" -q -v ON_ERROR_STOP=1
docker exec "$C" pg_dump -U postgres --statistics-only "$SRC" > "${TMPDIR:-/tmp}/stats_$$.sql"
docker cp "${TMPDIR:-/tmp}/stats_$$.sql" "$C":/tmp/stats.sql
rm -f "${TMPDIR:-/tmp}/stats_$$.sql"
# A twin must never be vacuumed or analyzed: that would overwrite the imported stats.
docker exec -i "$C" psql -U postgres -d "$CLONE" -q -v ON_ERROR_STOP=1 <<'SQL'
DO $$ DECLARE r record; BEGIN
  FOR r IN SELECT format('%I.%I', n.nspname, c.relname) AS name FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
           WHERE c.relkind = 'r' AND n.nspname = 'public' LOOP
    EXECUTE format('ALTER TABLE %s SET (autovacuum_enabled = false)', r.name);
  END LOOP; END $$;
SQL
ERRS=$(docker exec "$C" psql -U postgres -d "$CLONE" -q -v ON_ERROR_STOP=0 -f /tmp/stats.sql 2>&1 >/dev/null | grep -c ERROR || true)
echo "stats restore errors: $ERRS"
echo "clone $CLONE built: $(docker exec "$C" psql -U postgres -d "$CLONE" -tAc "SELECT count(*) FROM pg_class WHERE relkind IN ('r','i') AND reltuples > 0") relations with imported reltuples; physical table bytes: $(docker exec "$C" psql -U postgres -d "$CLONE" -tAc "SELECT coalesce(sum(pg_relation_size(oid)),0) FROM pg_class WHERE relkind='r' AND relnamespace='public'::regnamespace")"
