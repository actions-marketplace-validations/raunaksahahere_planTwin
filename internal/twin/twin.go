// Package twin runs a PostgreSQL container, applies migrations, imports the stats
// bundle, and labels the parts of the schema whose statistics are estimated.
// Docker is driven through the docker CLI, so the CLI needs no Docker SDK.
package twin

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/raunaksahahere/plantwin/internal/bundle"
	"github.com/raunaksahahere/plantwin/internal/errs"
)

// Twin is one running PostgreSQL container.
type Twin struct {
	Name  string
	DSN   string
	Image string
}

// RequireDocker returns a user error when the docker CLI or daemon is not available.
func RequireDocker(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput()
	if err != nil {
		return errs.Env("Docker is not running or not installed.",
			"Install Docker, start the daemon, and run `plantwin doctor` to check.",
			"docker info failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Start runs a container from image, waits until PostgreSQL accepts connections, and returns the twin.
func Start(ctx context.Context, name, image string) (*Twin, error) {
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
	// A fresh random password per container. It exists only for the life of the container and is never printed.
	pw, err := randomPassword()
	if err != nil {
		return nil, err
	}
	out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--name", name,
		"-e", "POSTGRES_PASSWORD="+pw,
		"-p", "127.0.0.1::5432", image, "-c", "autovacuum=off").CombinedOutput()
	if err != nil {
		return nil, errs.New(fmt.Sprintf("Could not start the PostgreSQL container from image %q.", image),
			"Check that the image exists locally (`docker image ls`). Build it with `docker build -t plantwin-postgres:18 .`, or set `image:` in plantwin.yaml.",
			"docker run failed: %s", strings.TrimSpace(string(out)))
	}
	port, err := hostPort(ctx, name)
	if err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("postgres://postgres:%s@127.0.0.1:%s/postgres?sslmode=disable", pw, port)
	t := &Twin{Name: name, DSN: dsn, Image: image}
	if err := t.waitReady(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

// Stop removes the container.
func (t *Twin) Stop() {
	_ = exec.Command("docker", "rm", "-f", t.Name).Run()
}

// randomPassword returns 24 random bytes, hex encoded.
func randomPassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hostPort(ctx context.Context, name string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "port", name, "5432/tcp").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker port: %s", strings.TrimSpace(string(out)))
	}
	line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	idx := strings.LastIndex(line, ":")
	if idx < 0 {
		return "", fmt.Errorf("unexpected docker port output %q", line)
	}
	return line[idx+1:], nil
}

func (t *Twin) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(90 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		c, err := pgx.Connect(ctx, t.DSN)
		if err == nil {
			var one int
			err = c.QueryRow(ctx, "SELECT 1").Scan(&one)
			_ = c.Close(ctx)
			if err == nil {
				return nil
			}
		}
		last = err
		time.Sleep(time.Second)
	}
	return errs.New("The PostgreSQL container did not become ready within 90 seconds.",
		"Run `docker logs "+t.Name+"` to see why it failed to start.", "readiness timeout: %v", last)
}

// Connect opens a connection to the twin's database.
func (t *Twin) Connect(ctx context.Context) (*pgx.Conn, error) {
	return pgx.Connect(ctx, t.DSN)
}

// ApplyMigrations runs the migration tool at dir. tool is "sql", "command" or "none".
func ApplyMigrations(ctx context.Context, t *Twin, tool, dir, command string) error {
	switch tool {
	case "none":
		return nil
	case "sql":
		files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
		if err != nil {
			return err
		}
		sort.Strings(files)
		if len(files) == 0 {
			return errs.New(fmt.Sprintf("No .sql migration files were found in %s.", dir),
				"Check migrations.path in plantwin.yaml, or create the first migration.", "no files")
		}
		conn, err := t.Connect(ctx)
		if err != nil {
			return err
		}
		defer conn.Close(ctx)
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			if _, err := conn.Exec(ctx, string(data)); err != nil {
				return errs.New(fmt.Sprintf("Migration %s failed on the twin.", filepath.Base(f)),
					"Run the migration against an empty database to see the full error. The migration may depend on data or extensions the twin lacks.",
					"exec %s: %v", filepath.Base(f), err)
			}
		}
		return nil
	case "command":
		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "DATABASE_URL="+t.DSN)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return &errs.User{
				What: "The migration command failed on the twin.",
				Why:  "Its output ends with: " + lastLines(stderr.String(), 4),
				Fix:  "Run the command by hand with DATABASE_URL pointing at a fresh database to see the full error.",
				Err:  fmt.Errorf("command %q: %v", command, err),
			}
		}
		return nil
	}
	return fmt.Errorf("unknown migration tool %q", tool)
}

// ImportResult summarises a stats import.
type ImportResult struct {
	Relations  int
	Attributes int
	Failures   []string // objects whose stats could not be restored (kept for the report)
}

