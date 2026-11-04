"""Record EXPLAIN (FORMAT JSON) for each query, keeping errors instead of stopping.

usage: record_corpus.py <container> <db> <queries_dir> <out_dir> [prelude_sql]
Writes <out_dir>/<name>.json for a plan, or <out_dir>/<name>.err for a query that does not plan.
"""
import json, os, re, sys
from dbutil import run_sql

container, db, qdir, out = sys.argv[1:5]
prelude = sys.argv[5] if len(sys.argv) > 5 else ""
os.makedirs(out, exist_ok=True)
for f in os.listdir(out):
    os.remove(os.path.join(out, f))
for name in sorted(f[:-4] for f in os.listdir(qdir) if f.endswith(".sql")):
    sql = open(os.path.join(qdir, name + ".sql")).read().strip().rstrip(";")
    ok, stdout, stderr = run_sql(container, db, (prelude + "\n" if prelude else "") + f"EXPLAIN (FORMAT JSON) {sql};")
    if ok:
        plan = json.loads(stdout)[0]["Plan"]
        json.dump(plan, open(os.path.join(out, name + ".json"), "w"), indent=1)
    else:
        msg = re.sub(r"^ERROR:\s*", "", stderr.splitlines()[0]) if stderr else "unknown error"
        open(os.path.join(out, name + ".err"), "w").write(msg + "\n")
print(f"{db}: recorded {len(os.listdir(out))} results into {out}")
