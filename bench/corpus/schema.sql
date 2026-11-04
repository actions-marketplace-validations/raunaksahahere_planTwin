-- Synthetic "production" schema for the migration corpus (not a public benchmark).
CREATE TABLE customers (id int PRIMARY KEY, name text NOT NULL, region text NOT NULL);
CREATE TABLE orders (
  id bigint PRIMARY KEY,
  customer_id int NOT NULL,
  status text NOT NULL,
  created_at timestamp NOT NULL,
  total numeric(10,2) NOT NULL
);
CREATE TABLE order_items (
  order_id bigint NOT NULL,
  line int NOT NULL,
  sku text NOT NULL,
  qty int NOT NULL,
  PRIMARY KEY (order_id, line)
);
CREATE INDEX customers_region_idx ON customers (region);
CREATE INDEX orders_customer_idx ON orders (customer_id);
CREATE INDEX orders_status_idx ON orders (status);
CREATE INDEX orders_created_idx ON orders (created_at);