// ImportBundle restores relation and attribute statistics. Individual failures are recorded, not fatal:
// objects that changed in the migration are expected to fail.
func ImportBundle(ctx context.Context, conn *pgx.Conn, b *bundle.Bundle) ImportResult {
	var res ImportResult
	ver := b.Server.VersionNum
	for _, r := range b.Relations {
		_, err := conn.Exec(ctx,
			`SELECT pg_catalog.pg_restore_relation_stats('version', $1::integer, 'schemaname', $2::text, 'relname', $3::text,
			  'relpages', $4::integer, 'reltuples', $5::real, 'relallvisible', $6::integer, 'relallfrozen', $7::integer)`,
			ver, r.Schema, r.Name, r.RelPages, r.RelTuples, r.AllVisible, r.AllFrozen)
		if err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("relation %s.%s: %v", r.Schema, r.Name, err))
			continue
		}
		res.Relations++
	}
	for _, a := range b.Attributes {
		sql, args := attributeCall(ver, a)
		if _, err := conn.Exec(ctx, sql, args...); err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("column %s.%s.%s: %v", a.Schema, a.Table, a.Column, err))
			continue
		}
		res.Attributes++
	}
	return res
}

// attributeCall builds pg_restore_attribute_stats with only the keys that have values.
func attributeCall(ver int, a bundle.Attribute) (string, []any) {
	var parts []string
	var args []any
	add := func(key, cast string, v any) {
		args = append(args, v)
		parts = append(parts, fmt.Sprintf("'%s', $%d::%s", key, len(args), cast))
	}
	add("version", "integer", ver)
	add("schemaname", "text", a.Schema)
	add("relname", "text", a.Table)
	add("attname", "text", a.Column)
	add("inherited", "boolean", a.Inherited)
	add("null_frac", "real", a.NullFrac)
	add("avg_width", "integer", a.AvgWidth)
	add("n_distinct", "real", a.NDistinct)
	if a.MCV != nil {
		add("most_common_vals", "text", *a.MCV)
	}
	if a.MCVFreqs != nil {
		add("most_common_freqs", "real[]", *a.MCVFreqs)
	}
	if a.Histogram != nil {
		add("histogram_bounds", "text", *a.Histogram)
	}
	if a.Correlation != nil {
		add("correlation", "real", *a.Correlation)
	}
	return "SELECT pg_catalog.pg_restore_attribute_stats(" + strings.Join(parts, ", ") + ")", args
}

// EstimateNewIndexes runs the embedded estimator and returns the names of indexes it sized.
func EstimateNewIndexes(ctx context.Context, t *Twin, sql string) ([]string, error) {
	var names []string
	cfg, err := pgx.ParseConfig(t.DSN)
	if err != nil {
		return nil, err
	}
	cfg.Config.OnNotice = func(_ *pgconn.PgConn, n *pgconn.Notice) {
		if strings.HasPrefix(n.Message, "estimated: ") {
			names = append(names, strings.TrimPrefix(n.Message, "estimated: "))
		}
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, sql); err != nil {
		return nil, fmt.Errorf("estimator failed: %w", err)
	}
	return names, nil
}

// TableSignatures returns one signature per public table (columns and index definitions).
// Comparing base and head signatures finds the tables the migration changed.
func TableSignatures(ctx context.Context, conn *pgx.Conn) (map[string]string, error) {
	rows, err := conn.Query(ctx, `
		SELECT c.relname, md5(
		  coalesce((SELECT string_agg(a.attname || ':' || format_type(a.atttypid, a.atttypmod) || ':' || a.attnotnull::text, ',' ORDER BY a.attname)
		            FROM pg_attribute a WHERE a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped), '') || '|' ||
		  coalesce((SELECT string_agg(pg_get_indexdef(i.indexrelid), ';' ORDER BY pg_get_indexdef(i.indexrelid))
		            FROM pg_index i WHERE i.indrelid = c.oid), ''))
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r' AND n.nspname = 'public'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, sig string
		if err := rows.Scan(&name, &sig); err != nil {
			return nil, err
		}
		out[name] = sig
	}
	return out, rows.Err()
}

// ChangedTables returns the tables whose signature differs between base and head, or that exist only in head.
func ChangedTables(base, head map[string]string) []string {
	var out []string
	for name, sig := range head {
		if base[name] != sig {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// SchemaHash identifies a migration set. Equal hashes mean equal schemas for the same image.
func SchemaHash(dir string, image string) (string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	h.Write([]byte(image + "\n"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h.Write([]byte(filepath.Base(f) + "\n"))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// DumpSchema returns the schema-only dump of the twin's database, with psql meta-commands removed
// so it can be replayed through pgx. Used as the schema cache (ADR-0012).
func DumpSchema(ctx context.Context, t *Twin) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "exec", t.Name, "pg_dump", "-U", "postgres", "--schema-only", "postgres").Output()
	if err != nil {
		return "", fmt.Errorf("pg_dump failed: %w", err)
	}
	var b strings.Builder
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "\\") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String(), sc.Err()
}

// ReplaySchema executes a schema dump produced by DumpSchema.
func ReplaySchema(ctx context.Context, t *Twin, schema string) error {
	conn, err := t.Connect(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, schema)
	return err
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidIdentifier reports whether s is a plain SQL identifier. Names from config are checked before use.
func ValidIdentifier(s string) bool { return identRe.MatchString(s) }

// lastLines returns the last n non-empty lines of s, joined with " | ", for short error messages.
func lastLines(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 0 {
		return "(no output)"
	}
	return strings.Join(lines, " | ")
}
