# PGXS build for the plantwin extension. Build against an installed PostgreSQL 18:
#   make USE_PGXS=1 PG_CONFIG=/path/to/pg_config && make USE_PGXS=1 install
# Regression tests (need a running server reachable by the pg_regress defaults):
#   make USE_PGXS=1 installcheck
MODULES = plantwin
EXTENSION = plantwin
DATA = sql/plantwin--0.1.sql
# plantwin_partition is written (sql/plantwin_partition.sql) but not in REGRESS until its expected output is generated
# and reviewed on PostgreSQL 18. Adding it without expected/plantwin_partition.out would fail the extension job.
REGRESS = plantwin_guard plantwin_hook
PG_CONFIG ?= pg_config
PG_CPPFLAGS = -Wall -Wextra -Wno-unused-parameter
PGXS := $(shell $(PG_CONFIG) --pgxs)
include $(PGXS)
