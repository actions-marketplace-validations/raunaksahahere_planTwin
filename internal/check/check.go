// Package check runs `plantwin check`: it builds a base and a head twin, imports the
// stats bundle into both, explains the query catalog on each, and reports the differences.
package check

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/raunaksahahere/plantwin/internal/bundle"
	"github.com/raunaksahahere/plantwin/internal/config"
	"github.com/raunaksahahere/plantwin/internal/errs"
	"github.com/raunaksahahere/plantwin/internal/plan"
	"github.com/raunaksahahere/plantwin/internal/report"
	"github.com/raunaksahahere/plantwin/internal/rules"
	"github.com/raunaksahahere/plantwin/internal/twin"
)

// Options is the input to Run.
type Options struct {
	Repo        string // repository root (where plantwin.yaml lives)
	ConfigPath  string
	Base, Head  string // git refs; Head may be "WORKTREE" to test uncommitted changes
	BundlePath  string // overrides bundle.path from the config
	KeepTwins   bool
	Parallel    int
	Verbose     bool
	ToolVersion string
	CacheDir    string // schema cache directory, default <repo>/.plantwin/cache
}

// Outcome is what Run returns: the report input and the configuration it was produced with.
type Outcome struct {
	Run      report.Run
	Cfg      config.Config
	Settings map[string]string
}

