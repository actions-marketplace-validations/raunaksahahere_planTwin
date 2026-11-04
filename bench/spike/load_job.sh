#!/usr/bin/env bash
# Load the Join Order Benchmark (IMDb, May 2013 CSVs) into database $DB.
# The container must mount the CSV directory at /data (read-only), for example:
#   docker run -d --name plantwin-job -v "$JOBDATA":/data:ro -e POSTGRES_HOST_AUTH_METHOD=trust plantwin-postgres:18
# usage: load_job.sh <container> <database> <jobrepo_dir>
# jobrepo_dir is a checkout of https://github.com/gregrahn/join-order-benchmark (schema.sql, fkindexes.sql).
set -euo pipefail
C="$1"; DB="$2"; REPO="$(cd "$3" && pwd)"
TABLES="aka_name aka_title cast_info char_name comp_cast_type company_name company_type complete_cast info_type keyword kind_type link_type movie_companies movie_info movie_info_idx movie_keyword movie_link name person_info role_type title"
docker exec "$C" psql -U postgres -q -c "DROP DATABASE IF EXISTS $DB" -c "CREATE DATABASE $DB"
docker exec -i "$C" psql -U postgres -d "$DB" -q -v ON_ERROR_STOP=1 < "$REPO/schema.sql"
for t in $TABLES; do
  docker exec -i "$C" psql -U postgres -d "$DB" -q -v ON_ERROR_STOP=1 -c "\copy $t FROM '/data/$t.csv' csv escape '\\'"
done
docker exec -i "$C" psql -U postgres -d "$DB" -q -v ON_ERROR_STOP=1 < "$REPO/fkindexes.sql"
docker exec "$C" psql -U postgres -d "$DB" -q -c "ANALYZE"
docker exec "$C" psql -U postgres -d "$DB" -tAc "SELECT 'title=' || count(*) FROM title"
