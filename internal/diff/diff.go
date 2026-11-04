// Package diff compares two canonical shape trees and reports the minimal set of
// changes between them (ADR-0007).
//
// Algorithm (top-down):
//   - Equal signatures: no change.
//   - Same label, relation and index: align the children with a longest common
//     subsequence over child signatures. Matched children are equal and are
//     skipped. The unmatched children left and right are paired in order and
//     compared recursively. Any extra children are reported as added or removed.
//   - Different label, relation or index: the node was replaced. The whole subtree
//     is reported once, not node by node.
//
// The LCS step means a single inserted child does not make every later sibling look changed.
package diff

import (
	"fmt"

	"github.com/raunaksahahere/plantwin/internal/shape"
)

// Kind of change.
type Kind string

const (
	Replaced Kind = "replaced" // the node at this position is a different operator or object
	Added    Kind = "added"    // a child exists only in the head plan
	Removed  Kind = "removed"  // a child exists only in the base plan
)

// Change is one difference between base and head.
type Change struct {
	Kind     Kind
	Path     string // position in the tree, for example "root/0/1"
	Old      *shape.Shape
	New      *shape.Shape
	Relation string // relation the change is about, if any
}

// String is a one-line description, used in reports and golden files.
func (c Change) String() string {
	switch c.Kind {
	case Replaced:
		return fmt.Sprintf("replaced at %s: %s -> %s", c.Path, short(c.Old), short(c.New))
	case Added:
		return fmt.Sprintf("added at %s: %s", c.Path, short(c.New))
	default:
		return fmt.Sprintf("removed at %s: %s", c.Path, short(c.Old))
	}
}

func short(s *shape.Shape) string {
	if s == nil {
		return "(none)"
	}
	if s.Relation != "" {
		return fmt.Sprintf("%s on %s", s.Label, s.Relation)
	}
	return s.Label
}

// Compare returns the changes that turn base into head. An empty result means identical shapes.
func Compare(base, head *shape.Shape) []Change {
	var out []Change
	walk(base, head, "root", &out)
	return out
}

func walk(a, b *shape.Shape, path string, out *[]Change) {
	if a.Sig == b.Sig {
		return
	}
	if a.Label != b.Label || a.Relation != b.Relation || a.Index != b.Index {
		*out = append(*out, Change{Kind: Replaced, Path: path, Old: a, New: b, Relation: firstNonEmpty(a.Relation, b.Relation)})
		return
	}

	matchA, matchB := lcs(a.Kids, b.Kids)
	var unA, unB []int
	for i := range a.Kids {
		if !matchA[i] {
			unA = append(unA, i)
		}
	}
	for j := range b.Kids {
		if !matchB[j] {
			unB = append(unB, j)
		}
	}
	paired := min(len(unA), len(unB))
	for k := 0; k < paired; k++ {
		i, j := unA[k], unB[k]
		walk(a.Kids[i], b.Kids[j], fmt.Sprintf("%s/%d", path, j), out)
	}
	for k := paired; k < len(unA); k++ {
		i := unA[k]
		*out = append(*out, Change{Kind: Removed, Path: fmt.Sprintf("%s/%d", path, i), Old: a.Kids[i], Relation: a.Kids[i].Relation})
	}
	for k := paired; k < len(unB); k++ {
		j := unB[k]
		*out = append(*out, Change{Kind: Added, Path: fmt.Sprintf("%s/%d", path, j), New: b.Kids[j], Relation: b.Kids[j].Relation})
	}
}

// lcs marks the children that appear, in order, in both lists (by signature).
func lcs(x, y []*shape.Shape) (matchX, matchY []bool) {
	n, m := len(x), len(y)
	matchX, matchY = make([]bool, n), make([]bool, m)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i].Sig == y[j].Sig {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		if x[i].Sig == y[j].Sig {
			matchX[i], matchY[j] = true, true
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			i++
		} else {
			j++
		}
	}
	return matchX, matchY
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