// Run executes a full check.
func Run(ctx context.Context, opts Options) (Outcome, error) {
	var out Outcome
	t0 := time.Now()
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return out, err
	}
	out.Cfg = cfg
	if err := twin.RequireDocker(ctx); err != nil {
		return out, err
	}

	bundlePath := opts.BundlePath
	if bundlePath == "" {
		bundlePath = filepath.Join(opts.Repo, cfg.Bundle.Path)
	}
	b, err := loadVerifiedBundle(opts.Repo, bundlePath, cfg.Bundle.PublicKey)
	if err != nil {
		return out, err
	}
	out.Settings = b.Settings
	phases := []report.Phase{report.Ms("load", time.Since(t0))}

	tStart := time.Now()
	baseDir, cleanBase, err := checkout(opts.Repo, opts.Base)
	if err != nil {
		return out, err
	}
	defer cleanBase()
	headDir, cleanHead, err := checkout(opts.Repo, opts.Head)
	if err != nil {
		return out, err
	}
	defer cleanHead()

	runID := newRunID()
	baseTwin, headTwin, err := startBoth(ctx, runID, cfg, baseDir, headDir, opts)
	if err != nil {
		return out, err
	}
	if !opts.KeepTwins {
		defer baseTwin.Stop()
		defer headTwin.Stop()
	}
	phases = append(phases, report.Ms("twins", time.Since(tStart)))

	// Import the same bundle into both twins, then label the estimates.
	tImport := time.Now()
	var warnings []string
	warnings = append(warnings, b.Warnings...)
	var estimated []string
	var sigBase, sigHead map[string]string
	for _, side := range []struct {
		t    *twin.Twin
		sig  *map[string]string
		name string
	}{{baseTwin, &sigBase, opts.Base}, {headTwin, &sigHead, opts.Head}} {
		conn, err := side.t.Connect(ctx)
		if err != nil {
			return out, err
		}
		res := twin.ImportBundle(ctx, conn, b)
		if len(res.Failures) > 0 && side.name == opts.Head {
			warnings = append(warnings, fmt.Sprintf("%d statistics object(s) on %s could not be restored (usually expected for objects the migration changed); they are estimated. First: %s",
				len(res.Failures), opts.Head, res.Failures[0]))
		}
		if _, err := twin.EstimateNewIndexes(ctx, side.t, twin.EstimateSQL); err != nil {
			conn.Close(ctx)
			return out, errs.New("Estimating statistics for new indexes failed on the twin.",
				"Run `plantwin doctor`. The twin image may not match the expected PostgreSQL version.", "%v", err)
		}
		sig, err := twin.TableSignatures(ctx, conn)
		conn.Close(ctx)
		if err != nil {
			return out, err
		}
		*side.sig = sig
	}
	estimated = twin.ChangedTables(sigBase, sigHead)
	phases = append(phases, report.Ms("import", time.Since(tImport)))

	// Explain the catalog on both twins in parallel.
	queries, err := loadQueries(opts.Repo, cfg)
	if err != nil {
		return out, err
	}
	tExplain := time.Now()
	workers := opts.Parallel
	if workers <= 0 {
		workers = 4
	}
	basePlans, baseErrs, err := explainAll(ctx, baseTwin, runID, b.Settings, queries, workers)
	if err != nil {
		return out, err
	}
	headPlans, headErrs, err := explainAll(ctx, headTwin, runID, b.Settings, queries, workers)
	if err != nil {
		return out, err
	}
	phases = append(phases, report.Ms("explain", time.Since(tExplain)))

	rcfg := cfg.Rules()
	// Table sizes from the bundle drive the selectivity rules (ADR-0008).
	rcfg.TableRows = map[string]float64{}
	for _, r := range b.Relations {
		if r.Kind == "r" && r.RelTuples >= 0 {
			rcfg.TableRows[r.Name] = float64(r.RelTuples)
		}
	}
	rcfg.EstimatedRelns = map[string]bool{}
	for _, t := range estimated {
		rcfg.EstimatedRelns[t] = true
	}
	var results []report.QueryResult
	for _, q := range queries {
		qr := report.QueryResult{Name: q.Name}
		if e, ok := baseErrs[q.Name]; ok {
			qr.BaseErr = e.Error()
			// The base query does not plan, so there is nothing to compare against.
			results = append(results, qr)
			continue
		}
		bp := basePlans[q.Name]
		qr.BasePlan = &bp
		in := rules.Input{Query: q.Name, Base: bp}
		if e, ok := headErrs[q.Name]; ok {
			qr.HeadErr = e.Error()
			in.HeadErr = qr.HeadErr
		} else {
			hp := headPlans[q.Name]
			qr.HeadPlan = &hp
			in.Head = hp
		}
		qr.Findings = rules.Analyse(in, rcfg)
		results = append(results, qr)
	}
	accepted, err := LoadAccepted(opts.Repo)
	if err != nil {
		return out, err
	}
	results, nAccepted := applyAccepted(results, accepted)
	if nAccepted > 0 {
		warnings = append(warnings, fmt.Sprintf("%d finding(s) were accepted in .plantwin/accepted.json and are not counted.", nAccepted))
	}
	phases = append(phases, report.Ms("total", time.Since(t0)))

	out.Run = report.Run{
		Base: opts.Base, Head: opts.Head, Queries: results, Timing: phases,
		Estimated: estimated, Warnings: warnings, FailOn: rcfg.FailOn,
	}
	if err := saveLastRun(opts.Repo, out.Run, results); err != nil && opts.Verbose {
		fmt.Fprintln(os.Stderr, "warning: could not save the last run:", err)
	}
	return out, nil
}

// Write renders the report in the requested format.
func Write(w io.Writer, run report.Run, format, toolVersion string) error {
	switch format {
	case "md", "markdown":
		_, err := io.WriteString(w, report.Markdown(run))
		return err
	case "json":
		data, err := report.JSON(run)
		if err != nil {
			return err
		}
		_, err = w.Write(append(data, '\n'))
		return err
	case "sarif":
		data, err := report.SARIF(run, toolVersion)
		if err != nil {
			return err
		}
		_, err = w.Write(append(data, '\n'))
		return err
	}
	return errs.New(fmt.Sprintf("Unknown output format %q.", format), "Use md, json or sarif.", "format")
}

