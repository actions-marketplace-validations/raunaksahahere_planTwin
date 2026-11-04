# Example: plain SQL migrations with a planted regression

The same repository the built-in demo creates (`plantwin demo`). Migration `0002` drops `orders_customer_idx`, which
the `customer_lookup` query uses.

This example is verified: `make test-integration` runs the demo path, which exercises the same files.

## Try it in a git repository

```bash
git init && git add migrations/0001_schema.sql plantwin.yaml queries && git commit -m "base"
git add migrations/0002_drop_customer_index.sql && git commit -m "Drop the customer index"
plantwin keygen --priv plantwin.key --pub plantwin.pub        # keep plantwin.key out of git
PLANTWIN_DSN=postgres://plantwin_export:…@replica/app plantwin export --sign-key plantwin.key
plantwin check --base HEAD~1 --head HEAD
```

The export needs the role from `docs/privacy-and-redaction.md`. Without a replica, use `plantwin demo`, which builds
a synthetic one.
