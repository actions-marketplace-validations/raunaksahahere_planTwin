ALTER TABLE orders ALTER COLUMN customer_id TYPE text USING customer_id::text;