func loadVerifiedBundle(repo, bundlePath, pubKey string) (*bundle.Bundle, error) {
	b, raw, err := bundle.Read(bundlePath)
	if err != nil {
		return nil, errs.New("The stats bundle cannot be read.",
			"Run `plantwin export` against a read replica, or check bundle.path in plantwin.yaml.", "%s: %v", bundlePath, err)
	}
	sig, err := os.ReadFile(bundlePath + ".sig")
	if err != nil {
		return nil, errs.New("The stats bundle is not signed.",
			"Sign it when exporting: `plantwin export --sign-key <key>`. Never run check on an unsigned bundle.", "%s.sig is missing", bundlePath)
	}
	keyPath := pubKey
	if !filepath.IsAbs(keyPath) {
		keyPath = filepath.Join(repo, pubKey)
	}
	if err := bundle.Verify(raw, string(sig), keyPath); err != nil {
		return nil, errs.New("The stats bundle signature does not verify.",
			"The bundle was changed after export, or it was signed with another key. Re-export, or check bundle.public_key.",
			"verify %s with %s: %v", bundlePath, keyPath, err)
	}
	return b, nil
}

// checkout returns a directory with the requested revision. "WORKTREE" means the repository as it is on disk.
func checkout(repo, ref string) (string, func(), error) {
	if ref == "WORKTREE" {
		return repo, func() {}, nil
	}
	// A ref that starts with '-' would be read by git as an option. Refuse it before calling git.
	if strings.HasPrefix(ref, "-") || strings.TrimSpace(ref) == "" {
		return "", func() {}, errs.New(fmt.Sprintf("%q is not a valid git ref.", ref),
			"Use a branch, tag or commit, for example main or HEAD~1.", "invalid ref %q", ref)
	}
	dir, err := os.MkdirTemp("", "plantwin-wt-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		_ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", dir).Run()
		_ = os.RemoveAll(dir)
	}
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "--detach", dir, ref).CombinedOutput(); err != nil {
		cleanup()
		return "", func() {}, errs.New(fmt.Sprintf("Git could not check out %q.", ref),
			"Check the ref exists (`git rev-parse "+ref+"`), and that the working tree is a git repository.",
			"git worktree add: %s", strings.TrimSpace(string(out)))
	}
	return dir, cleanup, nil
}

func startBoth(ctx context.Context, runID string, cfg config.Config, baseDir, headDir string, opts Options) (*twin.Twin, *twin.Twin, error) {
	type res struct {
		t   *twin.Twin
		err error
	}
	start := func(side, dir string) res {
		name := "plantwin-" + runID + "-" + side
		t, err := startAndMigrate(ctx, name, cfg, dir, opts)
		return res{t, err}
	}
	var wg sync.WaitGroup
	var rb, rh res
	wg.Add(2)
	go func() { defer wg.Done(); rb = start("base", baseDir) }()
	go func() { defer wg.Done(); rh = start("head", headDir) }()
	wg.Wait()
	if rb.err != nil || rh.err != nil {
		if rb.t != nil {
			rb.t.Stop()
		}
		if rh.t != nil {
			rh.t.Stop()
		}
		if rb.err != nil {
			return nil, nil, rb.err
		}
		return nil, nil, rh.err
	}
	return rb.t, rh.t, nil
}

