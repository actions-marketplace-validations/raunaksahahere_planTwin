package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/raunaksahahere/plantwin/internal/bundle"
	"github.com/raunaksahahere/plantwin/internal/check"
	"github.com/raunaksahahere/plantwin/internal/config"
	"github.com/raunaksahahere/plantwin/internal/demo"
	"github.com/raunaksahahere/plantwin/internal/doctor"
	"github.com/raunaksahahere/plantwin/internal/errs"
	"github.com/raunaksahahere/plantwin/internal/export"
	"github.com/raunaksahahere/plantwin/internal/initcmd"
	"github.com/raunaksahahere/plantwin/internal/report"
)

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func cmdInit(args []string) error {
	fs := newFlags("init")
	dir := fs.String("dir", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dets := initcmd.Detect(*dir)
	det := initcmd.Detection{Tool: "none", Name: "none", Note: "No migration tool was found. Set migrations.tool in plantwin.yaml."}
	if len(dets) > 0 {
		det = dets[0]
	}
	path, err := initcmd.Write(*dir, det, filepath.Join("queries", "example_lookup.sql"))
	if err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", path)
	if len(dets) > 0 {
		fmt.Printf("Detected migration tool: %s (%s)\n", det.Name, det.Path)
		if len(dets) > 1 {
			fmt.Printf("Also found: %s. Change migrations in plantwin.yaml if you need one of these.\n", initcmd.Names(dets[1:]))
		}
		if det.Note != "" {
			fmt.Printf("Note: %s\n", det.Note)
		}
	} else {
		fmt.Println("No migration tool detected. Edit migrations in plantwin.yaml.")
	}
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Build the twin image:     docker build -t plantwin-postgres:18 <path-to-plantwin-repo>")
	fmt.Println("  2. Replace the example query in queries/ with queries your application runs.")
	fmt.Println("  3. On a read replica, export the stats bundle (see docs/privacy-and-redaction.md).")
	fmt.Println("  4. Run `plantwin doctor` to check the setup, then `plantwin check`.")
	return nil
}

func cmdExport(ctx context.Context, args []string) error {
	fs := newFlags("export")
	cfgPath := fs.String("config", "plantwin.yaml", "config file (for redaction settings)")
	dsnEnv := fs.String("dsn-env", "PLANTWIN_DSN", "environment variable holding the replica DSN")
	out := fs.String("out", "plantwin-bundle.json", "bundle file to write")
	signKey := fs.String("sign-key", "", "ed25519 private key used to sign the bundle (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *signKey == "" {
		return errs.New("A bundle must be signed, and no key was given.",
			"Create a key with `plantwin keygen`, then pass --sign-key.", "missing --sign-key")
	}
	dsn := os.Getenv(*dsnEnv)
	if dsn == "" {
		return errs.New(fmt.Sprintf("The %s environment variable is empty.", *dsnEnv),
			"Export the replica's connection string into that variable. Use a read-only role (docs/privacy-and-redaction.md).", "no DSN")
	}
	cfg := config.Defaults()
	if _, err := os.Stat(*cfgPath); err == nil {
		loaded, err := config.Load(*cfgPath)
		if err != nil {
			return err
		}
		cfg = loaded
	}
	b, err := export.Run(ctx, export.Options{
		DSN:     dsn,
		Default: cfg.Redaction.Default,
		Columns: cfg.Redaction.Columns,
		Now:     time.Now(),
	})
	if err != nil {
		return err
	}
	if err := bundle.Write(*out, b); err != nil {
		return err
	}
	data, err := os.ReadFile(*out)
	if err != nil {
		return err
	}
	sig, err := bundle.Sign(data, *signKey)
	if err != nil {
		return errs.New("The signing key cannot be read.", "Check the path to the key file, or create a new key with `plantwin keygen`.", "%v", err)
	}
	if err := os.WriteFile(*out+".sig", []byte(sig+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("Exported %d relations and %d column statistics to %s (signed; signature in %s.sig).\n",
		len(b.Relations), len(b.Attributes), *out, *out)
	fmt.Printf("Redaction: default %s", b.Redaction.Default)
	for col, mode := range b.Redaction.Columns {
		fmt.Printf(", %s=%s", col, mode)
	}
	fmt.Println()
	for _, w := range b.Warnings {
		fmt.Println("Warning:", w)
	}
	return nil
}

func cmdKeygen(args []string) error {
	fs := newFlags("keygen")
	priv := fs.String("priv", "plantwin.key", "private key file (keep it out of git)")
	pub := fs.String("pub", "plantwin.pub", "public key file (commit this)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := bundle.GenerateKeys(*priv, *pub); err != nil {
		return errs.New("The key pair could not be written.", "Check that the directory is writable.", "%v", err)
	}
	fmt.Printf("Wrote private key %s (mode 0600) and public key %s.\n", *priv, *pub)
	fmt.Println("Add the private key to .gitignore. Commit the public key and set bundle.public_key to it.")
	return nil
}

func cmdCheck(ctx context.Context, args []string) error {
	fs := newFlags("check")
	cfgPath := fs.String("config", "plantwin.yaml", "config file")
	base := fs.String("base", "main", "base git ref (the branch the pull request targets)")
	head := fs.String("head", "HEAD", "head git ref, or WORKTREE to test uncommitted changes")
	format := fs.String("format", "md", "output format: md, json or sarif")
	outFile := fs.String("out", "", "write the report to this file instead of stdout")
	bundlePath := fs.String("bundle", "", "stats bundle to use (default: bundle.path in the config)")
	keep := fs.Bool("keep", false, "keep the twin containers after the run, for debugging")
	parallel := fs.Int("parallel", 4, "parallel EXPLAIN connections per twin")
	limit := fs.Duration("timeout", 30*time.Minute, "overall time limit for the check; when it is reached the run stops and its containers are removed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *limit <= 0 {
		return errs.New("--timeout must be a positive duration.", "For example --timeout 30m.", "timeout %s", *limit)
	}
	ctx, cancel := context.WithTimeout(ctx, *limit)
	defer cancel()
	absCfg, err := filepath.Abs(*cfgPath)
	if err != nil {
		return err
	}
	repo := filepath.Dir(absCfg)
	outcome, err := check.Run(ctx, check.Options{
		Repo: repo, ConfigPath: absCfg, Base: *base, Head: *head, BundlePath: *bundlePath,
		KeepTwins: *keep, Parallel: *parallel, Verbose: verbose, ToolVersion: Version,
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return errs.New(fmt.Sprintf("The check did not finish within %s.", *limit),
				"Raise --timeout if the migrations or the query catalog are large. If the time goes to one migration, look for a step that waits forever.",
				"deadline exceeded: %v", err)
		}
		return err
	}
	w := io.Writer(os.Stdout)
	if *outFile != "" {
		f, err := os.Create(*outFile)
		if err != nil {
			return errs.New("The report file cannot be written.", "Check the path given to --out.", "%v", err)
		}
		defer f.Close()
		w = f
	}
	if err := check.Write(w, outcome.Run, *format, Version); err != nil {
		return err
	}
	if outcome.Run.Failed() {
		return findingsError{}
	}
	return nil
}

func cmdExplain(args []string) error {
	fs := newFlags("explain")
	repo := fs.String("repo", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errs.New("explain needs one query name.", "Use `plantwin explain <query-name>`. Names are in plantwin.yaml.", "missing query")
	}
	name := fs.Arg(0)
	lr, err := check.LoadLastRun(*repo)
	if err != nil {
		return err
	}
	for _, p := range lr.Plans {
		if p.Name != name {
			continue
		}
		fmt.Printf("Query %s, %s -> %s\n\n", name, lr.Run.Base, lr.Run.Head)
		if p.Base != nil {
			fmt.Printf("%s:\n%s\n", lr.Run.Base, report.TreeText(p.Base))
		}
		if p.Head != nil {
			fmt.Printf("%s:\n%s\n", lr.Run.Head, report.TreeText(p.Head))
		}
		for _, q := range lr.Run.Queries {
			if q.Name != name {
				continue
			}
			if q.HeadErr != "" {
				fmt.Println("The query no longer plans on the head:", q.HeadErr)
			}
			for _, f := range q.Findings {
				fmt.Printf("%s %s: %s\n  Why: %s\n  Fix: %s\n", strings.ToUpper(string(f.Severity)), f.Rule, f.Message, f.Why, f.Fix)
			}
		}
		return nil
	}
	return errs.New(fmt.Sprintf("No query named %q in the last check.", name),
		"Check the name in plantwin.yaml, then run `plantwin check` again.", "unknown query")
}

func cmdAccept(args []string) error {
	fs := newFlags("accept")
	repo := fs.String("repo", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 || fs.NArg() > 3 {
		return errs.New("accept needs a query name and a rule name.",
			"Use `plantwin accept <query> <rule> [relation]`, for example `plantwin accept customer_lookup lost_index orders`.", "bad arguments")
	}
	a := check.Accepted{Query: fs.Arg(0), Rule: fs.Arg(1)}
	if fs.NArg() == 3 {
		a.Relation = fs.Arg(2)
	}
	if !knownRule(a.Rule) {
		return errs.New(fmt.Sprintf("Unknown rule %q.", a.Rule),
			"Rules: lost_index, seq_scan_large, join_method, sort_hash_work_mem, cost_ratio, shape_changed, query_error.", "rule")
	}
	if err := check.AddAccepted(*repo, a); err != nil {
		return err
	}
	fmt.Printf("Accepted %s for query %s", a.Rule, a.Query)
	if a.Relation != "" {
		fmt.Printf(" on %s", a.Relation)
	}
	fmt.Println(". The next check will not count it. The entry is in .plantwin/accepted.json; commit it.")
	return nil
}

func knownRule(r string) bool {
	switch r {
	case "lost_index", "seq_scan_large", "join_method", "sort_hash_work_mem", "cost_ratio", "shape_changed", "query_error":
		return true
	}
	return false
}

func cmdDoctor(ctx context.Context, args []string) error {
	fs := newFlags("doctor")
	cfgPath := fs.String("config", "plantwin.yaml", "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	results := doctor.Run(ctx, *cfgPath)
	failed := false
	for _, r := range results {
		mark := "ok  "
		if !r.OK {
			mark = "FAIL"
			failed = true
		}
		fmt.Printf("[%s] %s: %s\n", mark, r.Name, r.Message)
		if !r.OK && r.Fix != "" {
			fmt.Printf("       Fix: %s\n", r.Fix)
		}
	}
	if failed {
		return findingsError{}
	}
	fmt.Println("All checks passed.")
	return nil
}

func cmdDemo(ctx context.Context, args []string) error {
	fs := newFlags("demo")
	keep := fs.Bool("keep", false, "keep the demo files and containers for inspection")
	format := fs.String("format", "md", "output format: md, json or sarif")
	redaction := fs.String("redaction", "", "override redaction.default for this run: keep, hash, drop-mcv or drop-histogram")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := demo.Prepare(*redaction)
	if err != nil {
		return err
	}
	if !*keep {
		defer os.RemoveAll(dir)
	}
	fmt.Fprintf(os.Stderr, "Demo repository: %s\nCreating a synthetic production database, exporting its statistics, and checking the migration that drops an index.\n", dir)
	outcome, err := demo.Run(ctx, dir, *keep, verbose, Version)
	if err != nil {
		return err
	}
	return check.Write(os.Stdout, outcome.Run, *format, Version)
}
