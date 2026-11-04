-- Deterministic skewed data: setseed fixes the random sequence, so every run builds the same rows.
SELECT setseed(0.42);
INSERT INTO customers
SELECT g, 'name' || g, (ARRAY['EU','US','APAC','LATAM'])[1 + floor(random() * 4)::int]
FROM generate_series(1, 100000) g;

-- Popular customers: customer_id is skewed towards small ids.
INSERT INTO orders
SELECT g,
       1 + floor(power(random(), 3) * 100000)::int,
       CASE WHEN random() < 0.05 THEN 'open' WHEN random() < 0.94 THEN 'closed' ELSE 'returned' END,
       TIMESTAMP '2024-01-01' + random() * INTERVAL '900 days',
       round((random() * 500)::numeric, 2)
FROM generate_series(1, 1000000) g;

INSERT INTO order_items
SELECT o.id, l, 'SKU' || (1 + floor(random() * 20000)::int), 1 + floor(random() * 5)::int
FROM generate_series(1, 1000000) o(id), generate_series(1, 2) l(line);
