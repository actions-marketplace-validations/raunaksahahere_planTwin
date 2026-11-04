// Package report renders findings as Markdown (PR comment), JSON, or SARIF.
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/raunaksahahere/plantwin/internal/plan"
	"github.com/raunaksahahere/plantwin/internal/rules"
)

// SchemaVersion is the version of the JSON report format. Bump it on any incompatible change.
const SchemaVersion = 1

// QueryResult is one query's findings with its plans, for side-by-side display.
type QueryResult struct {
	Name     string          `json:"name"`
	BasePlan *plan.Node      `json:"-"`
	HeadPlan *plan.Node      `json:"-"`
	BaseErr  string          `json:"base_error,omitempty"`
	HeadErr  string          `json:"head_error,omitempty"`
	Findings []rules.Finding `json:"findings"`
}

// Run is everything a report needs.
type Run struct {
	Base, Head string // git refs or labels
	Queries    []QueryResult
	Timing     []Phase  // durations per phase, shown to the user
	Estimated  []string // relations whose stats are estimated
	Warnings   []string // export and import warnings
	FailOn     rules.Severity
}

// Phase is one timed step.
type Phase struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
}

// Ms builds a Phase from a measured duration.
func Ms(name string, d time.Duration) Phase {
	return Phase{Name: name, DurationMS: d.Milliseconds()}
}

// Failed reports whether the run should fail the check.
func (r Run) Failed() bool {
	for _, q := range r.Queries {
		if rules.Fails(q.Findings, r.FailOn) {
			return true
		}
	}
	return false
}

// Counts returns the number of findings per severity.
func (r Run) Counts() map[rules.Severity]int {
	out := map[rules.Severity]int{}
	for _, q := range r.Queries {
		for _, f := range q.Findings {
			out[f.Severity]++
		}
	}
	return out
}

// Markdown renders a PR-comment-style report with side-by-side plans for each finding.
func Markdown(r Run) string {
	var b strings.Builder
	c := r.Counts()
	fmt.Fprintf(&b, "## PlanTwin: %s → %s\n\n", r.Base, r.Head)
	if r.Failed() {
		fmt.Fprintf(&b, "**Result: failing.** %d error(s), %d warning(s), %d info.\n\n", c[rules.Error], c[rules.Warning], c[rules.Info])
	} else {
		fmt.Fprintf(&b, "**Result: passing.** %d error(s), %d warning(s), %d info.\n\n", c[rules.Error], c[rules.Warning], c[rules.Info])
	}
	if len(r.Estimated) > 0 {
		fmt.Fprintf(&b, "> **Estimated:** statistics for %s were estimated, not imported. Findings on them are labelled.\n\n", strings.Join(r.Estimated, ", "))
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "> **Note:** %s\n", w)
	}
	if len(r.Warnings) > 0 {
		b.WriteString("\n")
	}

	shown := false
	for _, q := range r.Queries {
		if len(q.Findings) == 0 && q.HeadErr == "" {
			continue
		}
		shown = true
		fmt.Fprintf(&b, "### `%s`\n\n", q.Name)
		for _, f := range q.Findings {
			label := strings.ToUpper(string(f.Severity))
			est := ""
			if f.Estimated {
				est = " *(estimated)*"
			}
			fmt.Fprintf(&b, "- **%s** `%s`%s: %s\n", label, f.Rule, est, f.Message)
			fmt.Fprintf(&b, "  - Why this matters: %s\n", f.Why)
			fmt.Fprintf(&b, "  - Suggested fix: %s\n", f.Fix)
		}
		if q.HeadErr != "" {
			fmt.Fprintf(&b, "- **ERROR** `query_error`: the query no longer plans on %s: %s\n", r.Head, q.HeadErr)
		}
		if q.BasePlan != nil && q.HeadPlan != nil && len(q.Findings) > 0 {
			b.WriteString("\n<details><summary>Plans, side by side</summary>\n\n")
			b.WriteString("| " + r.Base + " | " + r.Head + " |\n|---|---|\n")
			bl := strings.Split(strings.TrimRight(TreeText(q.BasePlan), "\n"), "\n")
			hl := strings.Split(strings.TrimRight(TreeText(q.HeadPlan), "\n"), "\n")
			for i := 0; i < max(len(bl), len(hl)); i++ {
				b.WriteString("| `" + cell(bl, i) + "` | `" + cell(hl, i) + "` |\n")
			}
			b.WriteString("\n</details>\n\n")
		}
	}
	if !shown {
		b.WriteString("No plan changes detected for the configured queries.\n\n")
	}
	if len(r.Timing) > 0 {
		var parts []string
		for _, p := range r.Timing {
			parts = append(parts, fmt.Sprintf("%s %s", p.Name, (time.Duration(p.DurationMS)*time.Millisecond).String()))
		}
		fmt.Fprintf(&b, "<sub>Timing: %s</sub>\n", strings.Join(parts, " · "))
	}
	return b.String()
}

