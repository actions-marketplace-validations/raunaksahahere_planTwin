"""Record EXPLAIN (FORMAT JSON) plans (no ANALYZE) for every query file in a directory.

usage: record_plans.py <container> <database> <queries_dir> <out_dir> [prelude_sql]
Query files: qNN.sql (TPC-H) or NNx.sql (JOB). Schema and index scripts are excluded by name.
prelude_sql runs first in the same session, e.g. "SET plantwin.ci_marker='ci:spike'; LOAD 'plantwin'; SET plantwin.enabled = on;".
Writes <out_dir>/<name>.json (raw plan) and <out_dir>/<name>.log (one line per query).
"""
import json, os, re, subprocess, sys

container, db, qdir, out = sys.argv[1:5]
prelude = sys.argv[5] if len(sys.argv) > 5 else ""
os.makedirs(out, exist_ok=True)

def query_names(d):
    return sorted(f[:-4] for f in os.listdir(d) if re.match(r"^(q\d\d|\d\w*)\.sql$", f))

def psql(sql):
    r = subprocess.run(["docker", "exec", "-i", container, "psql", "-U", "postgres", "-d", db, "-tAq", "-v", "ON_ERROR_STOP=1"],
                       input=prelude + "\n" + sql, capture_output=True, text=True)
    if r.returncode != 0:
        raise SystemExit(f"psql failed: {r.stderr.strip()}")
    return r.stdout

log = []
for name in query_names(qdir):
    sql = open(os.path.join(qdir, name + ".sql")).read().strip().rstrip(";")
    plan = json.loads(psql(f"EXPLAIN (FORMAT JSON) {sql};"))[0]["Plan"]
    with open(os.path.join(out, name + ".json"), "w") as f:
        json.dump(plan, f, indent=1)
    log.append(f"{name} top={plan['Node Type']} rows={plan['Plan Rows']} cost={plan['Total Cost']}")
open(os.path.join(out, "record.log"), "w").write("\n".join(log) + "\n")
print(f"recorded {len(log)} plans into {out}")
