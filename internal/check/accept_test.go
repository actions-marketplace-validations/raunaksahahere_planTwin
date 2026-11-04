package check

import (
	"path/filepath"
	"testing"

	"github.com/raunaksahahere/plantwin/internal/report"
	"github.com/raunaksahahere/plantwin/internal/rules"
)

func TestAcceptRemovesOnlyMatchingFindings(t *testing.T) {
	results := []report.QueryResult{{
		Name: "customer_lookup",
		Findings: []rules.Finding{
			{Rule: "lost_index", Relation: "orders"},
			{Rule: "lost_index", Relation: "customers"},
			{Rule: "join_method", Relation: "orders"},
		},
	}, {
		Name:     "open_orders",
		Findings: []rules.Finding{{Rule: "lost_index", Relation: "orders"}},
	}}
	accepted := []Accepted{{Query: "customer_lookup", Rule: "lost_index", Relation: "orders"}}
	got, removed := applyAccepted(results, accepted)
	if removed != 1 {
		t.Fatalf("removed %d, want 1", removed)
	}
	if len(got[0].Findings) != 2 || len(got[1].Findings) != 1 {
		t.Fatalf("wrong findings kept: %+v", got)
	}
}

func TestAcceptWithoutRelationMatchesAnyRelation(t *testing.T) {
	results := []report.QueryResult{{Name: "q", Findings: []rules.Finding{
		{Rule: "seq_scan_large", Relation: "a"}, {Rule: "seq_scan_large", Relation: "b"},
	}}}
	_, removed := applyAccepted(results, []Accepted{{Query: "q", Rule: "seq_scan_large"}})
	if removed != 2 {
		t.Fatalf("removed %d, want 2", removed)
	}
}

func TestAddAcceptedRoundTrip(t *testing.T) {
	repo := t.TempDir()
	if err := AddAccepted(repo, Accepted{Query: "q", Rule: "lost_index", Relation: "orders"}); err != nil {
		t.Fatal(err)
	}
	if err := AddAccepted(repo, Accepted{Query: "r", Rule: "cost_ratio"}); err != nil {
		t.Fatal(err)
	}
	all, err := LoadAccepted(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Relation != "orders" || all[1].Rule != "cost_ratio" {
		t.Fatalf("round trip lost entries: %+v", all)
	}
	if _, err := filepath.Abs(repo); err != nil {
		t.Fatal(err)
	}
}

func TestMissingAcceptedFileMeansNothingAccepted(t *testing.T) {
	all, err := LoadAccepted(t.TempDir())
	if err != nil || len(all) != 0 {
		t.Fatalf("got %v, %v", all, err)
	}
}
