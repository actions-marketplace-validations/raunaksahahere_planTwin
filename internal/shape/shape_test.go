package shape

import (
	"math/rand"
	"testing"

	"github.com/raunaksahahere/plantwin/internal/plan"
)

func scan(rel string) plan.Node {
	return plan.Node{NodeType: "Seq Scan", RelationName: rel, PlanRows: 10, PlanWidth: 8, TotalCost: 5}
}

func idx(rel, index string) plan.Node {
	return plan.Node{NodeType: "Index Scan", RelationName: rel, IndexName: index, PlanRows: 1, PlanWidth: 8, TotalCost: 1}
}

func TestCanonicalStringTable(t *testing.T) {
	cases := []struct {
		name string
		node plan.Node
		want string
	}{
		{"seq scan", scan("orders"), "Seq Scan on orders"},
		{"index scan", idx("orders", "orders_pkey"), "Index Scan orders_pkey on orders"},
		{
			"hash join hides Hash wrapper",
			plan.Node{NodeType: "Hash Join", JoinType: "Inner", Plans: []plan.Node{
				scan("a"),
				{NodeType: "Hash", Plans: []plan.Node{scan("b")}},
			}},
			"Hash Join(Seq Scan on a, Seq Scan on b)",
		},
		{
			"outer join keeps its type",
			plan.Node{NodeType: "Hash Join", JoinType: "Left", Plans: []plan.Node{scan("a"), scan("b")}},
			"Hash Join (Left)(Seq Scan on a, Seq Scan on b)",
		},
		{
			"aggregate keeps its strategy",
			plan.Node{NodeType: "Aggregate", Strategy: "Hashed", Plans: []plan.Node{scan("a")}},
			"Aggregate (Hashed)(Seq Scan on a)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FromPlan(&c.node).String()
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestEstimatesDoNotChangeShape(t *testing.T) {
	a := plan.Node{NodeType: "Seq Scan", RelationName: "orders", PlanRows: 10, TotalCost: 5}
	b := plan.Node{NodeType: "Seq Scan", RelationName: "orders", PlanRows: 9999, TotalCost: 5000}
	if !Equal(FromPlan(&a), FromPlan(&b)) {
		t.Fatal("same operator on same relation must have the same signature regardless of estimates")
	}
}

func TestChildOrderMatters(t *testing.T) {
	ab := plan.Node{NodeType: "Nested Loop", JoinType: "Inner", Plans: []plan.Node{scan("a"), scan("b")}}
	ba := plan.Node{NodeType: "Nested Loop", JoinType: "Inner", Plans: []plan.Node{scan("b"), scan("a")}}
	if Equal(FromPlan(&ab), FromPlan(&ba)) {
		t.Fatal("outer and inner sides of a join must not be interchangeable")
	}
}

// Property: wrapping any join's inner child in a Hash node never changes the shape.
func TestHashWrapperIsTransparent(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		tree := randomTree(r, 3)
		wrapped := wrapHashes(tree)
		a, b := FromPlan(&tree), FromPlan(&wrapped)
		if !Equal(a, b) {
			t.Fatalf("iteration %d: shape changed after adding Hash wrappers:\n%s\n%s", i, a, b)
		}
	}
}

func randomTree(r *rand.Rand, depth int) plan.Node {
	if depth == 0 || r.Intn(3) == 0 {
		return scan(string(rune('a' + r.Intn(5))))
	}
	kids := []plan.Node{randomTree(r, depth-1), randomTree(r, depth-1)}
	return plan.Node{NodeType: "Hash Join", JoinType: "Inner", Plans: kids}
}

// wrapHashes puts a Hash node above the inner (second) child of every join.
func wrapHashes(n plan.Node) plan.Node {
	out := n
	out.Plans = nil
	for i, c := range n.Plans {
		c = wrapHashes(c)
		if i == 1 {
			c = plan.Node{NodeType: "Hash", Plans: []plan.Node{c}}
		}
		out.Plans = append(out.Plans, c)
	}
	return out
}
