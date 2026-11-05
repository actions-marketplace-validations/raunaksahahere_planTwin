//go:build integration

package demo

// Integration test: real Docker, real PostgreSQL 18 with the plantwin extension, real export and check.
// Requires the image plantwin-postgres:18 (docker build -t plantwin-postgres:18 .).
// Run with `make test-integration`.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDemoFlagsDroppedIndex(t *testing.T) {
	ctx := context.Background()
	dir, err := Prepare("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	out, err := Run(ctx, dir, false, false, "test")
	if err != nil {
		t.Fatalf("demo run failed: %v", err)
	}
	if !out.Run.Failed() {
		t.Fatal("the demo migration drops an index and must fail the check")
	}
	found := false
	for _, q := range out.Run.Queries {
		for _, f := range q.Findings {
			if f.Rule == "lost_index" && f.Relation == "orders" {
				found = true
				if !f.Estimated {
					t.Errorf("orders statistics changed in the migration, so the finding must be labelled estimated")
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected a lost_index finding on orders, got %+v", out.Run.Queries)
	}

	// The schema cache: base and head migrations differ, so there must be two cache entries.
	entries, _ := filepath.Glob(filepath.Join(dir, ".plantwin", "cache", "*.sql"))
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 schema cache entries (base and head), found %d", len(entries))
	}
}
