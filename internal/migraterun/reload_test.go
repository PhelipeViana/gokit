package migraterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/config"
)

func TestContainsComposeService(t *testing.T) {
	services := "toolchain\nmysql\npostgres\n"
	if !containsComposeService(services, "mysql") {
		t.Fatal("serviço mysql deveria ser encontrado")
	}
	if containsComposeService(services, "sql") {
		t.Fatal("a busca deve comparar o nome completo do serviço")
	}
}

func TestReloadStartsDatabaseBeforePing(t *testing.T) {
	pipeline := InitReloadPipeline()
	startIndex, pingIndex := -1, -1
	for index, step := range pipeline.Group1 {
		switch step.Name {
		case "start_database":
			startIndex = index
		case "check_conn":
			pingIndex = index
		}
	}
	if startIndex < 0 || pingIndex < 0 || startIndex >= pingIndex {
		t.Fatalf("ordem inválida: start_database=%d check_conn=%d", startIndex, pingIndex)
	}
}

func TestGenerateORMRebuildsMappingsFromMigrationSchema(t *testing.T) {
	root := t.TempDir()
	migrations := filepath.Join(root, "database", "migrations")
	if err := os.MkdirAll(migrations, 0o755); err != nil {
		t.Fatal(err)
	}
	schema := `package migrations
import migrate "github.com/PhelipeViana/gokit/migration"
func Migration() migrate.Definition { return migrate.Define(migrate.CreateTable("users", migrate.Col("id").Integer().PrimaryKey(), migrate.Col("name").Varchar(100).Nullable()).Alias("users")) }
`
	if err := os.WriteFile(filepath.Join(migrations, "2026_01_01_000001_users.go"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	state := config.ConfigState{Config: &config.Config{Output: config.OutputConfig{Migrate: "database/migrations", ORM: "internal/gokit/core/orm"}}}
	count, err := GenerateORM(root, state)
	if err != nil || count != 1 {
		t.Fatalf("GenerateORM: count=%d err=%v", count, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "internal/gokit/core/orm/fields.gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"var UsersField", "Column: \"id\"", "var UsersFilter", "var UsersModel"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("gerado sem %q:\n%s", expected, data)
		}
	}
}
