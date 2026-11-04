package rules

import (
	"testing"

	"github.com/raunaksahahere/plantwin/internal/plan"
)

func seq(rel string, rows float64) plan.Node {
	return plan.Node{NodeType: "Seq Scan", RelationName: rel, PlanRows: rows, PlanWidth: 8, TotalCost: rows}
}

func ixscan(rel, index string, rows float64) plan.Node {
	return plan.Node{NodeType: "Index Scan", RelationName: rel, IndexName: index, PlanRows: rows, PlanWidth: 8, TotalCost: 2}
}

func join(kind string, rows float64, kids ...plan.Node) plan.Node {
	return plan.Node{NodeType: kind, JoinType: "Inner", PlanRows: rows, PlanWidth: 8, TotalCost: rows * 2, Plans: kids}
}

func rulesOf(fs []Finding) map[string]Severity {
	out := map[string]Severity{}
	for _, f := range fs {
		out[f.Rule] = f.Severity
	}
	return out
}

func TestLostIndexSeverityFollowsSize(t *testing.T) {
	cases := []struct {
		name     string
		rows     float64
		wantSev  Severity
		wantRule string
	}{
		{"large table is an error", 2_000_000, Error, "lost_index"},
		{"small table is a warning", 5_000, Warning, "lost_index"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := plan.Node{NodeType: "Index Scan", RelationName: "orders", IndexName: "orders_customer", PlanRows: c.rows, PlanWidth: 8, TotalCost: 3}
			head := seq("orders", c.rows)
			fs := Analyse(Input{Query: "q", Base: base, Head: head}, DefaultConfig())
			got := rulesOf(fs)
			if got[c.wantRule] != c.wantSev {
				t.Fatalf("got %v, want %s=%s", got, c.wantRule, c.wantSev)
			}
		})
	}
}

func TestLostIndexFromBitmapScan(t *testing.T) {
	base := plan.Node{NodeType: "Bitmap Heap Scan", RelationName: "orders", PlanRows: 300_000, PlanWidth: 8, TotalCost: 9,
		Plans: []plan.Node{{NodeType: "Bitmap Index Scan", IndexName: "orders_status", PlanRows: 300_000, PlanWidth: 0, TotalCost: 4}}}
	head := seq("orders", 300_000)
	got := rulesOf(Analyse(Input{Query: "q", Base: base, Head: head}, DefaultConfig()))
	if got["lost_index"] != Error {
		t.Fatalf("bitmap index loss on 300K rows should be an error, got %v", got)
	}
}

func TestSeqScanThreshold(t *testing.T) {
	// Base has no scan of orders, so a new sequential scan is an addition, not a lost index.
	base := seq("other", 10)
	cfg := DefaultConfig()
	below := rulesOf(Analyse(Input{Query: "q", Base: base, Head: seq("orders", 9_999)}, cfg))
	above := rulesOf(Analyse(Input{Query: "q", Base: base, Head: seq("orders", 10_001)}, cfg))
	if _, ok := below["seq_scan_large"]; ok {
		t.Fatalf("9,999 rows must not trigger seq_scan_large: %v", below)
	}
	if _, ok := above["seq_scan_large"]; !ok {
		t.Fatalf("10,001 rows must trigger seq_scan_large: %v", above)
	}
}

func TestJoinMethodChange(t *testing.T) {
	base := join("Nested Loop", 50_000, seq("a", 100), ixscan("b", "b_pk", 1))
	head := join("Hash Join", 50_000, seq("a", 100), plan.Node{NodeType: "Hash", PlanRows: 1, PlanWidth: 8, Plans: []plan.Node{seq("b", 1)}})
	got := rulesOf(Analyse(Input{Query: "q", Base: base, Head: head}, DefaultConfig()))
	if got["join_method"] != Warning {
		t.Fatalf("expected join_method warning, got %v", got)
	}
}

func TestSortOverWorkMemOnlyWhenNew(t *testing.T) {
	big := plan.Node{NodeType: "Sort", PlanRows: 1_000_000, PlanWidth: 64, TotalCost: 9, Plans: []plan.Node{seq("a", 1_000_000)}}
	head := big
	got := rulesOf(Analyse(Input{Query: "q", Base: seq("a", 1_000_000), Head: head}, DefaultConfig()))
	if got["sort_hash_work_mem"] != Warning {
		t.Fatalf("new 64 MB sort should be reported, got %v", got)
	}
	// The same sort already in the base plan is not new, so it is not reported.
	got = rulesOf(Analyse(Input{Query: "q", Base: big, Head: big}, DefaultConfig()))
	if _, ok := got["sort_hash_work_mem"]; ok {
		t.Fatalf("unchanged sort must not be reported: %v", got)
	}
}

func TestCostRatioIsInfoAndNeedsMinimumCost(t *testing.T) {
	base := plan.Node{NodeType: "Seq Scan", RelationName: "a", PlanRows: 1, TotalCost: 100}
	cheap := plan.Node{NodeType: "Seq Scan", RelationName: "a", PlanRows: 1, TotalCost: 900}
	pricey := plan.Node{NodeType: "Seq Scan", RelationName: "a", PlanRows: 1, TotalCost: 5000}
	cfg := DefaultConfig()
	if got := rulesOf(Analyse(Input{Query: "q", Base: base, Head: cheap}, cfg)); got["cost_ratio"] != "" {
		t.Fatalf("head cost below MinCost must not report cost_ratio: %v", got)
	}
	if got := rulesOf(Analyse(Input{Query: "q", Base: base, Head: pricey}, cfg)); got["cost_ratio"] != Info {
		t.Fatalf("50x rise above MinCost should be info: %v", got)
	}
}

