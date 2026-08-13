package migrationgo

import (
	"go/parser"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileSimpleCreateTable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gokit_migration_test")
	if err != nil {
		t.Fatalf("falha ao criar pasta temporária: %v", err)
	}
	defer os.RemoveAll(tempDir)

	src := `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

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

// A referência de tabela aceita as duas formas: o pacote dedicado (alias.X, a
// antiga) e o pacote core unificado (core.Table.X, a nova). Aceitar as duas é o
// que evita um dia de virada em que nenhuma migration do corpus compila.
func TestReferenciaDeCatalogoAceitaAsDuasFormas(t *testing.T) {
	casos := []struct {
		fonte     string
		esperado  string
		aceito    bool
		descricao string
	}{
		{"alias.Users", "Users", true, "forma antiga, pacote alias"},
		{"table.Users", "Users", true, "forma antiga, pacote table (legado anterior)"},
		{"core.Table.Users", "Users", true, "forma nova, pacote core unificado"},
		// O apelido do import é livre, então o identificador do pacote não é
		// validado — só o agrupador Table.
		{"app.Table.Users", "Users", true, "forma nova com outro apelido de import"},
		{"core.Column.Users", "", false, "agrupador errado não passa por tabela"},
		{"outro.Users", "", false, "pacote desconhecido na forma antiga"},
		{"Users", "", false, "identificador solto, sem seletor"},
		{`"users"`, "", false, "texto cru em vez de referência"},
	}

	for _, caso := range casos {
		expressao, err := parser.ParseExpr(caso.fonte)
		if err != nil {
			t.Fatalf("%s: fonte inválida %q: %v", caso.descricao, caso.fonte, err)
		}
		obtido, ok := referenciaDeCatalogo(expressao, "Table", []string{"alias", "table"})
		if ok != caso.aceito {
			t.Errorf("%s: %q aceito=%v, esperado %v", caso.descricao, caso.fonte, ok, caso.aceito)
			continue
		}
		if ok && obtido != caso.esperado {
			t.Errorf("%s: %q devolveu %q, esperado %q", caso.descricao, caso.fonte, obtido, caso.esperado)
		}
	}
}

// A view segue a mesma regra, com o agrupador próprio: view.X e core.View.X.
func TestReferenciaDeCatalogoDeView(t *testing.T) {
	casos := map[string]bool{
		"view.SaldoAtual":       true,
		"core.View.SaldoAtual":  true,
		"core.Table.SaldoAtual": false, // agrupador de tabela não vale para view
		"alias.SaldoAtual":      false,
	}
	for fonte, aceito := range casos {
		expressao, err := parser.ParseExpr(fonte)
		if err != nil {
			t.Fatalf("fonte inválida %q: %v", fonte, err)
		}
		if _, ok := referenciaDeCatalogo(expressao, "View", []string{"view"}); ok != aceito {
			t.Errorf("%q aceito=%v, esperado %v", fonte, ok, aceito)
		}
	}
}

// .Index() na coluna foi REMOVIDO do DSL. Ele era lido num único lugar e só
// produzia índice no MySQL — nos outros três não fazia nada, então quem escrevia
// esperando um índice em Postgres, Oracle ou SQL Server não recebia nenhum. A
// forma portátil é migrate.CreateIndex(tabela, nome, colunas...), que é tratada no
// executor, na validação e no rollback, e leva nome explícito (o Oracle trunca
// identificador em 30 e o nome de índice é único por schema nele e no SQL Server).
//
// O teste garante que quem tinha .Index() escrito recebe erro NOMEADO no validate,
// e não silêncio.
func TestIndexNaColunaFoiRemovidoEAvisa(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gokit_index_removido")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	src := `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("users",
			migrate.Col("id").Integer().PrimaryKey().AutoIncrement(),
			migrate.Col("email").Varchar(255).Index(),
		),
	)
}
`
	caminho := filepath.Join(tempDir, "2026_01_01_000001_users.go")
	if err := os.WriteFile(caminho, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = ParseFile(caminho)
	if err == nil {
		t.Fatal(".Index() na coluna deveria ser recusado pelo parser")
	}
	if !strings.Contains(err.Error(), "Index") {
		t.Fatalf("o erro deveria citar o método recusado, veio: %v", err)
	}
}
