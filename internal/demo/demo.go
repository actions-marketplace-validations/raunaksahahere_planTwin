// Package demo runs `plantwin demo`: a self-contained example. It creates a temporary git repository with
// two commits (the second drops an index), builds a synthetic "production" database in a container,
// exports and signs its statistics with an ephemeral key, and runs a normal check. Nothing is published.
package demo

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/raunaksahahere/plantwin/internal/bundle"
	"github.com/raunaksahahere/plantwin/internal/check"
	"github.com/raunaksahahere/plantwin/internal/config"
	"github.com/raunaksahahere/plantwin/internal/errs"
	"github.com/raunaksahahere/plantwin/internal/export"
	"github.com/raunaksahahere/plantwin/internal/twin"
)

//go:embed assets
var assets embed.FS

// Prepare creates the demo repository in a new temporary directory and returns its path.
// A non-empty redaction overrides redaction.default in the demo config (used by the redaction benchmark).
func Prepare(redaction string) (string, error) {
	dir, err := os.MkdirTemp("", "plantwin-demo-")
	if err != nil {
		return "", err
	}
	if err := copyAssets(dir); err != nil {
		return dir, err
	}
	if redaction != "" {
		switch redaction {
		case "keep", "hash", "drop-mcv", "drop-histogram":
		default:
			return dir, errs.New(fmt.Sprintf("Unknown redaction mode %q.", redaction),
				"Use keep, hash, drop-mcv or drop-histogram.", "redaction")
		}
		cfgPath := filepath.Join(dir, "plantwin.yaml")
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			return dir, err
		}
		updated := strings.Replace(string(data), "  default: hash", "  default: "+redaction, 1)
		if err := os.WriteFile(cfgPath, []byte(updated), 0o644); err != nil {
			return dir, err
		}
	}
	git := func(args ...string) error {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=plantwin-demo", "-c", "user.email=demo@plantwin.invalid",
			"-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return errs.Env("Git is needed for the demo.", "Install git and run `plantwin demo` again.",
				"git %v: %s", args, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := git("init", "-q", "-b", "main"); err != nil {
		return dir, err
	}
	// The base commit has only the first migration; the head commit adds the one that drops the index.
	if err := git("add", "plantwin.yaml", "migrations/0001_schema.sql", "queries"); err != nil {
		return dir, err
	}
	if err := git("commit", "-q", "-m", "Initial schema"); err != nil {
		return dir, err
	}
	if err := git("add", "migrations/0002_drop_customer_index.sql"); err != nil {
		return dir, err
	}
	if err := git("commit", "-q", "-m", "Drop the customer index"); err != nil {
		return dir, err
	}
	return dir, nil
}

// Run exports the synthetic production statistics and checks HEAD~1 against HEAD.
func Run(ctx context.Context, dir string, keep bool, verbose bool, version string) (check.Outcome, error) {
	var out check.Outcome
	cfg, err := config.Load(filepath.Join(dir, "plantwin.yaml"))
	if err != nil {
		return out, err
	}
	if err := twin.RequireDocker(ctx); err != nil {
		return out, err
	}

	// Ephemeral signing key: generated for this run, never stored outside the demo directory.
	keyDir := filepath.Join(dir, ".demo")
	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		return out, err
	}
	privPath := filepath.Join(keyDir, "plantwin.key")
	pubPath := filepath.Join(dir, "plantwin.pub")
	if err := bundle.GenerateKeys(privPath, pubPath); err != nil {
		return out, err
	}

	// The "production" database: the base schema plus synthetic data.
	prodMig := filepath.Join(keyDir, "prod-migrations")
	if err := os.MkdirAll(prodMig, 0o755); err != nil {
		return out, err
	}
	base, err := os.ReadFile(filepath.Join(dir, "migrations", "0001_schema.sql"))
	if err != nil {
		return out, err
	}
	if err := os.WriteFile(filepath.Join(prodMig, "0001_schema.sql"), base, 0o644); err != nil {
		return out, err
	}
	seed, err := os.ReadFile(filepath.Join(keyDir, "prod_seed.sql"))
	if err != nil {
		return out, err
	}
	prod, err := twin.Start(ctx, fmt.Sprintf("plantwin-demo-prod-%d", time.Now().UnixNano()%1e9), cfg.Image)
	if err != nil {
		return out, err
	}
	defer prod.Stop()
	if err := twin.ApplyMigrations(ctx, prod, "sql", prodMig, ""); err != nil {
		return out, err
	}
	conn, err := prod.Connect(ctx)
	if err != nil {
		return out, err
	}
	if _, err := conn.Exec(ctx, string(seed)); err != nil {
		conn.Close(ctx)
		return out, errs.New("The synthetic production data could not be loaded.", "Report this as a bug; the demo assets are part of the binary.", "%v", err)
	}
	conn.Close(ctx)

	// Export and sign, exactly as a read replica would be exported.
	b, err := export.Run(ctx, export.Options{DSN: prod.DSN, Default: cfg.Redaction.Default, Columns: cfg.Redaction.Columns, Now: time.Now()})
	if err != nil {
		return out, err
	}
	bundlePath := filepath.Join(dir, cfg.Bundle.Path)
	if err := bundle.Write(bundlePath, b); err != nil {
		return out, err
	}
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		return out, err
	}
	sig, err := bundle.Sign(raw, privPath)
	if err != nil {
		return out, err
	}
	if err := os.WriteFile(bundlePath+".sig", []byte(sig+"\n"), 0o644); err != nil {
		return out, err
	}
	prod.Stop()

	cfgPath := filepath.Join(dir, "plantwin.yaml")
	return check.Run(ctx, check.Options{
		Repo: dir, ConfigPath: cfgPath, Base: "HEAD~1", Head: "HEAD",
		KeepTwins: keep, Parallel: 4, Verbose: verbose, ToolVersion: version,
	})
}

// removeAll is a seam for tests, so they can clean up the demo directory.
var removeAll = os.RemoveAll

func copyAssets(dir string) error {
	return fs.WalkDir(assets, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "assets/")
		if rel == "assets" {
			return nil
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := assets.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		// prod_seed.sql is not part of the repository: it lives under .demo/.
		if rel == "prod_seed.sql" {
			target = filepath.Join(dir, ".demo", "prod_seed.sql")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
		}
		return os.WriteFile(target, data, 0o644)
	})
}