func TestEstimatedLabel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EstimatedRelns = map[string]bool{"orders": true}
	base := ixscan("orders", "orders_pk", 10)
	fs := Analyse(Input{Query: "q", Base: base, Head: seq("orders", 50_000)}, cfg)
	if len(fs) == 0 || !fs[0].Estimated {
		t.Fatalf("finding on an estimated relation must be labelled estimated: %+v", fs)
	}
}

func TestIdenticalPlansHaveNoFindings(t *testing.T) {
	p := join("Hash Join", 50_000, seq("a", 1000), seq("b", 1000))
	if fs := Analyse(Input{Query: "q", Base: p, Head: p}, DefaultConfig()); len(fs) != 0 {
		t.Fatalf("identical plans must have no findings: %+v", fs)
	}
}

func TestFailsRespectsThreshold(t *testing.T) {
	fs := []Finding{{Severity: Info}, {Severity: Warning}}
	if !Fails(fs, Warning) {
		t.Fatal("a warning must fail when FailOn=warning")
	}
	if Fails([]Finding{{Severity: Info}}, Warning) {
		t.Fatal("info must not fail when FailOn=warning")
	}
	if !Fails([]Finding{{Severity: Error}}, Error) {
		t.Fatal("an error must fail when FailOn=error")
	}
}

func TestHeadQueryErrorIsAnError(t *testing.T) {
	base := seq("orders", 1000)
	fs := Analyse(Input{Query: "q", Base: base, HeadErr: "operator does not exist: text = integer"}, DefaultConfig())
	if len(fs) != 1 || fs[0].Rule != "query_error" || fs[0].Severity != Error {
		t.Fatalf("expected a single query_error error, got %+v", fs)
	}
}

// A parallel plan puts the sequential scan under Gather. The rule must still find it.
func TestLostIndexFoundUnderGather(t *testing.T) {
	base := plan.Node{NodeType: "Bitmap Heap Scan", RelationName: "orders", PlanRows: 21, PlanWidth: 8, TotalCost: 85,
		Plans: []plan.Node{{NodeType: "Bitmap Index Scan", IndexName: "orders_customer_idx", PlanRows: 21, TotalCost: 4}}}
	head := plan.Node{NodeType: "Gather", PlanRows: 21, PlanWidth: 8, TotalCost: 14544,
		Plans: []plan.Node{seq("orders", 9)}}
	cfg := DefaultConfig()
	cfg.TableRows = map[string]float64{"orders": 1_000_000}
	got := rulesOf(Analyse(Input{Query: "q", Base: base, Head: head}, cfg))
	if got["lost_index"] == "" {
		t.Fatalf("lost index under Gather must be reported: %v", got)
	}
}

// If the old index was not selective (it returned 25% of the table), losing it is not a regression.
func TestLostIndexIgnoredWhenIndexWasNotSelective(t *testing.T) {
	base := plan.Node{NodeType: "Index Scan", RelationName: "customers", IndexName: "customers_region_idx", PlanRows: 25_000, PlanWidth: 8, TotalCost: 900}
	head := seq("customers", 25_000)
	cfg := DefaultConfig()
	cfg.TableRows = map[string]float64{"customers": 100_000}
	got := rulesOf(Analyse(Input{Query: "q", Base: base, Head: head}, cfg))
	if _, ok := got["lost_index"]; ok {
		t.Fatalf("a 25%% index scan is not selective; no lost_index expected: %v", got)
	}
}

// A selective index (5% of the table) lost to a sequential scan is a regression.
func TestLostIndexReportedWhenIndexWasSelective(t *testing.T) {
	base := plan.Node{NodeType: "Index Scan", RelationName: "orders", IndexName: "orders_status_idx", PlanRows: 50_000, PlanWidth: 8, TotalCost: 900}
	head := seq("orders", 50_000)
	cfg := DefaultConfig()
	cfg.TableRows = map[string]float64{"orders": 1_000_000}
	got := rulesOf(Analyse(Input{Query: "q", Base: base, Head: head}, cfg))
	if got["lost_index"] == "" {
		t.Fatalf("a 5%% index scan lost should be reported: %v", got)
	}
}

// A sequential scan that returns a large share of a big table is the right plan: no seq_scan_large.
func TestSeqScanLargeIgnoredWhenMostRowsAreReturned(t *testing.T) {
	base := seq("other", 10)
	head := seq("customers", 25_000)
	cfg := DefaultConfig()
	cfg.TableRows = map[string]float64{"customers": 100_000}
	got := rulesOf(Analyse(Input{Query: "q", Base: base, Head: head}, cfg))
	if _, ok := got["seq_scan_large"]; ok {
		t.Fatalf("returning 25%% of the table must not trigger seq_scan_large: %v", got)
	}
}
