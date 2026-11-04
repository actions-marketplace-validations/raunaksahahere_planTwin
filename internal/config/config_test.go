package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "plantwin.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDefaultsAndQueries(t *testing.T) {
	cfg, err := Load(write(t, `
queries:
  - name: lookup
    sql: SELECT 1
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Redaction.Default != "hash" || cfg.FailOn != "warning" || cfg.Migrations.Tool != "sql" {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if r := cfg.Rules(); r.LargeRows != 100000 || r.IndexSelectivity != 0.10 {
		t.Fatalf("threshold defaults wrong: %+v", r)
	}
}

func TestValidationMessagesAreSpecific(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"bad fail_on":            {"fail_on: loud\nqueries:\n  - name: q\n    sql: SELECT 1\n", "Unknown fail_on value"},
		"bad redaction":          {"redaction:\n  default: scramble\nqueries:\n  - name: q\n    sql: SELECT 1\n", "Unknown redaction mode"},
		"no queries have a name": {"queries:\n  - sql: SELECT 1\n", "has no name"},
		"duplicate query":        {"queries:\n  - name: q\n    sql: SELECT 1\n  - name: q\n    sql: SELECT 2\n", "Two queries are named"},
		"query without sql":      {"queries:\n  - name: q\n", "has no SQL"},
		"command tool empty":     {"migrations:\n  tool: command\nqueries:\n  - name: q\n    sql: SELECT 1\n", "needs a command"},
	}
	for name, c := range cases {
		_, err := Load(write(t, c.body))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want message containing %q", name, err, c.want)
		}
	}
}

func TestMissingFileSaysHowToFix(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil || !strings.Contains(err.Error(), "plantwin init") {
		t.Fatalf("missing config must point to `plantwin init`: %v", err)
	}
}
