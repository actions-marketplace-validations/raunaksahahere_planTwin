-- Synthetic production data for the Django example. Deterministic: setseed fixes the random sequence.
-- Table and column names are the ones Django creates for shop.Customer and shop.Order (see migrations/0001_initial.py).
SELECT setseed(0.7);
INSERT INTO shop_customer (id, name, region)
SELECT g, 'name' || g, (ARRAY['EU','US','APAC'])[1 + floor(random() * 3)::int]
FROM generate_series(1, 20000) g;
INSERT INTO shop_order (id, customer_id, status, created_at, total)
SELECT g,
       1 + floor(power(random(), 3) * 20000)::int,
       CASE WHEN random() < 0.05 THEN 'open' WHEN random() < 0.9 THEN 'closed' ELSE 'returned' END,
       TIMESTAMP '2024-01-01' + random() * INTERVAL '900 days',
       round((random() * 500)::numeric, 2)
FROM generate_series(1, 500000) g;
ANALYZE;
