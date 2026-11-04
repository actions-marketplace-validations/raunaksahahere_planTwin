// Package config loads and validates plantwin.yaml.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/raunaksahahere/plantwin/internal/errs"
	"github.com/raunaksahahere/plantwin/internal/rules"
)

// Config is the parsed plantwin.yaml.
type Config struct {
	Version    int        `yaml:"version"`
	Image      string     `yaml:"image"`
	Migrations Migrations `yaml:"migrations"`
	Queries    []Query    `yaml:"queries"`
	Thresholds Thresholds `yaml:"thresholds"`
	Redaction  Redaction  `yaml:"redaction"`
	FailOn     string     `yaml:"fail_on"`
	Bundle     Bundle     `yaml:"bundle"`
}

// Migrations says how to apply schema migrations to the twin.
type Migrations struct {
	Tool    string `yaml:"tool"`    // sql | command | none
	Path    string `yaml:"path"`    // for sql: directory of *.sql files, applied in name order
	Command string `yaml:"command"` // for command: shell command run with DATABASE_URL set to the twin
}

// Query is one named query from the catalog.
type Query struct {
	Name string `yaml:"name"`
	SQL  string `yaml:"sql"`
	File string `yaml:"file"`
}

// Thresholds mirrors rules.Config. Zero values mean "use the default".
type Thresholds struct {
	LargeRows    float64 `yaml:"large_rows"`
	SeqScanRows  float64 `yaml:"seq_scan_rows"`
	JoinRows     float64 `yaml:"join_rows"`
	WorkMemBytes float64 `yaml:"work_mem_bytes"`
	CostRatio    float64 `yaml:"cost_ratio"`
	MinCost      float64 `yaml:"min_cost"`
}

// Redaction says how statistics values are exported. See docs/privacy-and-redaction.md.
type Redaction struct {
	Default string            `yaml:"default"`
	Columns map[string]string `yaml:"columns"`
}

// Bundle says where the stats bundle is and how it is verified.
type Bundle struct {
	Path      string `yaml:"path"`
	PublicKey string `yaml:"public_key"`
}

// Defaults returns a config with every default filled in.
func Defaults() Config {
	return Config{
		Version:    1,
		Image:      "plantwin-postgres:18",
		Migrations: Migrations{Tool: "sql", Path: "migrations"},
		Redaction:  Redaction{Default: "hash"},
		FailOn:     "warning",
		Bundle:     Bundle{Path: "plantwin-bundle.json", PublicKey: "plantwin.pub"},
	}
}

// Load reads and validates a config file. Missing keys keep their defaults.
func Load(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, errs.New("plantwin.yaml was not found or cannot be read.",
			"Run `plantwin init` in the repository root to create it.", "cannot read %s", path)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, errs.New("plantwin.yaml is not valid YAML.",
			"Check the indentation and quoting near the line the parser reports.", "cannot parse %s: %v", path, err)
	}
	return cfg, Validate(&cfg)
}

// Validate checks the config and returns the first problem as a user error.
func Validate(cfg *Config) error {
	switch cfg.Migrations.Tool {
	case "sql":
		if cfg.Migrations.Path == "" {
			return errs.New("The SQL migration tool needs a directory.", "Set migrations.path, for example `migrations`.", "migrations.path is empty")
		}
	case "command":
		if strings.TrimSpace(cfg.Migrations.Command) == "" {
			return errs.New("The command migration tool needs a command.", "Set migrations.command, for example `python manage.py migrate`.", "migrations.command is empty")
		}
	case "none":
	default:
		return errs.New(fmt.Sprintf("Unknown migration tool %q.", cfg.Migrations.Tool),
			"Use one of: sql, command, none.", "migrations.tool")
	}
	switch cfg.FailOn {
	case "error", "warning", "info":
	default:
		return errs.New(fmt.Sprintf("Unknown fail_on value %q.", cfg.FailOn), "Use error, warning or info.", "fail_on")
	}
	switch cfg.Redaction.Default {
	case "keep", "hash", "drop-mcv", "drop-histogram":
	default:
		return errs.New(fmt.Sprintf("Unknown redaction mode %q.", cfg.Redaction.Default),
			"Use keep, hash, drop-mcv or drop-histogram. See docs/privacy-and-redaction.md.", "redaction.default")
	}
	for col, mode := range cfg.Redaction.Columns {
		switch mode {
		case "keep", "hash", "drop-mcv", "drop-histogram":
		default:
			return errs.New(fmt.Sprintf("Unknown redaction mode %q for column %s.", mode, col),
				"Use keep, hash, drop-mcv or drop-histogram.", "redaction.columns.%s", col)
		}
	}
	seen := map[string]bool{}
	for i, q := range cfg.Queries {
		if q.Name == "" {
			return errs.New(fmt.Sprintf("Query number %d has no name.", i+1), "Give every query a unique name.", "queries[%d].name", i)
		}
		if seen[q.Name] {
			return errs.New(fmt.Sprintf("Two queries are named %q.", q.Name), "Query names must be unique.", "queries")
		}
		seen[q.Name] = true
		if q.SQL == "" && q.File == "" {
			return errs.New(fmt.Sprintf("Query %q has no SQL.", q.Name), "Set either sql or file.", "queries.%s", q.Name)
		}
	}
	return nil
}

// Rules converts the thresholds into rules.Config, filling defaults for zero values.
func (c Config) Rules() rules.Config {
	r := rules.DefaultConfig()
	if c.Thresholds.LargeRows > 0 {
		r.LargeRows = c.Thresholds.LargeRows
	}
	if c.Thresholds.SeqScanRows > 0 {
		r.SeqScanRows = c.Thresholds.SeqScanRows
	}
	if c.Thresholds.JoinRows > 0 {
		r.JoinRows = c.Thresholds.JoinRows
	}
	if c.Thresholds.WorkMemBytes > 0 {
		r.WorkMemBytes = c.Thresholds.WorkMemBytes
	}
	if c.Thresholds.CostRatio > 0 {
		r.CostRatio = c.Thresholds.CostRatio
	}
	if c.Thresholds.MinCost > 0 {
		r.MinCost = c.Thresholds.MinCost
	}
	r.FailOn = rules.Severity(c.FailOn)
	return r
}
