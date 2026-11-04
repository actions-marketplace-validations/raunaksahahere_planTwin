package twin

import _ "embed"

// EstimateSQL is the estimator for indexes that have no imported statistics (ADR-0010).
// The same file is used by the corpus benchmark, so the two cannot drift apart.
//
//go:embed estimate_new_indexes.sql
var EstimateSQL string
