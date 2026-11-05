-- Partitions: the parent has no storage and is skipped; each child is sized from its own imported statistics.
SET plantwin.ci_marker = 'ci:regress';
LOAD 'plantwin';
CREATE TABLE plantwin_p (id int, v int) PARTITION BY RANGE (id);
CREATE TABLE plantwin_p1 PARTITION OF plantwin_p FOR VALUES FROM (0) TO (1000000);
CREATE TABLE plantwin_p2 PARTITION OF plantwin_p FOR VALUES FROM (1000000) TO (2000000);
CREATE INDEX plantwin_p_v ON plantwin_p (v);
SELECT * FROM pg_catalog.pg_restore_relation_stats(
    'version', '180006'::integer, 'schemaname', 'public', 'relname', 'plantwin_p1',
    'relpages', '4480'::integer, 'reltuples', '1e+06'::real,
    'relallvisible', '0'::integer, 'relallfrozen', '0'::integer);
SELECT * FROM pg_catalog.pg_restore_relation_stats(
    'version', '180006'::integer, 'schemaname', 'public', 'relname', 'plantwin_p2',
    'relpages', '4480'::integer, 'reltuples', '1e+06'::real,
    'relallvisible', '0'::integer, 'relallfrozen', '0'::integer);

SET plantwin.enabled = off;
EXPLAIN SELECT * FROM plantwin_p WHERE v = 7;

SET plantwin.enabled = on;
EXPLAIN SELECT * FROM plantwin_p WHERE v = 7;

SET plantwin.enabled = off;
DROP TABLE plantwin_p;
