-- The table is physically empty; imported stats say 1e6 rows over 4480 pages.
SET plantwin.ci_marker = 'ci:regress';
LOAD 'plantwin';
CREATE TABLE plantwin_t (id int, v int);
CREATE INDEX plantwin_t_v ON plantwin_t (v);
SELECT * FROM pg_catalog.pg_restore_relation_stats(
    'version', '180006'::integer, 'schemaname', 'public', 'relname', 'plantwin_t',
    'relpages', '4480'::integer, 'reltuples', '1e+06'::real,
    'relallvisible', '0'::integer, 'relallfrozen', '0'::integer);
SELECT * FROM pg_catalog.pg_restore_relation_stats(
    'version', '180006'::integer, 'schemaname', 'public', 'relname', 'plantwin_t_v',
    'relpages', '2700'::integer, 'reltuples', '1e+06'::real,
    'relallvisible', '0'::integer, 'relallfrozen', '0'::integer);

-- Hook off (default): the planner sees the empty heap.
SET plantwin.enabled = off;
EXPLAIN SELECT * FROM plantwin_t WHERE v = 7;

-- Hook on: the planner uses the imported sizes.
SET plantwin.enabled = on;
EXPLAIN SELECT * FROM plantwin_t WHERE v = 7;

-- Clean up.
SET plantwin.enabled = off;
DROP TABLE plantwin_t;
