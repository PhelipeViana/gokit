package migraterun

import (
	"strings"
	"testing"
	"time"

	"github.com/PhelipeViana/gokit/migration/acao"
)

func TestDocumentationBuildsCurrentSchema(t *testing.T) {
	files := []migrationFile{
		{ID: "2026_01_01_000001", Plan: Plan{Operations: []acao.Operacao{{
			Kind: "create_table", Table: "users", AliasName: "users",
			Columns: []acao.ColunaDefinicao{{Name: "id", Type: "integer", PrimaryKey: true}},
		}}}},
		{ID: "2026_01_01_000002", Plan: Plan{Operations: []acao.Operacao{{
			Kind: "add_column", Table: "users", Column: &acao.ColunaDefinicao{Name: "email", Type: "string", Length: 255, Unique: true},
		}}}},
	}
	document := renderDatabaseDocumentation(files, time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local))
	for _, expected := range []string{"Tabela: `users`", "`id`", "`email`", "`STRING(255)`", "🔑 PK"} {
		if !strings.Contains(document, expected) {
			t.Fatalf("database.md não contém %q:\n%s", expected, document)
		}
	}
}

func TestMigrationsDocumentationUsesDescendingOrder(t *testing.T) {
	files := []migrationFile{
		{ID: "2026_01_01_000001", Name: "primeira.go"},
		{ID: "2026_01_01_000003", Name: "terceira.go"},
		{ID: "2026_01_01_000002", Name: "segunda.go"},
	}
	metadata := map[string]migrationDocumentationMeta{
		"2026_01_01_000001": {Author: "Ana", Link: "../migrate/primeira.go"},
		"2026_01_01_000002": {Author: "Bruno", Link: "../migrate/segunda.go"},
		"2026_01_01_000003": {Author: "Carla", Link: "../migrate/terceira.go"},
	}
	document := renderMigrationsDocumentation(files, metadata, time.Now())
	third := strings.Index(document, "terceira.go")
	second := strings.Index(document, "segunda.go")
	first := strings.Index(document, "primeira.go")
	if third < 0 || !(third < second && second < first) {
		t.Fatalf("ordem deveria ser decrescente:\n%s", document)
	}
	if !strings.Contains(document, "[`terceira.go`](../migrate/terceira.go)") || !strings.Contains(document, "**Criada por:** Carla") {
		t.Fatalf("migration deveria ter link e autora:\n%s", document)
	}
}
