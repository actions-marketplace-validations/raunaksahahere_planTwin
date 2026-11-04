//go:build integration

package export

// Redaction coverage with a real database: a distinctive value that is a common value in a column must not
// appear in a bundle exported in hash or drop-mcv mode, and must appear in keep mode (so the test cannot pass vacuously).
// Requires the image plantwin-postgres:18. Run with `make test-integration`.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func leaked(b *bundle.Bundle, value string) bool {
	data, _ := json.Marshal(b)
	return strings.Contains(string(data), value)
}
