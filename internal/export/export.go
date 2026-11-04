// Package export reads planner statistics from a production replica and writes a
// stats bundle. It runs only SELECTs against catalogs and pg_stats. It never reads
// table rows. See docs/privacy-and-redaction.md for the exact GRANTs.
package export

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/raunaksahahere/plantwin/internal/bundle"
	"github.com/raunaksahahere/plantwin/internal/errs"
)

// Options controls one export.
type Options struct {
	DSN     string
	Default string            // default redaction mode
	Columns map[string]string // "table.column" -> mode
	Now     time.Time
}

// settingNames are the planner settings the twin must reproduce. Only these are read.
var settingNames = []string{
	"work_mem", "random_page_cost", "seq_page_cost", "cpu_tuple_cost", "cpu_index_tuple_cost",
	"cpu_operator_cost", "effective_cache_size", "default_statistics_target",
	"from_collapse_limit", "join_collapse_limit", "geqo", "geqo_threshold", "geqo_seed",
	"enable_bitmapscan", "enable_hashagg", "enable_hashjoin", "enable_incremental_sort",
	"enable_indexonlyscan", "enable_indexscan", "enable_material", "enable_mergejoin",
	"enable_nestloop", "enable_parallel_append", "enable_parallel_hash", "enable_partitionwise_join",
	"enable_partitionwise_aggregate", "enable_seqscan", "enable_sort", "enable_tidscan",
}

// Run exports a bundle. It returns a user error when the role lacks the privileges it needs.
func Run(ctx context.Context, opts Options) (*bundle.Bundle, error) {
	conn, err := pgx.Connect(ctx, opts.DSN)
	if err != nil {
		return nil, errs.New("Cannot connect to the export database.",
			"Check the DSN, the network route to the replica, and that the role has LOGIN.",
			"connect failed: %v", err)
	}
	defer conn.Close(ctx)

	var canRead bool
	if err := conn.QueryRow(ctx, `SELECT pg_has_role(current_user, 'pg_read_all_stats', 'MEMBER')`).Scan(&canRead); err != nil {
		return nil, fmt.Errorf("checking privileges: %w", err)
	}
	if !canRead {
		return nil, errs.New("The export role cannot read statistics for every table.",
			"Grant the role pg_read_all_stats (see docs/privacy-and-redaction.md, section 'Export role').",
			"role %s lacks pg_read_all_stats", "current_user")
	}

	b := bundle.NewBundle(opts.Now)
	b.Redaction = bundle.RedactionInfo{Default: opts.Default, Columns: opts.Columns}

	var versionNum int
	var version string
	if err := conn.QueryRow(ctx, `SELECT current_setting('server_version_num')::int, current_setting('server_version')`).Scan(&versionNum, &version); err != nil {
		return nil, err
	}
	b.Server = bundle.ServerInfo{VersionNum: versionNum, Version: version}

	rows, err := conn.Query(ctx, `SELECT name, setting FROM pg_settings WHERE name = ANY($1)`, settingNames)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			rows.Close()
			return nil, err
		}
		b.Settings[name] = value
	}
	rows.Close()

	if err := readRelations(ctx, conn, b); err != nil {
		return nil, err
	}
	salt, err := bundle.NewSalt()
	if err != nil {
		return nil, err
	}
	if err := readAttributes(ctx, conn, b, opts, salt); err != nil {
		return nil, err
	}
	if err := readExtendedStats(ctx, conn, b); err != nil {
		return nil, err
	}
	return b, nil
}

func readRelations(ctx context.Context, conn *pgx.Conn, b *bundle.Bundle) error {
	rows, err := conn.Query(ctx, `
		SELECT n.nspname, c.relname, c.relkind::text, c.relpages, c.reltuples, c.relallvisible, c.relallfrozen
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'm', 'i')
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND n.nspname NOT LIKE 'pg_temp%'
		ORDER BY 1, 2`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r bundle.Relation
		if err := rows.Scan(&r.Schema, &r.Name, &r.Kind, &r.RelPages, &r.RelTuples, &r.AllVisible, &r.AllFrozen); err != nil {
			return err
		}
		b.Relations = append(b.Relations, r)
		if r.RelPages > 0 && r.RelTuples < 0 {
			b.Warnings = append(b.Warnings, fmt.Sprintf("%s.%s has pages but no tuple estimate: run ANALYZE on production", r.Schema, r.Name))
		}
	}
	return rows.Err()
}

func readAttributes(ctx context.Context, conn *pgx.Conn, b *bundle.Bundle, opts Options, salt []byte) error {
	rows, err := conn.Query(ctx, `
		SELECT s.schemaname, s.tablename, s.attname, format_type(a.atttypid, a.atttypmod),
		       s.inherited, s.null_frac, s.avg_width, s.n_distinct,
		       s.most_common_vals::text, s.most_common_freqs::text, s.histogram_bounds::text, s.correlation
		FROM pg_stats s
		JOIN pg_namespace n ON n.nspname = s.schemaname
		JOIN pg_class c ON c.relnamespace = n.oid AND c.relname = s.tablename
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = s.attname
		WHERE s.schemaname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY 1, 2, 3, 5`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a bundle.Attribute
		if err := rows.Scan(&a.Schema, &a.Table, &a.Column, &a.DataType, &a.Inherited,
			&a.NullFrac, &a.AvgWidth, &a.NDistinct, &a.MCV, &a.MCVFreqs, &a.Histogram, &a.Correlation); err != nil {
			return err
		}
		mode := opts.Default
		if m, ok := opts.Columns[a.Table+"."+a.Column]; ok {
			mode = m
		}
		if err := bundle.Redact(&a, mode, salt); err != nil {
			return err
		}
		b.Attributes = append(b.Attributes, a)
	}
	return rows.Err()
}

func readExtendedStats(ctx context.Context, conn *pgx.Conn, b *bundle.Bundle) error {
	rows, err := conn.Query(ctx, `
		SELECT n.nspname || '.' || e.stxname
		FROM pg_statistic_ext e JOIN pg_namespace n ON n.oid = e.stxnamespace
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY 1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		b.ExtendedStatistics = append(b.ExtendedStatistics, name)
	}
	if len(b.ExtendedStatistics) > 0 {
		b.Warnings = append(b.Warnings, fmt.Sprintf(
			"%s: extended statistics are not exported (PG18). Multi-column estimates in the twin will differ. See ADR-0004.",
			strings.Join(b.ExtendedStatistics, ", ")))
	}
	return rows.Err()
}
