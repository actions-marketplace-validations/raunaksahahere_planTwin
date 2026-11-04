"""List the tables whose definition changed between two schema dumps.

usage: estimated_tables.py <base_schema.sql> <head_schema.sql> <out.json>
A changed line is attributed to the table it belongs to: a line that names "public.<table>" (index, constraint,
ALTER TABLE), or otherwise the table whose CREATE TABLE block contains it. The imported stats no longer describe
such a table exactly, so findings on it are labelled "estimated". This is a conservative rule, not a measurement.
"""
import difflib, json, re, sys

CREATE = re.compile(r"^CREATE TABLE public\.(\w+) \(")
NAMED = re.compile(r"public\.(\w+)")

def contexts(lines):
    """Return, for each line index, the table whose CREATE TABLE block contains it (or None)."""
    out, current = [None] * len(lines), None
    for i, line in enumerate(lines):
        m = CREATE.match(line)
        if m:
            current = m.group(1)
        out[i] = current
        if current and line.startswith(")"):
            current = None
    return out

def tables_for(line, ctx):
    if line.startswith("COMMENT ON"):
        return set()  # comments do not change planner statistics
    names = set(NAMED.findall(line))
    if names:
        return names
    return {ctx} if ctx else set()

base, head, out = sys.argv[1:4]
a = open(base).read().splitlines()
b = open(head).read().splitlines()
ca, cb = contexts(a), contexts(b)
changed = set()
for tag, i1, i2, j1, j2 in difflib.SequenceMatcher(None, a, b, autojunk=False).get_opcodes():
    if tag == "equal":
        continue
    for i in range(i1, i2):
        changed |= tables_for(a[i], ca[i])
    for j in range(j1, j2):
        changed |= tables_for(b[j], cb[j])
json.dump(sorted(changed), open(out, "w"), indent=1)
print("estimated tables:", sorted(changed))
