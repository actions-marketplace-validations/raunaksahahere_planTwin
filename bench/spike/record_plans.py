"""Record EXPLAIN (FORMAT JSON) plans (no ANALYZE) for the 22 TPC-H queries.

usage: record_plans.py <container> <database> <queries_dir> <out_dir>
Writes <out_dir>/qNN.json (raw plan) and prints a one-line summary per query.
"""
import json, os, subprocess, sys

container, db, qdir, out = sys.argv[1:5]
os.makedirs(out, exist_ok=True)

def psql(sql):
    r = subprocess.run(["docker", "exec", "-i", container, "psql", "-U", "postgres", "-d", db, "-tAq", "-v", "ON_ERROR_STOP=1"],
                       input=sql, capture_output=True, text=True)
    if r.returncode != 0:
        raise SystemExit(f"psql failed: {r.stderr.strip()}")
    return r.stdout

for i in range(1, 23):
    name = f"q{i:02d}"
    sql = open(os.path.join(qdir, name + ".sql")).read().strip().rstrip(";")
    raw = psql(f"EXPLAIN (FORMAT JSON) {sql};")
    plan = json.loads(raw)[0]["Plan"]
    with open(os.path.join(out, name + ".json"), "w") as f:
        json.dump(plan, f, indent=1)
    print(f"{name} top={plan['Node Type']} rows={plan['Plan Rows']} cost={plan['Total Cost']}")
