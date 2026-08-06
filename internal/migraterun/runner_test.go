package migraterun

import (
	"os"
	"path/filepath"
	"testing"

	"gokit/internal/config"
	"gokit/internal/migrationgo"
)

func TestCreateScaffoldMigrationAllMethods(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gokit_scaffold_test")
	if err != nil {
		t.Fatalf("falha ao criar pasta temporária: %v", err)
	}
	defer os.RemoveAll(tempDir)

	state := config.ConfigState{
		Config: &config.Config{
			Output: config.OutputConfig{
				Migrate: tempDir,
			},
		},
	}

	methods := []string{
		"create_table", "drop_table", "add_column", "alter_column", "drop_column",
		"add_foreign_key", "drop_foreign_key", "create_index", "drop_index",
		"create_view", "alter_view", "drop_view", "create_sequence", "drop_sequence",
		"rename_table", "rename_column", "add_primary_key", "add_unique", "add_check",
		"drop_constraint", "raw_sql", "todo",
	}

	// Cria go.mod no diretório temporário para ser reconhecido como raiz do projeto
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module temp_test_project\ngo 1.25"), 0644); err != nil {
		t.Fatalf("falha ao criar go.mod: %v", err)
	}

	// Escreve dsl.gen.go fictício de aliases de tabela no Core
	aliasDir := filepath.Join(tempDir, "internal", "gokit", "core", "migration", "alias")
	if err := os.MkdirAll(aliasDir, 0755); err != nil {
		t.Fatalf("falha ao criar pasta de catálogo de aliases: %v", err)
	}
	dslGenContent := `package alias
import migrate "gokit/migration"
var Users = migrate.Table("users")
`
	if err := os.WriteFile(filepath.Join(aliasDir, "dsl.gen.go"), []byte(dslGenContent), 0644); err != nil {
		t.Fatalf("falha ao criar dsl.gen.go fictício: %v", err)
	}

	// Escreve dsl.gen.go fictício para aliases de views
	viewDir := filepath.Join(tempDir, "internal", "gokit", "core", "migration", "view")
	if err := os.MkdirAll(viewDir, 0755); err != nil {
		t.Fatalf("falha ao criar pasta de catálogo de views: %v", err)
	}
	viewGenContent := `package view
import migrate "gokit/migration"
var VwUsers = migrate.RegisteredView("vw_users")
`
	if err := os.WriteFile(filepath.Join(viewDir, "dsl.gen.go"), []byte(viewGenContent), 0644); err != nil {
		t.Fatalf("falha ao criar catálogo de views fictício: %v", err)
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			filename, err := CreateScaffoldMigration(".", state, "users", method, "")
			if err != nil {
				t.Fatalf("falha ao gerar scaffold para %s: %v", method, err)
			}

			filePath := filepath.Join(tempDir, filename)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Errorf("arquivo %s não foi criado", filename)
			}

			// Executa parser AST para garantir que a sintaxe gerada é 100% válida e compatível
			ops, err := migrationgo.ParseFile(filePath)
			if err != nil {
				t.Errorf("erro de sintaxe/parsing AST no scaffold gerado para %s: %v", method, err)
			}

			if len(ops) == 0 {
				t.Errorf("nenhuma operação identificada para %s", method)
			}
		})
	}
}

func TestLoadCatalogTablesAndViews(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gokit_load_catalog_test")
	if err != nil {
		t.Fatalf("falha ao criar pasta temporária: %v", err)
	}
	defer os.RemoveAll(tempDir)

	state := config.ConfigState{
		Config: &config.Config{
			Output: config.OutputConfig{
				Migrate: tempDir,
			},
		},
	}

	// Cria go.mod fictício
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module load_test\ngo 1.25"), 0644); err != nil {
		t.Fatalf("falha ao criar go.mod: %v", err)
	}

	// Escreve dsl.gen.go de aliases de tabela no Core
	aliasDir := filepath.Join(tempDir, "internal", "gokit", "core", "migration", "alias")
	if err := os.MkdirAll(aliasDir, 0755); err != nil {
		t.Fatalf("falha ao criar pasta de catálogo de aliases: %v", err)
	}
	dslGenContent := `package alias
import migrate "gokit/migration"
var Roles = migrate.Table("roles")
var Users = migrate.Table("users")
`
	if err := os.WriteFile(filepath.Join(aliasDir, "dsl.gen.go"), []byte(dslGenContent), 0644); err != nil {
		t.Fatalf("falha ao criar dsl.gen.go: %v", err)
	}

	// Escreve dsl.gen.go de aliases de views
	viewDir := filepath.Join(tempDir, "internal", "gokit", "core", "migration", "view")
	if err := os.MkdirAll(viewDir, 0755); err != nil {
		t.Fatalf("falha ao criar pasta de catálogo de views: %v", err)
	}
	viewGenContent := `package view
import migrate "gokit/migration"
var VwUsers = migrate.RegisteredView("vw_users")
var VwReports = migrate.RegisteredView("vw_reports")
`
	if err := os.WriteFile(filepath.Join(viewDir, "dsl.gen.go"), []byte(viewGenContent), 0644); err != nil {
		t.Fatalf("falha ao criar catálogo de views: %v", err)
	}

	tables, views, err := LoadCatalogTablesAndViews(tempDir, state)
	if err != nil {
		t.Fatalf("falha ao carregar catálogo: %v", err)
	}

	if len(tables) != 2 || tables[0] != "Roles" || tables[1] != "Users" {
		t.Errorf("tabelas carregadas incorretamente: %v", tables)
	}

	if len(views) != 2 || views[0] != "VwReports" || views[1] != "VwUsers" {
		t.Errorf("views carregadas incorretamente: %v", views)
	}
}