// startAndMigrate starts a twin and applies migrations, using the schema cache when the migrations are unchanged.
func startAndMigrate(ctx context.Context, name string, cfg config.Config, dir string, opts Options) (*twin.Twin, error) {
	migDir := filepath.Join(dir, cfg.Migrations.Path)
	hash := ""
	cacheFile := ""
	if cfg.Migrations.Tool == "sql" {
		h, err := twin.SchemaHash(migDir, cfg.Image)
		if err == nil {
			hash = h
			cacheDir := opts.CacheDir
			if cacheDir == "" {
				cacheDir = filepath.Join(opts.Repo, ".plantwin", "cache")
			}
			cacheFile = filepath.Join(cacheDir, h+".sql")
		}
	}
	t, err := twin.Start(ctx, name, cfg.Image)
	if err != nil {
		return nil, err
	}
	if cacheFile != "" {
		if schema, err := os.ReadFile(cacheFile); err == nil {
			if err := twin.ReplaySchema(ctx, t, string(schema)); err == nil {
				return t, nil
			}
			// A stale or broken cache entry is ignored and rebuilt below.
		}
	}
	if err := twin.ApplyMigrations(ctx, t, cfg.Migrations.Tool, migDir, cfg.Migrations.Command); err != nil {
		t.Stop()
		return nil, err
	}
	if cacheFile != "" && hash != "" {
		if schema, err := twin.DumpSchema(ctx, t); err == nil {
			_ = os.MkdirAll(filepath.Dir(cacheFile), 0o755)
			_ = os.WriteFile(cacheFile, []byte(schema), 0o644)
		}
	}
	return t, nil
}

type namedQuery struct {
	Name string
	SQL  string
}

func loadQueries(repo string, cfg config.Config) ([]namedQuery, error) {
	var out []namedQuery
	for _, q := range cfg.Queries {
		sql := q.SQL
		if sql == "" {
			data, err := os.ReadFile(filepath.Join(repo, q.File))
			if err != nil {
				return nil, errs.New(fmt.Sprintf("The SQL file for query %q cannot be read.", q.Name),
					"Check the file path in plantwin.yaml.", "%s: %v", q.File, err)
			}
			sql = string(data)
		}
		out = append(out, namedQuery{Name: q.Name, SQL: strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";"))})
	}
	if len(out) == 0 {
		return nil, errs.New("plantwin.yaml has no queries.", "Add at least one query under `queries:`, or run `plantwin init` again.", "no queries")
	}
	return out, nil
}

// explainAll runs EXPLAIN for every query on one twin, using up to workers connections in parallel.
// It returns the plan for each query that planned, and the error for each query that did not.
// A connection-level failure is returned as the error of the whole call.
func explainAll(ctx context.Context, t *twin.Twin, runID string, settings map[string]string, queries []namedQuery, workers int) (map[string]plan.Node, map[string]error, error) {
	plans := map[string]plan.Node{}
	failed := map[string]error{}
	var mu sync.Mutex
	var fatal error
	jobs := make(chan namedQuery)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := t.Connect(ctx)
			if err == nil {
				err = prelude(ctx, conn, runID, settings)
			}
			if err != nil {
				mu.Lock()
				if fatal == nil {
					fatal = err
				}
				mu.Unlock()
				for range jobs {
				}
				if conn != nil {
					conn.Close(ctx)
				}
				return
			}
			defer conn.Close(ctx)
			for q := range jobs {
				n, err := explainOne(ctx, conn, q.SQL)
				mu.Lock()
				if err != nil {
					failed[q.Name] = err
				} else {
					plans[q.Name] = n
				}
				mu.Unlock()
			}
		}()
	}
	for _, q := range queries {
		jobs <- q
	}
	close(jobs)
	wg.Wait()
	if fatal != nil {
		return nil, nil, fatal
	}
	return plans, failed, nil
}

func explainOne(ctx context.Context, conn *pgx.Conn, sql string) (plan.Node, error) {
	opts := "FORMAT JSON"
	if strings.Contains(sql, "$") {
		opts += ", GENERIC_PLAN"
	}
	var raw []byte
	if err := conn.QueryRow(ctx, "EXPLAIN ("+opts+") "+sql).Scan(&raw); err != nil {
		return plan.Node{}, err
	}
	return plan.Parse(raw)
}

