"""Ground truth: run each query with EXPLAIN (ANALYZE) on a production-like database, several times.

usage: measure_truth.py <container> <db> <queries_dir> <out_dir> [runs]
Writes <out_dir>/<name>.json with every run's Execution Time (ms) and the median, or <name>.err if the query fails.
One warm-up run is executed first and discarded.
Only SELECT statements are run.
"""
import json, os, re, statistics, sys
from dbutil import run_sql

container, db, qdir, out = sys.argv[1:5]
runs = int(sys.argv[5]) if len(sys.argv) > 5 else 3
os.makedirs(out, exist_ok=True)
for name in sorted(f[:-4] for f in os.listdir(qdir) if f.endswith(".sql")):
    sql = open(os.path.join(qdir, name + ".sql")).read().strip().rstrip(";")
    times, err = [], None
    # Warm-up run, discarded: a freshly copied database is read from disk the first time, which would bias the timings.
    run_sql(container, db, f"EXPLAIN (ANALYZE, FORMAT JSON) {sql};")
    for _ in range(runs):
        ok, stdout, stderr = run_sql(container, db, f"EXPLAIN (ANALYZE, FORMAT JSON) {sql};")
        if not ok:
            err = re.sub(r"^ERROR:\s*", "", stderr.splitlines()[0]) if stderr else "unknown error"
            break
        times.append(json.loads(stdout)[0]["Execution Time"])
    if err:
        open(os.path.join(out, name + ".err"), "w").write(err + "\n")
    else:
        json.dump({"runs_ms": times, "median_ms": statistics.median(times)}, open(os.path.join(out, name + ".json"), "w"), indent=1)
print(f"{db}: measured {len(os.listdir(out))} queries into {out}")
