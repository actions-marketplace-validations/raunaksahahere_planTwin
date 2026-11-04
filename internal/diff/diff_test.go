package diff

import (
	"testing"

	"github.com/raunaksahahere/plantwin/internal/plan"
	"github.com/raunaksahahere/plantwin/internal/shape"
)

func node(t string, rel, index string, kids ...plan.Node) plan.Node {
	return plan.Node{NodeType: t, RelationName: rel, IndexName: index, JoinType: "", Plans: kids, PlanRows: 1, PlanWidth: 1}
}

func sh(n plan.Node) *shape.Shape { return shape.FromPlan(&n) }

func TestCompareTable(t *testing.T) {
	cases := []struct {
		name       string
		base, head plan.Node
		want       []Kind
	}{
		{
			name: "identical",
			base: node("Seq Scan", "a", ""),
			head: node("Seq Scan", "a", ""),
			want: nil,
		},
		{
			name: "index replaced by seq scan on same relation",
			base: node("Index Scan", "orders", "orders_pkey"),
			head: node("Seq Scan", "orders", ""),
			want: []Kind{Replaced},
		},
		{
			name: "one extra child on the head side",
			base: node("Nested Loop", "", "", node("Seq Scan", "a", "")),
			head: node("Nested Loop", "", "", node("Seq Scan", "a", ""), node("Seq Scan", "b", "")),
			want: []Kind{Added},
		},
		{
			name: "inserted child does not cascade",
			base: node("Nested Loop", "", "", node("Seq Scan", "a", ""), node("Seq Scan", "c", "")),
			head: node("Nested Loop", "", "", node("Seq Scan", "x", ""), node("Seq Scan", "a", ""), node("Seq Scan", "c", "")),
			want: []Kind{Added},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Compare(sh(c.base), sh(c.head))
			if len(got) != len(c.want) {
				t.Fatalf("got %d changes %v, want %d", len(got), got, len(c.want))
			}
			for i, k := range c.want {
				if got[i].Kind != k {
					t.Fatalf("change %d: kind %s, want %s (%v)", i, got[i].Kind, k, got[i])
				}
			}
		})
	}
}

func TestCompareDescribesReplacement(t *testing.T) {
	got := Compare(sh(node("Index Scan", "orders", "orders_pkey")), sh(node("Seq Scan", "orders", "")))
	if len(got) != 1 || got[0].Relation != "orders" {
		t.Fatalf("unexpected changes: %v", got)
	}
	want := "replaced at root: Index Scan on orders -> Seq Scan on orders"
	if got[0].String() != want {
		t.Fatalf("String() = %q, want %q", got[0].String(), want)
	}
}
