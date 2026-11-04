CREATE TABLE customers (id int PRIMARY KEY, name text NOT NULL, region text NOT NULL);
CREATE TABLE orders (
  id bigint PRIMARY KEY,
  customer_id int NOT NULL,
  status text NOT NULL,
  created_at timestamp NOT NULL,
  total numeric(10,2) NOT NULL
);
CREATE INDEX orders_customer_idx ON orders (customer_id);
CREATE INDEX orders_status_idx ON orders (status);
