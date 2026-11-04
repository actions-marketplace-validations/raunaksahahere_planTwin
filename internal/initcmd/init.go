// Package initcmd implements `plantwin init`: detect the migration tool and write a starter plantwin.yaml.
package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/raunaksahahere/plantwin/internal/errs"
)

// Detection is one migration tool found in the repository.
type Detection struct {
	Tool    string // sql | command | none
	Name    string // human name: "Django", "Alembic", ...
	Path    string // directory or file that identified it
	Command string // command to apply migrations to the twin (for command tools)
	Note    string // what the user must still do
}

// Detect looks at the repository and returns every migration tool it can identify, most specific first.
// A repository can contain several; init picks the first and lists the rest.
func Detect(dir string) []Detection {
	var out []Detection
	has := func(rel string) bool {
		_, err := os.Stat(filepath.Join(dir, rel))
		return err == nil
	}
	glob := func(pattern string) []string {
		m, _ := filepath.Glob(filepath.Join(dir, pattern))
		return m
	}
	if has("manage.py") && len(glob("*/migrations/*.py")) > 0 {
		out = append(out, Detection{Tool: "command", Name: "Django", Path: "manage.py",
			Command: "python manage.py migrate", Note: "Needs Python and the project's requirements on this machine."})
	}
	if has("prisma/schema.prisma") {
		out = append(out, Detection{Tool: "command", Name: "Prisma", Path: "prisma/schema.prisma",
			Command: "npx prisma migrate deploy", Note: "Needs Node.js and the project's dependencies on this machine."})
	}
	if has("alembic.ini") {
		out = append(out, Detection{Tool: "command", Name: "Alembic", Path: "alembic.ini",
			Command: "alembic upgrade head", Note: "Needs Python and the project's requirements on this machine."})
	}
	if has("db/migrate") && has("config/database.yml") {
		out = append(out, Detection{Tool: "command", Name: "Rails", Path: "db/migrate",
			Command: "bin/rails db:migrate", Note: "Needs Ruby and the project's gems on this machine. Rails may also need RAILS_ENV set."})
	}
	if has("atlas.hcl") {
		out = append(out, Detection{Tool: "command", Name: "Atlas", Path: "atlas.hcl",
			Command: "atlas migrate apply --url \"$DATABASE_URL\" --dir file://migrations", Note: "Needs the atlas CLI on this machine."})
	}
	if has("flyway.conf") || len(glob("sql/V*__*.sql")) > 0 {
		out = append(out, Detection{Tool: "sql", Name: "Flyway", Path: "sql",
			Note: "Flyway versioned files are applied in name order. Repeatable (R__) migrations are not supported yet."})
	}
	if len(glob("migrations/*.sql")) > 0 {
		out = append(out, Detection{Tool: "sql", Name: "plain SQL", Path: "migrations",
			Note: "Files are applied in name order."})
	}
	return out
}

// Write creates plantwin.yaml in dir. It refuses to overwrite an existing file.
func Write(dir string, det Detection, queriesFile string) (string, error) {
	path := filepath.Join(dir, "plantwin.yaml")
	if _, err := os.Stat(path); err == nil {
		return path, errs.New("plantwin.yaml already exists.",
			"Edit it directly, or delete it and run `plantwin init` again.", "%s exists", path)
	}
	var b strings.Builder
	b.WriteString("version: 1\n")
	b.WriteString("# Image used for the twin. Build it with `docker build -t plantwin-postgres:18 .` in the PlanTwin repository.\n")
	b.WriteString("image: plantwin-postgres:18\n\n")
	b.WriteString("migrations:\n")
	switch det.Tool {
	case "sql":
		fmt.Fprintf(&b, "  tool: sql\n  path: %s\n", filepath.ToSlash(det.Path))
	case "command":
		fmt.Fprintf(&b, "  tool: command\n  command: %q\n", det.Command)
	default:
		b.WriteString("  tool: none\n")
	}
	b.WriteString("\n# Named queries whose plans are compared. $1, $2 ... are allowed (GENERIC_PLAN is used).\n")
	b.WriteString("queries:\n")
	fmt.Fprintf(&b, "  - name: example_lookup\n    file: %s\n", queriesFile)
	b.WriteString(`
# Redaction for statistics values exported from production (docs/privacy-and-redaction.md).
#   keep | hash | drop-mcv | drop-histogram
redaction:
  default: hash
  # columns:
  #   orders.status: keep

# Thresholds (defaults are in docs/rules.md). Uncomment to change.
# thresholds:
#   large_rows: 100000
#   seq_scan_rows: 10000

fail_on: warning

bundle:
  path: plantwin-bundle.json
  public_key: plantwin.pub
`)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return path, err
	}
	qdir := filepath.Join(dir, filepath.Dir(queriesFile))
	if err := os.MkdirAll(qdir, 0o755); err != nil {
		return path, err
	}
	qpath := filepath.Join(dir, queriesFile)
	if _, err := os.Stat(qpath); os.IsNotExist(err) {
		_ = os.WriteFile(qpath, []byte("-- Replace with a query your application runs. Use $1 for parameters.\nSELECT 1;\n"), 0o644)
	}
	return path, nil
}

// Names returns the detected tool names, for printing.
func Names(dets []Detection) string {
	var n []string
	for _, d := range dets {
		n = append(n, d.Name)
	}
	sort.Strings(n)
	return strings.Join(n, ", ")
}