// prelude loads plantwin in a CI-only session and applies the bundle's planner settings.
func prelude(ctx context.Context, conn *pgx.Conn, runID string, settings map[string]string) error {
	if _, err := conn.Exec(ctx, "SELECT set_config('plantwin.ci_marker', $1, false)", "ci:"+runID); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, "LOAD 'plantwin'"); err != nil {
		return errs.New("The plantwin extension is not available in the twin image.",
			"Use an image built from this repository's Dockerfile (for example plantwin-postgres:18), or set `image:` in plantwin.yaml.",
			"LOAD plantwin: %v", err)
	}
	if _, err := conn.Exec(ctx, "SELECT set_config('plantwin.enabled', 'on', false)"); err != nil {
		return err
	}
	for k, v := range settings {
		if _, err := conn.Exec(ctx, "SELECT set_config($1, $2, false)", k, v); err != nil {
			return fmt.Errorf("apply setting %s: %w", k, err)
		}
	}
	return nil
}

func newRunID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Accepted is one finding the team has chosen to accept (see `plantwin accept`).
type Accepted struct {
	Query    string `json:"query"`
	Rule     string `json:"rule"`
	Relation string `json:"relation,omitempty"` // empty matches any relation
}

// LoadAccepted reads .plantwin/accepted.json. A missing file means nothing is accepted.
func LoadAccepted(repo string) ([]Accepted, error) {
	data, err := os.ReadFile(filepath.Join(repo, ".plantwin", "accepted.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Accepted
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, errs.New(".plantwin/accepted.json is not valid JSON.", "Fix or delete the file.", "%v", err)
	}
	return out, nil
}

// AddAccepted appends one accepted finding and writes the file.
func AddAccepted(repo string, a Accepted) error {
	all, err := LoadAccepted(repo)
	if err != nil {
		return err
	}
	all = append(all, a)
	dir := filepath.Join(repo, ".plantwin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "accepted.json"), append(data, '\n'), 0o644)
}

// applyAccepted removes findings that match an accepted entry. It returns the number removed.
func applyAccepted(results []report.QueryResult, accepted []Accepted) ([]report.QueryResult, int) {
	removed := 0
	for i := range results {
		var kept []rules.Finding
		for _, f := range results[i].Findings {
			if isAccepted(f, results[i].Name, accepted) {
				removed++
				continue
			}
			kept = append(kept, f)
		}
		results[i].Findings = kept
	}
	return results, removed
}

func isAccepted(f rules.Finding, query string, accepted []Accepted) bool {
	for _, a := range accepted {
		if a.Query == query && a.Rule == f.Rule && (a.Relation == "" || a.Relation == f.Relation) {
			return true
		}
	}
	return false
}

// LastRun is what `plantwin explain` reads: the report and the plan trees behind it.
type LastRun struct {
	Run   report.Run `json:"run"`
	Plans []PlanPair `json:"plans"`
}

// PlanPair holds both plan trees for one query.
type PlanPair struct {
	Name string     `json:"name"`
	Base *plan.Node `json:"base,omitempty"`
	Head *plan.Node `json:"head,omitempty"`
}

// LoadLastRun reads the last check's output, or returns a user error explaining how to produce one.
func LoadLastRun(repo string) (LastRun, error) {
	var lr LastRun
	data, err := os.ReadFile(filepath.Join(repo, ".plantwin", "last-run.json"))
	if err != nil {
		return lr, errs.New("There is no previous check result to explain.", "Run `plantwin check` first.", "%v", err)
	}
	if err := json.Unmarshal(data, &lr); err != nil {
		return lr, errs.New(".plantwin/last-run.json is not valid.", "Run `plantwin check` again.", "%v", err)
	}
	return lr, nil
}

func saveLastRun(repo string, run report.Run, queries []report.QueryResult) error {
	dir := filepath.Join(repo, ".plantwin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lr := LastRun{Run: run}
	for _, q := range queries {
		lr.Plans = append(lr.Plans, PlanPair{Name: q.Name, Base: q.BasePlan, Head: q.HeadPlan})
	}
	data, err := json.MarshalIndent(lr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "last-run.json"), append(data, '\n'), 0o644)
}
