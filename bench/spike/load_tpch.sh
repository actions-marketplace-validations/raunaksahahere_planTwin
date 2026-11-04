#!/usr/bin/env bash
# Create database $DB in container $C, apply tpch_schema.sql, COPY SF1 CSVs, ANALYZE.
# usage: load_tpch.sh <container> <database> <csv_dir>
set -euo pipefail
C="$1"; DB="$2"; DIR="$(cd "$3" && pwd)"
HERE="$(cd "$(dirname "$0")" && pwd)"
docker exec "$C" psql -U postgres -q -c "DROP DATABASE IF EXISTS $DB" -c "CREATE DATABASE $DB"
docker exec -i "$C" psql -U postgres -d "$DB" -q -v ON_ERROR_STOP=1 < "$HERE/tpch_schema.sql"
docker exec "$C" mkdir -p /data
docker cp "$DIR/." "$C":/data/ >/dev/null
for t in region nation supplier customer part partsupp orders lineitem; do
  docker exec -i "$C" psql -U postgres -d "$DB" -q -v ON_ERROR_STOP=1 \
    -c "\copy $t FROM '/data/$t.csv' (FORMAT csv, DELIMITER '|', HEADER)"
done
docker exec "$C" psql -U postgres -d "$DB" -q -c "ANALYZE"
docker exec "$C" psql -U postgres -d "$DB" -tAc "SELECT 'lineitem=' || count(*) FROM lineitem"