func cell(lines []string, i int) string {
	if i < len(lines) {
		return strings.ReplaceAll(lines[i], "|", "\\|")
	}
	return ""
}

// TreeText renders a plan as an indented tree with rows and cost, one node per line.
func TreeText(n *plan.Node) string {
	var b strings.Builder
	var rec func(*plan.Node, int)
	rec = func(x *plan.Node, depth int) {
		name := x.NodeType
		if x.JoinType != "" && x.JoinType != "Inner" {
			name += " (" + x.JoinType + ")"
		}
		if x.RelationName != "" {
			name += " on " + x.RelationName
		}
		if x.IndexName != "" {
			name += " using " + x.IndexName
		}
		fmt.Fprintf(&b, "%s%s  rows=%.0f cost=%.0f\n", strings.Repeat("  ", depth), name, x.PlanRows, x.TotalCost)
		for i := range x.Plans {
			rec(&x.Plans[i], depth+1)
		}
	}
	rec(n, 0)
	return b.String()
}

// JSON renders the machine-readable report. It carries schema_version so consumers can check compatibility.
func JSON(r Run) ([]byte, error) {
	type out struct {
		SchemaVersion int           `json:"schema_version"`
		Base          string        `json:"base"`
		Head          string        `json:"head"`
		Failed        bool          `json:"failed"`
		Estimated     []string      `json:"estimated_relations"`
		Warnings      []string      `json:"warnings"`
		Timing        []Phase       `json:"timing"`
		Queries       []QueryResult `json:"queries"`
	}
	o := out{SchemaVersion: SchemaVersion, Base: r.Base, Head: r.Head, Failed: r.Failed(),
		Estimated: r.Estimated, Warnings: r.Warnings, Timing: r.Timing, Queries: r.Queries}
	if o.Estimated == nil {
		o.Estimated = []string{}
	}
	if o.Warnings == nil {
		o.Warnings = []string{}
	}
	return json.MarshalIndent(o, "", "  ")
}

// SARIF renders a SARIF 2.1.0 log so findings show as code-scanning alerts. Levels: error, warning, note.
func SARIF(r Run, toolVersion string) ([]byte, error) {
	type msg struct {
		Text string `json:"text"`
	}
	type rule struct {
		ID               string `json:"id"`
		ShortDescription msg    `json:"shortDescription"`
		HelpURI          string `json:"helpUri,omitempty"`
	}
	type location struct {
		LogicalLocations []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"logicalLocations"`
	}
	type result struct {
		RuleID    string     `json:"ruleId"`
		Level     string     `json:"level"`
		Message   msg        `json:"message"`
		Locations []location `json:"locations"`
	}

	ruleSeen := map[string]bool{}
	var ruleList []rule
	var results []result
	for _, q := range r.Queries {
		for _, f := range q.Findings {
			if !ruleSeen[f.Rule] {
				ruleSeen[f.Rule] = true
				ruleList = append(ruleList, rule{ID: f.Rule, ShortDescription: msg{Text: f.Why}})
			}
			loc := location{}
			loc.LogicalLocations = append(loc.LogicalLocations, struct {
				Name string `json:"name"`
				Kind string `json:"kind"`
			}{Name: q.Name, Kind: "query"})
			results = append(results, result{
				RuleID:    f.Rule,
				Level:     sarifLevel(f.Severity),
				Message:   msg{Text: f.Message + " Fix: " + f.Fix},
				Locations: []location{loc},
			})
		}
	}
	sort.SliceStable(ruleList, func(i, j int) bool { return ruleList[i].ID < ruleList[j].ID })
	log := map[string]any{
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"version": "2.1.0",
		"runs": []any{map[string]any{
			"tool": map[string]any{"driver": map[string]any{
				"name": "plantwin", "version": toolVersion, "rules": ruleList,
			}},
			"results": results,
		}},
	}
	return json.MarshalIndent(log, "", "  ")
}

func sarifLevel(s rules.Severity) string {
	switch s {
	case rules.Error:
		return "error"
	case rules.Warning:
		return "warning"
	default:
		return "note"
	}
}
