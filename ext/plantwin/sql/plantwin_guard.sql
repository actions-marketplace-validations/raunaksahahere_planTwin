-- Loading must fail without a CI marker.
LOAD 'plantwin';
-- With a marker, loading succeeds.
SET plantwin.ci_marker = 'ci:regress';
LOAD 'plantwin';
SHOW plantwin.enabled;
