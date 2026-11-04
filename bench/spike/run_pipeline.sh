#!/usr/bin/env bash
# One full pipeline run for a benchmark. Writes ../results/<bench>/<label>/.
# usage: run_pipeline.sh <bench> <label> <container> <database> <queries_dir>
#   bench:   step0 (TPC-H) | job
# Requires: the container's database already loaded (load_tpch.sh / load_job.sh). The extension is in the image.
set -euo pipefail
cd "$(dirname "$0")"
BENCH="$1"; L="$2"; C="$3"; DB="$4"; Q="$5"
R="../results/$BENCH/$L"
CLONE="${DB}_clone"
mkdir -p "$R"
HOOK="SET plantwin.ci_marker = 'ci:spike'; LOAD 'plantwin'; SET plantwin.enabled = on;"
docker exec "$C" psql -U postgres -d "$DB" -q -c "ANALYZE"            # fresh sampling for this run
python3 record_plans.py "$C" "$DB" "$Q" "$R/populated" > "$R/populated.out"
./build_clone.sh "$C" "$DB" "$CLONE" > "$R/clone_build.log"
python3 record_plans.py "$C" "$CLONE" "$Q" "$R/clone_no_hook" > "$R/clone_no_hook.out"
python3 record_plans.py "$C" "$CLONE" "$Q" "$R/clone_hook" "$HOOK" > "$R/clone_hook.out"
python3 compare_plans.py "$R/populated" "$R/clone_no_hook" "$R/comparison_no_hook.md" > "$R/comparison_no_hook.txt"
python3 compare_plans.py "$R/populated" "$R/clone_hook" "$R/comparison_hook.md" > "$R/comparison_hook.txt"
tail -1 "$R/comparison_no_hook.txt"; tail -1 "$R/comparison_hook.txt"
