package migraterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
)

// A normalização é uma só. Antes havia duas, e a divergência só não machucava
// porque o validador de nome físico barrava os casos onde elas discordavam.
func TestNormalizacaoDeIdentificadorEUnica(t *testing.T) {
	casos := map[string]string{
		"users":     "Users",
		"estado_id": "EstadoId",
		"MIL_PST":   "MilPst",
		"cod-ref":   "CodRef",
		"valor tot": "ValorTot",
		"":          "",
	}
	for entrada, esperado := range casos {
		if obtido := migrationgo.ExportedIdentifier(entrada); obtido != esperado {
			t.Errorf("ExportedIdentifier(%q) = %q, esperado %q", entrada, obtido, esperado)
		}
	}
	if obtido := migrationgo.UnexportedIdentifier("estado_id"); obtido != "estadoId" {
		t.Errorf("UnexportedIdentifier: obtido %q", obtido)
	}
	// A do gerador da ORM delega para a mesma, então não pode divergir.
	for entrada := range casos {
		if exportedORMIdentifier(entrada) != migrationgo.ExportedIdentifier(entrada) {
			t.Errorf("as duas normalizações divergem em %q", entrada)
		}
	}
}

// Duas colunas que PASSAM no validador de nome físico e colidem no identificador
// Go. Sem a guarda, o arquivo gerado sai com campo repetido e não compila — e o
// gerador não reclamava.
func TestColunasQueColidemSaoRecusadas(t *testing.T) {
	root := t.TempDir()
	migrations := filepath.Join(root, "database", "migrations")
	if err := os.MkdirAll(migrations, 0o755); err != nil {
		t.Fatal(err)
	}
	fonte := `package migrations
import migrate "github.com/PhelipeViana/gokit/migration"
func Migration() migrate.Definition {
	return migrate.Define(migrate.CreateTable("t",
		migrate.Col("id").Integer().PrimaryKey(),
		migrate.Col("nivel_1").Varchar(10).Nullable(),
		migrate.Col("nivel1").Varchar(10).Nullable(),
	).Alias("t"))
}
`
	if err := os.WriteFile(filepath.Join(migrations, "2026_01_01_000001_t.go"), []byte(fonte), 0o644); err != nil {
		t.Fatal(err)
	}
	state := config.ConfigState{Config: &config.Config{Output: config.OutputConfig{
		Migrate: "database/migrations", ORM: "internal/gokit/core"}}}

	_, err := GenerateORM(root, state)
	if err == nil {
		t.Fatal("colunas que colidem no identificador deveriam ser recusadas")
	}
	for _, esperado := range []string{"nivel_1", "nivel1", "Nivel1", "renomeie"} {
		if !strings.Contains(err.Error(), esperado) {
			t.Errorf("a mensagem deveria conter %q: %v", esperado, err)
		}
	}
	// E nada foi escrito no disco.
	if _, err := os.Stat(filepath.Join(root, "internal/gokit/core/entities.gen.go")); err == nil {
		t.Error("o arquivo não deveria ter sido gerado")
	}
}
