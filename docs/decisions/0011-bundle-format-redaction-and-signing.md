# ADR-0011: Stats bundle format, redaction and signing

**Context.** The bundle is the only artifact that leaves production. It must not carry rows, must not leak literal
values that the team considers sensitive, and must not be altered in transit.

**Decision.**
- **Format.** One JSON document, `schema_version: 1`: server version, planner settings (an allow-list of names only),
  relation sizes (`relpages`, `reltuples`, `relallvisible`, `relallfrozen`) for tables, matviews and indexes, and one
  record per `pg_stats` row (with the column's type from `format_type`). It records the redaction that was applied.
- **Export reads.** Catalogs and `pg_stats` only. The export role needs `pg_read_all_stats` and nothing that reads
  table data (see `docs/privacy-and-redaction.md`).
- **Redaction modes**, per column (`keep`, `hash`, `drop-mcv`, `drop-histogram`):
  - `keep`: stored as is.
  - `hash`: each most-common value is replaced by a same-type surrogate. Integers become an integer from
    `HMAC-SHA256(salt, value) mod (2^31 - 1)`. Text becomes `h` + 12 hex digits. Other types drop the MCV list.
    The histogram is dropped, because surrogates cannot keep its order. Frequencies are kept, so the twin keeps
    the *shape* of the distribution without the literal values. Duplicate surrogates are dropped.
  - `drop-mcv`: the MCV list and its frequencies are removed.
  - `drop-histogram`: the histogram bounds are removed.
  - The default is `hash` (`redaction.default`).
- **The salt** is 32 random bytes generated per export and never written to the bundle. Without it a dictionary of
  candidate values cannot be matched against the surrogates.
- **Signing.** ed25519. `plantwin export --sign-key` writes `<bundle>.sig` (base64). `check` and `doctor` verify with
  `bundle.public_key` and refuse an unsigned or mismatched bundle (fail closed). Private keys are written with mode 0600.
- **Integration test.** A planted common value must not appear in a `hash` or `drop-mcv` bundle, and must appear in a
  `keep` bundle, so the test cannot pass vacuously (`internal/export/export_integration_test.go`).

**Fidelity cost.** `hash` keeps frequencies but not literals. A query whose literal is a common value gets the
planner's default estimate for an unknown value, not the real one. This cost is measured and reported in
`docs/privacy-and-redaction.md`.

**Consequences.** Extended statistics are not in the bundle (ADR-0004). Range histograms and element statistics are
not exported in v0.1 (they are dropped when present), so array and range columns are less faithful.
