#!/usr/bin/env bash
# Synthetic reproduction of the empty-clone planner gap (PG18).
# Not the Step 0 gate: the gate uses TPC-H SF1. This shows the mechanism on 1M rows.
# Usage: bench/spike/synthetic_repro.sh [container-name]
set -euo pipefail
C="${1:-pt18}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
docker rm -f "$C" >/dev/null 2>&1 || true
docker run -d --name "$C" -e POSTGRES_PASSWORD=pw -e POSTGRES_HOST_AUTH_METHOD=trust postgres:18 >/dev/null
until docker exec "$C" pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
sleep 2
docker exec -i "$C" psql -U postgres -v ON_ERROR_STOP=1 -q <<'SQL'
CREATE TABLE prod_t AS SELECT g AS id, (g % 100) AS status FROM generate_series(1,1000000) g;
CREATE INDEX prod_t_status ON prod_t(status);
ANALYZE prod_t;
CREATE DATABASE empty_clone;
SQL
docker exec "$C" pg_dump -U postgres --statistics-only -t prod_t postgres > "$WORK/stats.sql"
docker cp "$WORK/stats.sql" "$C":/tmp/stats.sql
docker exec -i "$C" psql -U postgres -d empty_clone -q -c "CREATE TABLE prod_t (id bigint, status int); CREATE INDEX prod_t_status ON prod_t(status);" -f /tmp/stats.sql >/dev/null
echo "== production"; docker exec "$C" psql -U postgres -tAc "EXPLAIN SELECT * FROM prod_t WHERE status = 7"
echo "== empty clone, imported stats only"; docker exec "$C" psql -U postgres -d empty_clone -tAc "EXPLAIN SELECT * FROM prod_t WHERE status = 7"
