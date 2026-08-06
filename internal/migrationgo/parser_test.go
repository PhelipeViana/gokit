package migrationgo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFileSimpleCreateTable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gokit_migration_test")
	if err != nil {
		t.Fatalf("falha ao criar pasta temporária: %v", err)
	}
	defer os.RemoveAll(tempDir)

	src := `package migrations

import migrate "gokit/migration"

func Migration() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("users",
			migrate.Col("id").Integer().PrimaryKey().AutoIncrement(),
			migrate.Col("name").Varchar(255).Nullable(),
		).Alias("usuarios"),
	)
}
`
	migrationFile := filepath.Join(tempDir, "2026_08_06_120000_create_users_table.go")
	if err := os.WriteFile(migrationFile, []byte(src), 0644); err != nil {
		t.Fatalf("falha ao escrever arquivo de teste: %v", err)
	}

	ops, err := ParseFile(migrationFile)
	if err != nil {
		t.Fatalf("falha ao parsear arquivo de migration: %v", err)
	}

	if len(ops) != 1 {
		t.Fatalf("esperada 1 operação, obtidas: %d", len(ops))
	}

	op := ops[0]
	if op.Kind != "create_table" {
		t.Errorf("Kind esperado 'create_table', obtido: %s", op.Kind)
	}

	if op.Table != "users" {
		t.Errorf("Table esperado 'users', obtido: %s", op.Table)
	}

	if op.AliasName != "usuarios" {
		t.Errorf("AliasName esperado 'usuarios', obtido: %s", op.AliasName)
	}

	if len(op.Columns) != 2 {
		t.Fatalf("esperadas 2 colunas, obtidas: %d", len(op.Columns))
	}

	if op.Columns[0].Name != "id" || op.Columns[0].Type != "integer" || !op.Columns[0].PrimaryKey || !op.Columns[0].AutoIncrement {
		t.Errorf("primeira coluna incorreta: %+v", op.Columns[0])
	}

	if op.Columns[1].Name != "name" || op.Columns[1].Type != "string" || op.Columns[1].Length != 255 || !op.Columns[1].Nullable {
		t.Errorf("segunda coluna incorreta: %+v", op.Columns[1])
	}
}
