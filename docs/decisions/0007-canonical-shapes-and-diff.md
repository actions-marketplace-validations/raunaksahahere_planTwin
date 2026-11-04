# ADR-0007: Canonical shape trees and the differ

**Context.** Two plans can differ in estimates (rows, cost) without doing different work. The review signal is
the change in *what the plan does*: operators, relations, indexes and join order. The estimates are the secondary
signal (cost), and they are noisy.

**Decision.**
- A canonical *shape* keeps: node type with a behaviour qualifier (join type for joins, strategy for Aggregate,
  CTE name for CTE Scan), the relation and index names, and the child order. It drops rows, widths and costs.
- A `Hash` node is transparent: a hash join's inner side is the `Hash`'s child. No other node is dropped.
  Property test: adding Hash wrappers above inner children never changes a shape (200 random trees).
- Each subtree has a signature (SHA-256 truncated to 64 bits over the canonical encoding). Equal signatures mean
  equal subtrees.
- The differ aligns children with a longest common subsequence over child signatures. Matched children are equal.
  Unmatched children are paired in order and compared recursively. Extra children are added or removed.
  Otherwise a node with a different label, relation or index is reported once, as *replaced*, with its whole subtree.

**Why.** LCS alignment means one inserted child does not make every later sibling look changed (a unit test
checks this). Reporting a replaced node once keeps the output short, which is part of the noise requirement.

**Consequences.**
- A change inside a replaced subtree is only visible through the rules that search that subtree (ADR-0008).
- 64-bit signatures: collision probability is negligible for plan-sized trees, but the signature is not a
  cryptographic commitment. It is used only for equality within one run.
