package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/raunaksahahere/plantwin/internal/plan"
	"github.com/raunaksahahere/plantwin/internal/rules"
)

func sampleRun() Run {
	base := &plan.Node{NodeType: "Index Scan", RelationName: "orders", IndexName: "orders_customer_idx", PlanRows: 21, TotalCost: 85}
	head := &plan.Node{NodeType: "Seq Scan", RelationName: "orders", PlanRows: 9, TotalCost: 14544}
	return Run{
		Base: "main", Head: "HEAD", FailOn: rules.Warning,
		Queries: []QueryResult{{
			Name: "customer_lookup", BasePlan: base, HeadPlan: head,
			Findings: []rules.Finding{{
				Rule: "lost_index", Severity: rules.Error, Query: "customer_lookup", Relation: "orders", Estimated: true,
				Message: "Index use lost on orders.", Why: "Reads every row.", Fix: "Recreate the index.",
			}},
		}},
		Estimated: []string{"orders"},
		Timing:    []Phase{Ms("total", 1500)},
	}
}

func TestJSONCarriesSchemaVersion(t *testing.T) {
	data, err := JSON(sampleRun())
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		SchemaVersion int  `json:"schema_version"`
		Failed        bool `json:"failed"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if v.SchemaVersion != SchemaVersion || !v.Failed {
		t.Fatalf("got %+v", v)
	}
}

func TestSARIFIsValidLogWithRules(t *testing.T) {
	data, err := SARIF(sampleRun(), "test")
	if err != nil {
		t.Fatal(err)
	}
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatal(err)
	}
	if log.Version != "2.1.0" || len(log.Runs) != 1 {
		t.Fatalf("not a SARIF 2.1.0 log: %+v", log)
	}
	if len(log.Runs[0].Results) != 1 || log.Runs[0].Results[0].Level != "error" || log.Runs[0].Results[0].RuleID != "lost_index" {
		t.Fatalf("unexpected results: %+v", log.Runs[0].Results)
	}
	if len(log.Runs[0].Tool.Driver.Rules) != 1 || log.Runs[0].Tool.Driver.Rules[0].ID != "lost_index" {
		t.Fatalf("rules not listed once: %+v", log.Runs[0].Tool.Driver.Rules)
	}
}

func TestMarkdownShowsPlansAndLabels(t *testing.T) {
	md := Markdown(sampleRun())
	for _, want := range []string{"## PlanTwin: main → HEAD", "**Result: failing.**", "*(estimated)*",
		"Why this matters:", "Suggested fix:", "Plans, side by side", "Index Scan on orders using orders_customer_idx"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown is missing %q", want)
		}
	}
}

func TestPassingRunSaysSo(t *testing.T) {
	r := sampleRun()
	r.Queries[0].Findings = nil
	if r.Failed() {
		t.Fatal("no findings must not fail the run")
	}
	if !strings.Contains(Markdown(r), "No plan changes detected") {
		t.Fatal("a run without findings must say so")
	}
}

func TestTreeTextIndentsChildren(t *testing.T) {
	n := &plan.Node{NodeType: "Hash Join", JoinType: "Left", PlanRows: 5, TotalCost: 9,
		Plans: []plan.Node{{NodeType: "Seq Scan", RelationName: "a", PlanRows: 1, TotalCost: 1}}}
	got := TreeText(n)
	if !strings.Contains(got, "Hash Join (Left)  rows=5 cost=9\n  Seq Scan on a") {
		t.Fatalf("unexpected tree text:\n%s", got)
	}
}
