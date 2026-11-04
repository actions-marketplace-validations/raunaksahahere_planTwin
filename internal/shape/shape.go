// Package shape turns a plan tree into a canonical "shape tree" that ignores
// estimates. Two plans with the same shape do the same work in the same order;
// only their row and cost numbers differ.
//
// Canonical rules (ADR-0007):
//   - A Hash node is transparent: the join's inner side is its child, so the
//     Hash wrapper is dropped. No other node type is dropped.
//   - The label of a node is its Node Type plus the qualifier that changes
//     behaviour: the join type for joins, the strategy for Aggregate, the CTE name for CTE Scan.
//   - Scans keep the relation and index names. Estimates (rows, width, cost) are dropped.
//   - Child order is kept, because the outer and inner sides of a join matter.
//   - Each subtree gets a signature: a SHA-256 over its canonical encoding. Equal
//     signatures mean equal subtrees, so comparing subtrees is O(1).
package shape

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/raunaksahahere/plantwin/internal/plan"
)

// Shape is one node of the canonical tree.
type Shape struct {
	Label    string   // Node Type plus qualifier, for example "Hash Join (Inner)"
	Relation string   // scanned relation, empty for non-scans
	Index    string   // index name for index scans, empty otherwise
	Kids     []*Shape // children, in plan order
	Sig      string   // subtree signature (16 hex chars)
}

// FromPlan canonicalises a plan tree.
func FromPlan(n *plan.Node) *Shape {
	if n.NodeType == "Hash" && len(n.Plans) == 1 {
		return FromPlan(&n.Plans[0])
	}
	s := &Shape{
		Label:    label(n),
		Relation: n.RelationName,
		Index:    n.IndexName,
	}
	for i := range n.Plans {
		child := FromPlan(&n.Plans[i])
		s.Kids = append(s.Kids, child)
	}
	s.Sig = signature(s)
	return s
}

func label(n *plan.Node) string {
	switch {
	case n.JoinType != "" && n.JoinType != "Inner":
		return fmt.Sprintf("%s (%s)", n.NodeType, n.JoinType)
	case n.NodeType == "Aggregate" && n.Strategy != "":
		return fmt.Sprintf("%s (%s)", n.NodeType, n.Strategy)
	case n.NodeType == "CTE Scan" && n.CTEName != "":
		return fmt.Sprintf("%s (%s)", n.NodeType, n.CTEName)
	default:
		return n.NodeType
	}
}

func signature(s *Shape) string {
	var b strings.Builder
	b.WriteString(s.Label)
	b.WriteByte('|')
	b.WriteString(s.Relation)
	b.WriteByte('|')
	b.WriteString(s.Index)
	b.WriteByte('|')
	for _, k := range s.Kids {
		b.WriteString(k.Sig)
		b.WriteByte(',')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}

// String renders the shape as one line, for logs and golden files.
// Example: "Hash Join(Nested Loop(Index Scan orders_pkey on orders, Seq Scan on customers), Seq Scan on items)".
func (s *Shape) String() string {
	var b strings.Builder
	b.WriteString(s.Label)
	if s.Index != "" {
		fmt.Fprintf(&b, " %s", s.Index)
	}
	if s.Relation != "" {
		fmt.Fprintf(&b, " on %s", s.Relation)
	}
	if len(s.Kids) > 0 {
		b.WriteByte('(')
		for i, k := range s.Kids {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(k.String())
		}
		b.WriteByte(')')
	}
	return b.String()
}

// Equal reports whether two shapes are identical.
func Equal(a, b *Shape) bool {
	return a.Sig == b.Sig
}
