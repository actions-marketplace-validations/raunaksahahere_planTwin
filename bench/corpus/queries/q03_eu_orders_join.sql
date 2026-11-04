SELECT o.id, c.name FROM orders o JOIN customers c ON c.id = o.customer_id WHERE c.region = 'EU' AND o.created_at >= '2026-06-01';
