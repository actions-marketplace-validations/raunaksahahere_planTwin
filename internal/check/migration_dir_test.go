package check

import (
	"path/filepath"
	"testing"

	"github.com/raunaksahahere/plantwin/internal/config"
)

func TestMigrationDirByTool(t *testing.T) {
	root := "/work/rev"
	sql := config.Defaults() // tool sql, path migrations
	if got := migrationDir(root, sql); got != filepath.Join(root, "migrations") {
		t.Fatalf("sql tool: got %s", got)
	}
	cmd := config.Defaults()
	cmd.Migrations.Tool = "command"
	cmd.Migrations.Command = "python manage.py migrate"
	// The default path must not leak into a command tool: Django runs from the revision root.
	if got := migrationDir(root, cmd); got != root {
		t.Fatalf("command tool must run in the revision root, got %s", got)
	}
}
