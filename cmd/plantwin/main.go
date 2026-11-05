// Command plantwin shows how a pull request changes PostgreSQL query plans, using production
// planner statistics and no production rows. See README.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/raunaksahahere/plantwin/internal/errs"
)

// Version is set at build time with -ldflags "-X main.Version=...". The release workflow sets it from the tag.
var Version = "1.0.0"

// Exit codes. Scripts can rely on them.
const (
	exitOK          = 0 // checks passed
	exitFindings    = 1 // checks ran and found problems at or above fail_on
	exitUsage       = 2 // bad flags, bad config, or a missing file the user must fix
	exitEnvironment = 3 // Docker or Git is missing or not running
)

var verbose bool

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:]))
}

func run(ctx context.Context, args []string) int {
	// --verbose is accepted anywhere on the command line and removed before dispatch.
	var kept []string
	for _, a := range args {
		if a == "--verbose" || a == "-verbose" {
			verbose = true
			continue
		}
		kept = append(kept, a)
	}
	args = kept
	if len(args) == 0 {
		usage(os.Stderr)
		return exitUsage
	}
	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "init":
		err = cmdInit(rest)
	case "export":
		err = cmdExport(ctx, rest)
	case "keygen":
		err = cmdKeygen(rest)
	case "check":
		err = cmdCheck(ctx, rest)
	case "explain":
		err = cmdExplain(rest)
	case "accept":
		err = cmdAccept(rest)
	case "doctor":
		err = cmdDoctor(ctx, rest)
	case "demo":
		err = cmdDemo(ctx, rest)
	case "version", "--version":
		fmt.Println("plantwin", Version)
		return exitOK
	case "help", "-h", "--help":
		usage(os.Stdout)
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "plantwin: unknown command %q\n\n", cmd)
		usage(os.Stderr)
		return exitUsage
	}
	if err == nil {
		return exitOK
	}
	return reportError(err)
}

// reportError prints a user error plainly, and the underlying error only with --verbose.
func reportError(err error) int {
	var fe findingsError
	if errors.As(err, &fe) {
		return exitFindings
	}
	var user *errs.User
	if errors.As(err, &user) {
		fmt.Fprintln(os.Stderr, "plantwin: "+user.Error())
		if verbose && user.Err != nil {
			fmt.Fprintln(os.Stderr, "  Detail:", user.Err)
		}
		if user.Environment {
			return exitEnvironment
		}
		return exitUsage
	}
	fmt.Fprintln(os.Stderr, "plantwin:", err)
	return exitUsage
}

// findingsError is returned by check when findings meet fail_on. It is not printed as an error.
type findingsError struct{}

func (findingsError) Error() string { return "checks failed" }

func usage(w *os.File) {
	fmt.Fprint(w, `PlanTwin: catch PostgreSQL query-plan regressions in pull requests.

Usage:
  plantwin init                       create plantwin.yaml for this repository
  plantwin export --out FILE          export planner statistics from a read replica (run on production)
  plantwin keygen --priv FILE --pub FILE
  plantwin check [--base REF] [--head REF] [--format md|json|sarif] [--out FILE]
  plantwin explain QUERY              show the base and head plans for one query from the last check
  plantwin accept QUERY RULE [RELATION]
  plantwin doctor                     check Docker, the image, the bundle and the config
  plantwin demo                       run a built-in example that drops an index
  plantwin version

Global flags: --verbose (show underlying errors).
Exit codes: 0 passed, 1 findings failed the check, 2 usage or config error, 3 Docker or Git unavailable.
`)
}
