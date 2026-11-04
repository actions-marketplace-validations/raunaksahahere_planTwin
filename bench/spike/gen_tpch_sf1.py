"""Generate TPC-H SF1 with DuckDB's official tpch extension.

Writes pipe-delimited CSVs (one per table) and the 22 query texts (q01.sql..q22.sql)
into the directory given as argv[1]. Requires: pip install duckdb (version pinned in bench/spike/requirements.txt).
"""
import os, sys
import duckdb

out = os.path.abspath(sys.argv[1])
os.makedirs(out, exist_ok=True)
con = duckdb.connect()
con.execute("INSTALL tpch; LOAD tpch;")
con.execute("CALL dbgen(sf=1);")
for t in ["region", "nation", "supplier", "customer", "part", "partsupp", "orders", "lineitem"]:
    con.execute(f"COPY {t} TO '{out}/{t}.csv' (HEADER, DELIMITER '|')")
for nr, text in con.execute("SELECT query_nr, query FROM tpch_queries() ORDER BY query_nr").fetchall():
    with open(f"{out}/q{nr:02d}.sql", "w") as f:
        f.write(text)
print("wrote SF1 CSVs and 22 queries to", out)
