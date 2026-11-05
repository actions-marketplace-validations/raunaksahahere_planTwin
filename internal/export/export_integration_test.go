//go:build integration

package export

// Redaction coverage with a real database: a distinctive value that is a common value in a column must not
// appear in a bundle exported in hash or drop-mcv mode, and must appear in keep mode (so the test cannot pass vacuously).
// Requires the image plantwin-postgres:18. Run with `make test-integration`.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/raunaksahahere/plantwin/internal/bundle"
	"github.com/raunaksahahere/plantwin/internal/twin"
)

const secret = "secret-email-domain-7f3a.example"

func TestRedactionDoesNotLeakCommonValues(t *testing.T) {
	ctx := context.Background()
	prod, err := twin.Start(ctx, "plantwin-redaction-test", "plantwin-postgres:18")
	if err != nil {
		t.Fatal(err)
	}
	defer prod.Stop()

	conn, err := prod.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	setup := `
		CREATE TABLE accounts (id int PRIMARY KEY, email_domain text NOT NULL, tier int NOT NULL);
		INSERT INTO accounts SELECT g,
		  CASE WHEN g % 2 = 0 THEN '` + secret + `' ELSE 'other' || (g % 50) || '.example' END,
		  g % 5
		FROM generate_series(1, 50000) g;
		ANALYZE accounts;`
	if _, err := conn.Exec(ctx, setup); err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{bundle.Hash, bundle.DropMCV} {
		b, err := Run(ctx, Options{DSN: prod.DSN, Default: mode, Now: time.Now()})
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if leaked(b, secret) {
			t.Fatalf("mode %s leaked the common value %q", mode, secret)
		}
	}

	kept, err := Run(ctx, Options{DSN: prod.DSN, Default: bundle.Keep, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if !leaked(kept, secret) {
		t.Fatal("keep mode should contain the common value; the leak test would be vacuous otherwise")
	}
}

// TestExportRoleWithDocumentedGrants runs the export as the least-privilege role from docs/privacy-and-redaction.md.
// The export must succeed, and the role must be unable to read table rows.
func TestExportRoleWithDocumentedGrants(t *testing.T) {
	ctx := context.Background()
	prod, err := twin.Start(ctx, "plantwin-grants-test", "plantwin-postgres:18")
	if err != nil {
		t.Fatal(err)
	}
	defer prod.Stop()

	admin, err := prod.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	rolePW, err := randomSecret()
	if err != nil {
		t.Fatal(err)
	}
	setup := `
		CREATE TABLE accounts (id int PRIMARY KEY, email_domain text NOT NULL);
		INSERT INTO accounts SELECT g, 'd' || (g % 20) || '.example' FROM generate_series(1, 20000) g;
		ANALYZE accounts;
		CREATE ROLE plantwin_export LOGIN PASSWORD '` + rolePW + `' NOSUPERUSER NOCREATEDB NOCREATEROLE;
		GRANT CONNECT ON DATABASE postgres TO plantwin_export;
		GRANT USAGE ON SCHEMA public TO plantwin_export;
		GRANT SELECT ON pg_statistic TO plantwin_export;`
	if _, err := admin.Exec(ctx, setup); err != nil {
		t.Fatal(err)
	}

	roleDSN := "postgres://plantwin_export:" + rolePW + "@" + hostOf(prod.DSN) + "/postgres?sslmode=disable"
	b, err := Run(ctx, Options{DSN: roleDSN, Default: bundle.Hash, Now: time.Now()})
	if err != nil {
		t.Fatalf("export with the documented role failed: %v", err)
	}
	found := false
	for _, a := range b.Attributes {
		if a.Table == "accounts" && a.Column == "email_domain" {
			found = true
		}
	}
	if !found {
		t.Fatal("the export role should see statistics for accounts.email_domain")
	}

	roleConn, err := pgx.Connect(ctx, roleDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer roleConn.Close(ctx)
	var n int
	if err := roleConn.QueryRow(ctx, "SELECT count(*) FROM accounts").Scan(&n); err == nil {
		t.Fatal("the export role must not be able to read table rows")
	}
}

// hostOf returns host:port from a DSN produced by twin.Start.
func hostOf(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	rest := dsn[at+1:]
	if slash := strings.Index(rest, "/"); slash >= 0 {
		rest = rest[:slash]
	}
	return rest
}

// randomSecret returns a fresh random value for a test role, so no credential is stored in the repository.
func randomSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func leaked(b *bundle.Bundle, value string) bool {
	data, _ := json.Marshal(b)
	return strings.Contains(string(data), value)
}
