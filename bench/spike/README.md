# Step 0 spike: reproducing the empty-clone planner gap

Run from this directory. Requires Docker, Python 3, and `pip install -r requirements.txt` (DuckDB, pinned).
Data files go to a directory you choose; they are not committed.

```bash
D=$(mktemp -d)
python3 gen_tpch_sf1.py "$D/tpch"                       # SF1 CSVs + 22 queries (DuckDB tpch)
./load_tpch.sh pt18 tpch "$D/tpch"                      # PG18 container "pt18" (docker run postgres:18)
python3 record_plans.py pt18 tpch "$D/tpch" ../results/step0/populated
./build_clone.sh pt18 tpch tpch_clone                   # empty DB with imported stats only
python3 record_plans.py pt18 tpch_clone "$D/tpch" ../results/step0/clone_no_hook
python3 compare_plans.py ../results/step0/populated ../results/step0/clone_no_hook ../results/step0/comparison_no_hook.md
./synthetic_repro.sh                                    # 1M-row mechanism check (own container)
```

Container setup, if needed: `docker run -d --name pt18 -e POSTGRES_PASSWORD=pw -e POSTGRES_HOST_AUTH_METHOD=trust postgres:18`.
