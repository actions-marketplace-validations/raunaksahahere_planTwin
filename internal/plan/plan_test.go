package plan

import "testing"

const sampleSeqScan = `[{"Plan": {"Node Type": "Seq Scan", "Relation Name": "orders", "Alias": "orders",
  "Startup Cost": 0.0, "Total Cost": 1693.0, "Plan Rows": 9, "Plan Width": 8, "Filter": "(status = 'open')"}}]`

func TestParseSeqScan(t *testing.T) {
	n, err := Parse([]byte(sampleSeqScan))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.NodeType != "Seq Scan" || n.RelationName != "orders" || n.PlanRows != 9 || n.TotalCost != 1693 {
		t.Fatalf("unexpected node: %+v", n)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"empty":     "",
		"not json":  "EXPLAIN (oops",
		"no plan":   `[{"Other": 1}]`,
		"two plans": `[{"Plan": {"Node Type": "Result"}}, {"Plan": {"Node Type": "Result"}}]`,
	}
	for name, in := range cases {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseAcceptsAllStoredForms(t *testing.T) {
	forms := map[string]string{
		"array":  `[{"Plan": {"Node Type": "Result", "Total Cost": 1, "Plan Rows": 1, "Plan Width": 1}}]`,
		"object": `{"Plan": {"Node Type": "Result", "Total Cost": 1, "Plan Rows": 1, "Plan Width": 1}}`,
		"node":   `{"Node Type": "Result", "Total Cost": 1, "Plan Rows": 1, "Plan Width": 1}`,
	}
	for name, in := range forms {
		n, err := Parse([]byte(in))
		if err != nil || n.NodeType != "Result" {
			t.Fatalf("%s form: %v %+v", name, err, n)
		}
	}
}

func TestRelationsDedupAndOrder(t *testing.T) {
	n, err := Parse([]byte(`[{"Plan": {"Node Type": "Hash Join", "Total Cost": 1, "Plan Rows": 1, "Plan Width": 1,
	  "Plans": [
	    {"Node Type": "Seq Scan", "Relation Name": "a", "Total Cost": 1, "Plan Rows": 1, "Plan Width": 1},
	    {"Node Type": "Index Scan", "Relation Name": "b", "Index Name": "b_pkey", "Total Cost": 1, "Plan Rows": 1, "Plan Width": 1},
	    {"Node Type": "Seq Scan", "Relation Name": "a", "Total Cost": 1, "Plan Rows": 1, "Plan Width": 1}]}}]`))
	if err != nil {
		t.Fatal(err)
	}
	got := n.Relations()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("Relations() = %v, want [a b]", got)
	}
}
