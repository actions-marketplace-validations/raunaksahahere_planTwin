# Architecture decision records

Each non-trivial decision taken for PlanTwin is recorded here: the context, the decision, and the consequences.
Decisions that were taken on the owner's behalf during the autonomous build are marked as such in their text.

| ADR | Title |
|---|---|
| [0001](0001-ci-only-guard.md) | CI-only guard for the planner extension |
| [0002](0002-btree-height-estimate.md) | Estimating B-tree height from imported sizes |
| [0003](0003-hook-scope-and-loading.md) | Hook scope and per-session loading |
| [0004](0004-extended-statistics.md) | Extended statistics: detect and warn, do not restore |
| [0005](0005-benchmark-sources-and-config.md) | Benchmark data sources, containers and autovacuum |
| [0006](0006-go-toolchain-in-docker.md) | Go toolchain runs in disposable containers |
| [0007](0007-canonical-shapes-and-diff.md) | Canonical shape trees and the LCS differ |
| [0008](0008-rules-and-selectivity.md) | Rules, thresholds and the selectivity cut |
| [0009](0009-dependencies-and-docker-cli.md) | Dependencies, and driving Docker through the CLI |
| [0010](0010-estimate-new-indexes.md) | Estimating statistics for indexes the PR creates |
| [0011](0011-bundle-format-redaction-and-signing.md) | Stats bundle format, redaction and signing |
| [0012](0012-schema-cache.md) | The schema cache stores the schema, not the container |
| [0013](0013-ci-and-release-pipeline.md) | CI, release and published-artifact verification |
