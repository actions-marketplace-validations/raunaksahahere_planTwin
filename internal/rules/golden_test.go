package rules

// Golden tests: the rules applied to plans recorded from the migration corpus (bench/corpus).
// Run `go test ./internal/rules -run Golden -update` after an intended change, and review the diff.

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/raunaksahahere/plantwin/internal/plan"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestGoldenCorpusFindings(t *testing.T) {
	root := filepath.Join("testdata", "golden")
	var rows map[string]float64
	data, err := os.ReadFile(filepath.Join(root, "table_rows.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	cases, _ := filepath.Glob(filepath.Join(root, "c*"))
	sort.Strings(cases)
	for _, dir := range cases {
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.TableRows = rows
			var all []Finding
			bases, _ := filepath.Glob(filepath.Join(dir, "base", "*.json"))
			sort.Strings(bases)
			for _, bpath := range bases {
				q := strings.TrimSuffix(filepath.Base(bpath), ".json")
				base := mustPlan(t, bpath)
				in := Input{Query: q, Base: base}
				if e, err := os.ReadFile(filepath.Join(dir, "head", q+".err")); err == nil {
					in.HeadErr = strings.TrimSpace(string(e))
				} else {
					in.Head = mustPlan(t, filepath.Join(dir, "head", q+".json"))
				}
				all = append(all, Analyse(in, cfg)...)
			}
			got, err := json.MarshalIndent(all, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join(dir, "expected.json")
			if *update {
				if err := os.WriteFile(golden, append(got, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden file (run with -update): %v", err)
			}
			if strings.TrimSpace(string(want)) != strings.TrimSpace(string(got)) {
				t.Fatalf("findings differ from %s; run with -update if the change is intended", golden)
			}
		})
	}
}

func mustPlan(t *testing.T, path string) plan.Node {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	n, err := plan.Parse(data)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return n
}
