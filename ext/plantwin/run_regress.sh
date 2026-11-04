#!/usr/bin/env bash
# Runs the pg_regress tests against a temporary PG18 cluster. Used by the build image and locally.
# Must run as the 'postgres' user (pg_regress refuses root). Expects the extension already installed.
set -euo pipefail
BIN=/usr/lib/postgresql/18/bin
DATA="$(mktemp -d)/data"
"$BIN/initdb" -D "$DATA" -U postgres --auth=trust >/dev/null
"$BIN/pg_ctl" -D "$DATA" -l "$DATA/../server.log" -o "-k /var/run/postgresql -c listen_addresses=''" -w start >/dev/null
status=0
make USE_PGXS=1 installcheck || status=$?
"$BIN/pg_ctl" -D "$DATA" -m fast -w stop >/dev/null
exit $status
