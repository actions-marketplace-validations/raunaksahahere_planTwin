// Package rules turns plan changes into findings. Shape rules come first; the
// cost rule is informational (ADR-0008). Every rule has a documented threshold
// in Config and a plain-language explanation.
package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/raunaksahahere/plantwin/internal/diff"
	"github.com/raunaksahahere/plantwin/internal/plan"
	"github.com/raunaksahahere/plantwin/internal/shape"
)

// Severity orders findings. Error and warning fail the check by default.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Info    Severity = "info"
)

// Config holds the thresholds. Defaults are chosen to keep noise low; see ADR-0008.
type Config struct {
	LargeRows        float64            // a relation at or above this many rows is "large" (lost_index severity)
	SeqScanRows      float64            // a new sequential scan above this many estimated rows is reported
	JoinRows         float64            // a join-method change above this many estimated output rows is reported
	WorkMemBytes     float64            // Sort and Hash nodes estimated above this many bytes are reported
	CostRatio        float64            // head/base root cost ratio at or above this is reported as info
	MinCost          float64            // the cost rule ignores plans cheaper than this on head
	IndexSelectivity float64            // a scan returning at least this fraction of its table is not "selective" (ADR-0008)
	TableRows        map[string]float64 // table row counts from the stats bundle; nil or missing = unknown
	FailOn           Severity
	EstimatedRelns   map[string]bool // relations whose stats were estimated, not imported
}

// DefaultConfig returns the default thresholds.
func DefaultConfig() Config {
	return Config{
		LargeRows:        100_000,
		SeqScanRows:      10_000,
		JoinRows:         10_000,
		WorkMemBytes:     4 * 1024 * 1024, // PostgreSQL default work_mem
		CostRatio:        2.0,
		MinCost:          1000,
		IndexSelectivity: 0.10,
		FailOn:           Warning,
	}
}

// Finding is one reported problem.
type Finding struct {
	Rule      string   `json:"rule"`
	Severity  Severity `json:"severity"`
	Query     string   `json:"query"`
	Relation  string   `json:"relation,omitempty"`
	Message   string   `json:"message"`
	Why       string   `json:"why"`
	Fix       string   `json:"fix"`
	Estimated bool     `json:"estimated"`
	Old       string   `json:"old_plan,omitempty"`
	New       string   `json:"new_plan,omitempty"`
}

// Input is one query's base and head plans. If the head query failed to plan
// (for example after a column type change), HeadErr holds the error text and Head is ignored.
type Input struct {
	Query   string
	Base    plan.Node
	Head    plan.Node
	HeadErr string
}

