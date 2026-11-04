SELECT i.sku, sum(i.qty) FROM order_items i JOIN orders o ON o.id = i.order_id WHERE o.customer_id = 77 GROUP BY i.sku;
