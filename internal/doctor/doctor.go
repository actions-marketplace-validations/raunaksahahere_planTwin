// Package doctor checks that a repository is ready for `plantwin check`. Each check explains how to fix a failure.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/raunaksahahere/plantwin/internal/bundle"
	"github.com/raunaksahahere/plantwin/internal/config"
)

// Result is one check.
type Result struct {
	Name    string
	OK      bool
	Message string
	Fix     string
}

// MaxBundleAge is how old a bundle may be before doctor warns. Stale statistics make the twin drift from production.
const MaxBundleAge = 14 * 24 * time.Hour

// Run executes every check and returns the results in order.
func Run(ctx context.Context, cfgPath string) []Result {
	var out []Result
	add := func(r Result) { out = append(out, r) }

	dockerOK := false
	if msg, err := run(ctx, "docker", "info", "--format", "{{.ServerVersion}}"); err == nil {
		dockerOK = true
		add(Result{Name: "Docker", OK: true, Message: "daemon reachable (server " + strings.TrimSpace(msg) + ")"})
	} else {
		add(Result{Name: "Docker", OK: false, Message: "daemon not reachable",
			Fix: "Install Docker and start the daemon. On Linux: `sudo systemctl start docker`, then add your user to the docker group."})
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		add(Result{Name: "Config", OK: false, Message: firstLine(err.Error()), Fix: "Fix plantwin.yaml as described above."})
		return out
	}
	add(Result{Name: "Config", OK: true, Message: fmt.Sprintf("%s is valid (migrations: %s)", cfgPath, cfg.Migrations.Tool)})

	repo := filepath.Dir(cfgPath)
	if dockerOK {
		add(imageCheck(ctx, cfg.Image))
		add(extensionCheck(ctx, cfg.Image))
	}

	if msg, err := run(ctx, "git", "--version"); err == nil {
		add(Result{Name: "Git", OK: true, Message: strings.TrimSpace(msg)})
	} else {
		add(Result{Name: "Git", OK: false, Message: "git is not installed or not on PATH",
			Fix: "Install git. `plantwin check` uses git worktrees to read the base and head revisions."})
	}

	bundlePath := filepath.Join(repo, cfg.Bundle.Path)
	b, raw, err := bundle.Read(bundlePath)
	if err != nil {
		add(Result{Name: "Stats bundle", OK: false, Message: "cannot read " + cfg.Bundle.Path,
			Fix: "Export it on a read replica with `plantwin export --sign-key <key>`, then copy the bundle and its .sig file into this repository (or use CI artifacts)."})
		return out
	}
	add(bundleSignature(repo, bundlePath, cfg.Bundle.PublicKey, raw))
	add(bundleAge(b))
	add(bundleCompleteness(b))
	if len(b.ExtendedStatistics) > 0 {
		add(Result{Name: "Extended statistics", OK: true,
			Message: fmt.Sprintf("%d object(s) are not exported; multi-column estimates will differ (ADR-0004)", len(b.ExtendedStatistics))})
	}
	for _, w := range b.Warnings {
		add(Result{Name: "Bundle warning", OK: true, Message: w})
	}
	return out
}

func imageCheck(ctx context.Context, image string) Result {
	if _, err := run(ctx, "docker", "image", "inspect", image); err != nil {
		return Result{Name: "Twin image", OK: false, Message: fmt.Sprintf("image %q is not built locally", image),
			Fix: "Build it from the PlanTwin repository: `docker build -t " + image + " .`"}
	}
	out, err := run(ctx, "docker", "run", "--rm", image, "postgres", "--version")
	if err != nil {
		return Result{Name: "Twin image", OK: false, Message: "cannot run postgres inside " + image, Fix: "Rebuild the image."}
	}
	re := regexp.MustCompile(`PostgreSQL\) (\d+)`)
	m := re.FindStringSubmatch(out)
	if m == nil || m[1] != "18" {
		return Result{Name: "Twin image", OK: false, Message: "image is not PostgreSQL 18: " + strings.TrimSpace(out),
			Fix: "Statistics import needs PostgreSQL 18. Build the image from this repository's Dockerfile (base postgres:18)."}
	}
	return Result{Name: "Twin image", OK: true, Message: fmt.Sprintf("%s runs PostgreSQL %s", image, m[1])}
}

func extensionCheck(ctx context.Context, image string) Result {
	_, err := run(ctx, "docker", "run", "--rm", image, "sh", "-c", `test -f "$(pg_config --pkglibdir)/plantwin.so"`)
	if err != nil {
		return Result{Name: "plantwin extension", OK: false, Message: "image " + image + " does not contain plantwin.so",
			Fix: "Build the image from this repository's Dockerfile, which installs the extension. Plain postgres images do not have it."}
	}
	return Result{Name: "plantwin extension", OK: true, Message: "installed in " + image + " (not preloaded; loads only with plantwin.ci_marker)"}
}

func bundleSignature(repo, bundlePath, pubKey string, raw []byte) Result {
	sig, err := os.ReadFile(bundlePath + ".sig")
	if err != nil {
		return Result{Name: "Bundle signature", OK: false, Message: "no .sig file next to the bundle",
			Fix: "Re-export with `plantwin export --sign-key <key>`. Unsigned bundles are refused."}
	}
	key := pubKey
	if !filepath.IsAbs(key) {
		key = filepath.Join(repo, pubKey)
	}
	if err := bundle.Verify(raw, string(sig), key); err != nil {
		return Result{Name: "Bundle signature", OK: false, Message: err.Error(),
			Fix: "Re-export the bundle, or check that bundle.public_key matches the key that signed it."}
	}
	return Result{Name: "Bundle signature", OK: true, Message: "verifies with " + pubKey}
}

func bundleAge(b *bundle.Bundle) Result {
	t, err := time.Parse(time.RFC3339, b.ExportedAt)
	if err != nil {
		return Result{Name: "Bundle age", OK: false, Message: "exported_at is not a valid time", Fix: "Re-export the bundle."}
	}
	age := time.Since(t)
	if age > MaxBundleAge {
		return Result{Name: "Bundle age", OK: false, Message: fmt.Sprintf("exported %d days ago (limit %d)", int(age.Hours()/24), int(MaxBundleAge.Hours()/24)),
			Fix: "Re-export on the replica. Old statistics make the twin drift from production."}
	}
	return Result{Name: "Bundle age", OK: true, Message: fmt.Sprintf("exported %s ago", age.Round(time.Hour))}
}

func bundleCompleteness(b *bundle.Bundle) Result {
	have := map[string]bool{}
	for _, a := range b.Attributes {
		have[a.Schema+"."+a.Table] = true
	}
	var missing []string
	for _, r := range b.Relations {
		if r.Kind == "r" && r.RelPages > 0 && !have[r.Schema+"."+r.Name] {
			missing = append(missing, r.Name)
		}
	}
	if len(missing) > 0 {
		return Result{Name: "Stats completeness", OK: false,
			Message: fmt.Sprintf("%d table(s) have pages but no column statistics: %s", len(missing), strings.Join(missing, ", ")),
			Fix:     "Run ANALYZE on those tables on production (the owner's maintenance job), then re-export. PlanTwin never runs ANALYZE itself."}
	}
	return Result{Name: "Stats completeness", OK: true, Message: fmt.Sprintf("%d relations, %d column statistics", len(b.Relations), len(b.Attributes))}
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}