// Analyse returns the findings for one query, sorted by severity, rule and relation.
func Analyse(in Input, cfg Config) []Finding {
	if in.HeadErr != "" {
		return []Finding{{Rule: "query_error", Severity: Error, Query: in.Query,
			Message: "The query no longer plans on the head revision: " + in.HeadErr,
			Why:     "The application would fail at runtime with this query.",
			Fix:     "Fix the migration or update the query to match the new column type.",
			Old:     shape.FromPlan(&in.Base).String()}}
	}
	baseShape := shape.FromPlan(&in.Base)
	headShape := shape.FromPlan(&in.Head)
	changes := diff.Compare(baseShape, headShape)

	headRows := rowsBySig(&in.Head)
	baseSigs := sigSet(&in.Base)
	var out []Finding
	seen := map[string]bool{}
	add := func(f Finding) {
		key := f.Rule + "|" + f.Relation
		if seen[key] {
			return
		}
		seen[key] = true
		f.Query = in.Query
		f.Estimated = f.Relation != "" && cfg.EstimatedRelns[f.Relation]
		f.Old = baseShape.String()
		f.New = headShape.String()
		out = append(out, f)
	}

	baseRows := rowsBySig(&in.Base)
	for _, c := range changes {
		// Every applicable rule runs for every change. shape_changed is the fallback when none applied.
		handled := false
		added := c.Kind == diff.Replaced || c.Kind == diff.Added

		if added && c.New != nil && hasSeqScan(c.New) {
			handled = true
			for _, sc := range seqScansIn(c.New) {
				tRows, known := cfg.TableRows[sc.Relation]
				outRows := headRows[sc.Sig]
				if known && tRows > 0 && outRows/tRows >= cfg.IndexSelectivity {
					// Reads a large share of the table: a sequential scan is the right plan, so this is not a regression.
					continue
				}
				if c.Kind == diff.Replaced && indexedRelations(c.Old)[sc.Relation] {
					oldRows := baseRows[c.Old.Sig]
					if known && tRows > 0 && oldRows/tRows >= cfg.IndexSelectivity {
						continue // the index was not selective, so losing it is not a regression
					}
					size := tRows
					if !known {
						size = outRows
					}
					sev := Warning
					if size >= cfg.LargeRows {
						sev = Error
					}
					add(Finding{Rule: "lost_index", Severity: sev, Relation: sc.Relation,
						Message: fmt.Sprintf("Index use lost on %s: a selective index is now unused, and the table is read by a sequential scan (about %s rows).", sc.Relation, human(size)),
						Why:     "Queries on this table used a selective index. Without it, PostgreSQL reads every row of the table.",
						Fix:     "Recreate the index, or add it back in this migration. To accept the change, run `plantwin accept`."})
					continue
				}
				size := tRows
				if !known {
					size = outRows
				}
				if size >= cfg.SeqScanRows {
					add(Finding{Rule: "seq_scan_large", Severity: Warning, Relation: sc.Relation,
						Message: fmt.Sprintf("New sequential scan on %s (table about %s rows, %s returned).", sc.Relation, human(size), human(outRows)),
						Why:     "A sequential scan over a large table is expensive at production size.",
						Fix:     "Add an index on the filter columns, or confirm the table is small in production."})
				}
			}
		}

		if c.Kind == diff.Replaced && isJoin(c.Old) && isJoin(c.New) && sameRelations(c.Old, c.New) {
			handled = true
			rows := headRows[c.New.Sig]
			if rows >= cfg.JoinRows {
				add(Finding{Rule: "join_method", Severity: Warning, Relation: c.New.Relation,
					Message: fmt.Sprintf("Join method changed from %s to %s (about %s output rows).", c.Old.Label, c.New.Label, human(rows)),
					Why:     "A different join method can change the cost of a large join by orders of magnitude.",
					Fix:     "Check the join keys and indexes. If the estimates changed unexpectedly, review the statistics."})
			}
		}

		if added && c.New != nil && (c.New.Label == "Sort" || c.New.Label == "Hash") && !baseSigs[c.New.Sig] {
			handled = true
			if bytes := headBytes(&in.Head, c.New.Sig); bytes > cfg.WorkMemBytes {
				add(Finding{Rule: "sort_hash_work_mem", Severity: Warning,
					Message: fmt.Sprintf("New %s is estimated at %s, above work_mem (%s): it may spill to disk.", c.New.Label, humanBytes(bytes), humanBytes(cfg.WorkMemBytes)),
					Why:     "Operators that exceed work_mem write temporary files, which is slow at production size.",
					Fix:     "Add an index that provides the sort order, or raise work_mem for this query."})
			}
		}

		if !handled {
			add(Finding{Rule: "shape_changed", Severity: Info, Relation: c.Relation,
				Message: "Plan shape changed: " + c.String(),
				Why:     "The plan now does its work in a different order or with different operators.",
				Fix:     "Compare the two plans below and decide whether the change is expected."})
		}
	}

	if cfg.CostRatio > 0 && in.Base.TotalCost > 0 && in.Head.TotalCost >= cfg.MinCost {
		ratio := in.Head.TotalCost / in.Base.TotalCost
		if ratio >= cfg.CostRatio {
			add(Finding{Rule: "cost_ratio", Severity: Info,
				Message: fmt.Sprintf("Estimated total cost rose %.1fx (%.0f to %.0f).", ratio, in.Base.TotalCost, in.Head.TotalCost),
				Why:     "Cost is an estimate. A large rise is a signal to look, not proof of a slowdown.",
				Fix:     "Compare the two plans below and verify on a representative workload."})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if rank(out[i].Severity) != rank(out[j].Severity) {
			return rank(out[i].Severity) < rank(out[j].Severity)
		}
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		return out[i].Relation < out[j].Relation
	})
	return out
}

