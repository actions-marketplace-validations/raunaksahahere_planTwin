DROP INDEX orders_customer_idx;
CREATE INDEX orders_customer_open ON orders (customer_id) WHERE status = 'open';
