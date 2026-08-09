package migraterun

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/config"
)

func TestDynamicDeclarationName(t *testing.T) {
	got := dynamicDeclarationName("Migration", "2026_08_08_000002_create_users_table.go")
	if got != "Migration_2026_08_08_000002_Create_Users_Table" {
		t.Fatalf("nome dinâmico inesperado: %s", got)
	}
}

func TestDevelopmentRollbackPlanBlocksTrackedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "gokit", "migrate"), 0o755); err != nil {
		t.Fatal(err)
	}
	tracked := filepath.Join(root, "internal", "gokit", "migrate", "tracked.go")
	untracked := filepath.Join(root, "internal", "gokit", "migrate", "untracked.go")
	for _, path := range []string{tracked, untracked} {
		if err := os.WriteFile(path, []byte("package migrations\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(arguments ...string) {
		t.Helper()
		cmd := exec.Command("git", arguments...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, output)
		}
	}
	git("init", "-q")
	git("add", "internal/gokit/migrate/tracked.go")
	if err := recordGeneratedFile(root, "users", "migration", tracked); err != nil {
		t.Fatal(err)
	}
	if err := recordGeneratedFile(root, "users", "migration", untracked); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanDevelopmentRollback(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || !strings.HasSuffix(plan.Files[0], "untracked.go") {
		t.Fatalf("arquivos removíveis inesperados: %v", plan.Files)
	}
	if len(plan.Blocked) != 1 || !strings.Contains(plan.Blocked[0], "tracked.go") {
		t.Fatalf("arquivos bloqueados inesperados: %v", plan.Blocked)
	}
}

func TestNormalizeLegacyDeclarations(t *testing.T) {
	root := t.TempDir()
	migrations := filepath.Join(root, "migrations")
	seeds := filepath.Join(root, "seeds")
	if err := os.MkdirAll(migrations, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(seeds, 0o755); err != nil {
		t.Fatal(err)
	}
	migrationPath := filepath.Join(migrations, "2026_08_08_000002_create_users.go")
	seedPath := filepath.Join(seeds, "2026_08_08_000003_seeder.go")
	if err := os.WriteFile(migrationPath, []byte("package migrations\nfunc Migration() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seedPath, []byte("package seeds\nfunc Seeder() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := config.ConfigState{Config: &config.Config{Output: config.OutputConfig{Migrate: migrations, Seed: seeds}}}
	changed, err := normalizeLegacyDeclarations(root, state)
	if err != nil || changed != 2 {
		t.Fatalf("normalização: changed=%d err=%v", changed, err)
	}
	for _, path := range []string{migrationPath, seedPath} {
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "func Migration()") || strings.Contains(string(data), "func Seeder()") {
			t.Fatalf("declaração fixa permaneceu em %s", path)
		}
	}
}

func TestFreshReloadGuardsEnvironmentAndRemoteHost(t *testing.T) {
	state := config.ConfigState{ActiveEnv: "production", Config: &config.Config{}}
	if err := ensureDevelopmentResetAllowed(state); err == nil {
		t.Fatal("produção deveria ser bloqueada")
	}
	state.ActiveEnv = "development"
	state.ActiveClient = "mysql"
	state.Config.Connections = map[string]config.ConnConfig{"mysql": {Dialect: "mysql", Host: "database.example.com"}}
	if err := ensureDevelopmentResetAllowed(state); err == nil {
		t.Fatal("host remoto deveria ser bloqueado")
	}
}