// Fails reports whether any finding meets the failure threshold.
func Fails(findings []Finding, failOn Severity) bool {
	for _, f := range findings {
		if rank(f.Severity) <= rank(failOn) {
			return true
		}
	}
	return false
}

func rank(s Severity) int {
	switch s {
	case Error:
		return 0
	case Warning:
		return 1
	default:
		return 2
	}
}

// rowsBySig maps each subtree signature to the estimated rows of its root node.
func rowsBySig(root *plan.Node) map[string]float64 {
	out := map[string]float64{}
	root.Walk(func(n *plan.Node) {
		if n.NodeType == "Hash" {
			return
		}
		s := shape.FromPlan(n)
		if n.PlanRows > out[s.Sig] {
			out[s.Sig] = n.PlanRows
		}
	})
	return out
}

// headBytes returns the largest estimated rows*width for a node with the given signature.
func headBytes(root *plan.Node, sig string) float64 {
	var best float64
	root.Walk(func(n *plan.Node) {
		if n.NodeType == "Hash" {
			return
		}
		if shape.FromPlan(n).Sig == sig && n.PlanRows*n.PlanWidth > best {
			best = n.PlanRows * n.PlanWidth
		}
	})
	return best
}

func sigSet(root *plan.Node) map[string]bool {
	out := map[string]bool{}
	root.Walk(func(n *plan.Node) {
		if n.NodeType != "Hash" {
			out[shape.FromPlan(n).Sig] = true
		}
	})
	return out
}

// hasSeqScan reports whether a sequential scan appears anywhere in the subtree.
func hasSeqScan(s *shape.Shape) bool {
	return len(seqScansIn(s)) > 0
}

// seqScansIn returns every sequential scan in the subtree, in plan order.
func seqScansIn(s *shape.Shape) []*shape.Shape {
	var out []*shape.Shape
	var visit func(*shape.Shape)
	visit = func(x *shape.Shape) {
		if isSeqScan(x) && x.Relation != "" {
			out = append(out, x)
		}
		for _, k := range x.Kids {
			visit(k)
		}
	}
	visit(s)
	return out
}

// indexedRelations returns the relations read through an index anywhere in the subtree.
// A bitmap heap scan has no index of its own: its index is a child, so the relation counts when any descendant uses an index.
func indexedRelations(s *shape.Shape) map[string]bool {
	out := map[string]bool{}
	var visit func(*shape.Shape) bool
	visit = func(x *shape.Shape) bool {
		uses := x.Index != ""
		for _, k := range x.Kids {
			if visit(k) {
				uses = true
			}
		}
		if uses && x.Relation != "" {
			out[x.Relation] = true
		}
		return uses
	}
	visit(s)
	return out
}

func isSeqScan(s *shape.Shape) bool {
	return strings.HasPrefix(s.Label, "Seq Scan") || strings.HasPrefix(s.Label, "Parallel Seq Scan")
}

func isJoin(s *shape.Shape) bool {
	return s != nil && (strings.HasPrefix(s.Label, "Hash Join") || strings.HasPrefix(s.Label, "Merge Join") || strings.HasPrefix(s.Label, "Nested Loop"))
}

// sameRelations is true when both shapes read the same set of relations.
func sameRelations(a, b *shape.Shape) bool {
	return strings.Join(relations(a), ",") == strings.Join(relations(b), ",")
}

func relations(s *shape.Shape) []string {
	seen := map[string]bool{}
	var out []string
	var visit func(*shape.Shape)
	visit = func(x *shape.Shape) {
		if x.Relation != "" && !seen[x.Relation] {
			seen[x.Relation] = true
			out = append(out, x.Relation)
		}
		for _, k := range x.Kids {
			visit(k)
		}
	}
	visit(s)
	sort.Strings(out)
	return out
}

func human(rows float64) string {
	switch {
	case rows >= 1e6:
		return fmt.Sprintf("%.1fM", rows/1e6)
	case rows >= 1e3:
		return fmt.Sprintf("%.0fK", rows/1e3)
	default:
		return fmt.Sprintf("%.0f", rows)
	}
}

func humanBytes(b float64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", b/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.0f KB", b/1024)
	default:
		return fmt.Sprintf("%.0f B", b)
	}
}
