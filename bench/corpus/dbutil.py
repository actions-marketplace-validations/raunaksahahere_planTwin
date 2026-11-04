"""Small psql helper shared by the corpus scripts. Runs SQL inside a container and returns (ok, stdout, stderr)."""
import subprocess

def run_sql(container, db, sql, quiet=True, tuples=True):
    args = ["docker", "exec", "-i", container, "psql", "-U", "postgres", "-d", db, "-v", "ON_ERROR_STOP=1"]
    if quiet:
        args.append("-q")
    if tuples:
        args += ["-tA"]
    r = subprocess.run(args, input=sql, capture_output=True, text=True)
    return r.returncode == 0, r.stdout, r.stderr.strip()
