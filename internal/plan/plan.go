// Package plan parses PostgreSQL EXPLAIN (FORMAT JSON) output into a typed tree.
//
// Only the fields PlanTwin uses are modelled. Unknown fields are ignored, so the
// parser keeps working across PostgreSQL minor versions.
package plan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Node is one operator in an EXPLAIN plan tree.
type Node struct {
	NodeType           string  `json:"Node Type"`
	RelationName       string  `json:"Relation Name,omitempty"`
	Schema             string  `json:"Schema,omitempty"`
	Alias              string  `json:"Alias,omitempty"`
	IndexName          string  `json:"Index Name,omitempty"`
	JoinType           string  `json:"Join Type,omitempty"`
	CTEName            string  `json:"CTE Name,omitempty"`
	Strategy           string  `json:"Strategy,omitempty"`
	ParentRelationship string  `json:"Parent Relationship,omitempty"`
	ParallelAware      bool    `json:"Parallel Aware,omitempty"`
	StartupCost        float64 `json:"Startup Cost"`
	TotalCost          float64 `json:"Total Cost"`
	PlanRows           float64 `json:"Plan Rows"`
	PlanWidth          float64 `json:"Plan Width"`
	Filter             string  `json:"Filter,omitempty"`
	IndexCond          string  `json:"Index Cond,omitempty"`
	Plans              []Node  `json:"Plans,omitempty"`
}

// explainDoc is the top-level shape of EXPLAIN (FORMAT JSON): an array of one object.
type explainDoc struct {
	Plan Node `json:"Plan"`
}

// Parse reads a plan. It accepts three forms:
//   - the full EXPLAIN (FORMAT JSON) output: an array with one {"Plan": ...} object;
//   - a single {"Plan": ...} object;
//   - a bare plan node, which is how the corpus recorder stores plans.
func Parse(data []byte) (Node, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return Node{}, errors.New("empty EXPLAIN output")
	}
	switch data[0] {
	case '[':
		var docs []explainDoc
		if err := json.Unmarshal(data, &docs); err != nil {
			return Node{}, fmt.Errorf("EXPLAIN output is not valid JSON: %w", err)
		}
		if len(docs) != 1 {
			return Node{}, fmt.Errorf("expected one plan in EXPLAIN output, got %d", len(docs))
		}
		return checked(docs[0].Plan)
	case '{':
		var doc explainDoc
		if err := json.Unmarshal(data, &doc); err == nil && doc.Plan.NodeType != "" {
			return checked(doc.Plan)
		}
		var node Node
		if err := json.Unmarshal(data, &node); err != nil {
			return Node{}, fmt.Errorf("plan is not valid JSON: %w", err)
		}
		return checked(node)
	}
	return Node{}, errors.New("plan is neither a JSON array nor an object")
}

func checked(n Node) (Node, error) {
	if n.NodeType == "" {
		return Node{}, errors.New("plan has no Node Type")
	}
	return n, nil
}

// Walk calls fn for every node in depth-first order (parent before children).
func (n *Node) Walk(fn func(*Node)) {
	fn(n)
	for i := range n.Plans {
		n.Plans[i].Walk(fn)
	}
}

// Relations returns every relation name scanned in the subtree, in plan order, without duplicates.
func (n *Node) Relations() []string {
	seen := map[string]bool{}
	var out []string
	n.Walk(func(x *Node) {
		if x.RelationName != "" && !seen[x.RelationName] {
			seen[x.RelationName] = true
			out = append(out, x.RelationName)
		}
	})
	return out
}
