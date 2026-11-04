# Example: Django project with a planted regression

A small Django project with two commits' worth of migrations. The second migration removes `orders_customer_idx`,
which the `customer_lookup` query needs.

**Status: not executed.** This example is written against Django 5.1 migration files and PlanTwin's `command`
migration tool. Django is not installed in the environment where PlanTwin was developed, so the example has not been
run end to end. The SQL example (`examples/sql-migrations`) is the verified one. Running this example requires Python 3,
Django 5.1 and a `DATABASE_URL` for the twin, which PlanTwin sets.

## Try it

```bash
pip install "Django>=5.1,<5.2"     # on the machine that runs `plantwin check`
# Make two commits: the base has only 0001_initial.py; the head adds 0002_remove_customer_index.py.
git init && git add shop/migrations/0001_initial.py shop/models.py && git commit -m "base"
git add shop/migrations/0002_remove_customer_index.py && git commit -m "Remove customer index"
plantwin check --base HEAD~1 --head HEAD
```

In a real project, export the statistics from the replica first (`plantwin export --sign-key …`), then run `check`.

## Files

- `plantwin.yaml` uses the `command` tool with `python manage.py migrate`. Queries use the table names Django creates
  (`shop_order`, `shop_customer`).
- `shop/migrations/0001_initial.py` creates the models and both indexes.
- `shop/migrations/0002_remove_customer_index.py` removes `orders_customer_idx`.
- `project/settings.py` reads `DATABASE_URL`, which PlanTwin sets to the twin.
