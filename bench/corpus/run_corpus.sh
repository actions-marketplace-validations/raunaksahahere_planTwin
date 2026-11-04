#!/usr/bin/env bash
# Run the migration corpus end to end and write bench/results/corpus/.
#   prod_base         synthetic production-like data (schema.sql + gen_data.sql), ANALYZEd. Used for ground truth only.
#   twin_base         empty clone: base schema plus imported stats. This is what PlanTwin sees.
#   prod_<case>       prod_base copy with the migration applied and ANALYZEd (ground truth, head).
#   twin_<case>       empty clone of base, migration applied, stats imported, new indexes estimated (PlanTwin's head).
# Requires: docker, python3, and the image plantwin-postgres:18 built from the repo Dockerfile.
# usage: run_corpus.sh            (SCRATCH=<dir> optional, for schema and stats dumps)
set -euo pipefail
cd "$(dirname "$0")"
REPO="$(cd ../.. && pwd)"
C=plantwin-corpus
R="$REPO/bench/results/corpus"
S="${SCRATCH:-$(mktemp -d)}"; mkdir -p "$S"
PRELUDE="SET plantwin.ci_marker = 'ci:corpus'; LOAD 'plantwin'; SET plantwin.enabled = on;"
GO=(docker run --rm -v "$REPO":/src -w /src -v plantwin-gomod:/go/pkg/mod golang:1.24 go run -buildvcs=false ./bench/corpus/driver)

# Usage: psqlq <db> < file.sql   (SQL is read from stdin)
psqlq() { docker exec -i "$C" psql -U postgres -d "$1" -q -v ON_ERROR_STOP=1; }

docker rm -f "$C" >/dev/null 2>&1 || true
docker run -d --name "$C" -e POSTGRES_PASSWORD=pw -e POSTGRES_HOST_AUTH_METHOD=trust plantwin-postgres:18 -c autovacuum=off >/dev/null
until docker exec "$C" pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done; sleep 2
docker exec "$C" psql -U postgres -tAc "SHOW autovacuum" | grep -q off

rm -rf "$R"; mkdir -p "$R"/{plans,truth,cases,findings}
echo "== production-like base"
docker exec "$C" psql -U postgres -q -c "CREATE DATABASE prod_base"
psqlq prod_base < schema.sql
psqlq prod_base < gen_data.sql
docker exec "$C" psql -U postgres -d prod_base -q -c "ANALYZE"
# Table row counts for the selectivity rules (from the production-like statistics, as the stats bundle would supply).
docker exec "$C" psql -U postgres -d prod_base -tAc "SELECT json_object_agg(relname, reltuples) FROM pg_class WHERE relkind = 'r' AND relnamespace = 'public'::regnamespace" > "$S/table_rows.json"
cp "$S/table_rows.json" "$R/table_rows.json"

echo "== stats bundle from prod_base"
docker exec "$C" pg_dump -U postgres --schema-only prod_base > "$S/schema_base.sql"
docker exec "$C" pg_dump -U postgres --statistics-only prod_base > "$S/stats_base.sql"
docker cp "$S/stats_base.sql" "$C":/tmp/stats_base.sql
docker cp "$S/schema_base.sql" "$C":/tmp/schema_base.sql

make_twin() { # make_twin <db> ; empty schema from base, then stats import (errors tolerated and counted)
  docker exec "$C" psql -U postgres -q -c "DROP DATABASE IF EXISTS $1" -c "CREATE DATABASE $1"
  docker exec "$C" psql -U postgres -d "$1" -q -v ON_ERROR_STOP=1 -f /tmp/schema_base.sql
}
make_twin twin_base
ERRS=$(docker exec "$C" psql -U postgres -d twin_base -q -v ON_ERROR_STOP=0 -f /tmp/stats_base.sql 2>&1 >/dev/null | grep -c ERROR || true)
echo "   twin_base stats import errors: $ERRS"
python3 record_corpus.py "$C" twin_base queries "$R/plans/base" "$PRELUDE"
python3 measure_truth.py "$C" prod_base queries "$R/truth/base" 3

for CASE_DIR in cases/*/; do
  CASE=$(basename "$CASE_DIR")
  MIG="$CASE_DIR/migration.sql"
  echo "== $CASE"
  cp "$MIG" "$R/cases/$CASE.sql"
  # Ground truth: the migration on a copy of the production-like data.
  docker exec "$C" psql -U postgres -q -c "DROP DATABASE IF EXISTS prod_$CASE" -c "CREATE DATABASE prod_$CASE TEMPLATE prod_base"
  psqlq "prod_$CASE" < "$MIG"
  docker exec "$C" psql -U postgres -d "prod_$CASE" -q -c "ANALYZE"
  docker exec "$C" pg_dump -U postgres --schema-only "prod_$CASE" > "$S/schema_$CASE.sql"
  # The twin: base schema, migration, imported stats (errors counted), estimates for new indexes.
  make_twin "twin_$CASE"
  psqlq "twin_$CASE" < "$MIG"
  ERRS=$(docker exec "$C" psql -U postgres -d "twin_$CASE" -q -v ON_ERROR_STOP=0 -f /tmp/stats_base.sql 2>&1 >/dev/null | grep -c ERROR || true)
  docker cp "$REPO/internal/twin/estimate_new_indexes.sql" "$C":/tmp/estimate.sql
  docker exec "$C" psql -U postgres -d "twin_$CASE" -q -v ON_ERROR_STOP=1 -f /tmp/estimate.sql >/dev/null
  echo "   stats import errors (expected for changed objects): $ERRS"
  python3 estimated_tables.py "$S/schema_base.sql" "$S/schema_$CASE.sql" "$R/cases/$CASE.json"
  python3 record_corpus.py "$C" "twin_$CASE" queries "$R/plans/$CASE/head" "$PRELUDE"
  python3 measure_truth.py "$C" "prod_$CASE" queries "$R/truth/$CASE" 3
  # The driver runs inside the golang container, where the repository is mounted at /src.
  RR="${R#"$REPO"/}"
  "${GO[@]}" -base "/src/$RR/plans/base" -head "/src/$RR/plans/$CASE/head" -estimated "/src/$RR/cases/$CASE.json" -table-rows "/src/$RR/table_rows.json" -out "/src/$RR/findings/$CASE.json"
done

python3 score.py "$R" "$R/summary.md" | tail -6
echo "done. Summary: $R/summary.md"
