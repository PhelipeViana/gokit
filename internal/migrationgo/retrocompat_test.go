package migrationgo

// Regressão de retrocompatibilidade do DSL.
//
// Corpus escrito por uma versão anterior continua em disco. Quando um método sai do DSL,
// o que aquele arquivo recebe de volta tem de dizer O QUE FAZER — o corpus é a fonte, e
// mandar o usuário adivinhar é o pior lugar para isso acontecer.
//
// O defeito medido: `tableReference` rodava ANTES do switch de métodos, então
// `migrate.CreateView("v", "SELECT ...")` era diagnosticado como "exige core.Table.*" —
// mandando corrigir o primeiro argumento de um método que não existe mais.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func corpusComUmaMigration(t *testing.T, corpo string) string {
	t.Helper()
	raiz := t.TempDir()
	pasta := filepath.Join(raiz, "create_table")
	if err := os.MkdirAll(pasta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasta, "2020_01_01_000001_antiga.go"), []byte(corpo), 0o644); err != nil {
		t.Fatal(err)
	}
	return raiz
}

func TestViewRemovidaDizParaOndeAResponsabilidadeFoi(t *testing.T) {
	raiz := corpusComUmaMigration(t, `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration_2020_01_01_000001_Antiga() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("antigos", migrate.Col("id").Integer().PrimaryKey()),
		migrate.CreateView("v_antiga", "SELECT 1 AS um"),
	)
}
`)
	_, err := ParseFile(filepath.Join(raiz, "create_table", "2020_01_01_000001_antiga.go"))
	if err == nil {
		t.Fatal("CreateView deveria ser recusado: saiu do DSL")
	}
	mensagem := err.Error()
	// O diagnóstico ERRADO que existia. Se voltar, este teste falha.
	if strings.Contains(mensagem, "core.Table") {
		t.Errorf("o erro não deveria pedir core.Table para um método inexistente:\n%s", mensagem)
	}
	for _, esperado := range []string{"CreateView", "gokit special"} {
		if !strings.Contains(mensagem, esperado) {
			t.Errorf("o erro deveria citar %q:\n%s", esperado, mensagem)
		}
	}
}

func TestIndexDeColunaRemovidoApontaOCreateIndex(t *testing.T) {
	raiz := corpusComUmaMigration(t, `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration_2020_01_01_000001_Antiga() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("antigos", migrate.Col("nome").Varchar(50).Index()),
	)
}
`)
	_, err := ParseFile(filepath.Join(raiz, "create_table", "2020_01_01_000001_antiga.go"))
	if err == nil {
		t.Fatal(".Index() de coluna deveria ser recusado: saiu do DSL")
	}
	if !strings.Contains(err.Error(), "CreateIndex") {
		t.Errorf("o erro deveria apontar o CreateIndex como substituto:\n%s", err)
	}
}

// Método que NUNCA existiu continua sendo "não suportado" — a mensagem específica é só
// para o que saiu, senão qualquer erro de digitação viraria conselho errado.
func TestMetodoInventadoSegueSendoNaoSuportado(t *testing.T) {
	raiz := corpusComUmaMigration(t, `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration_2020_01_01_000001_Antiga() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("antigos", migrate.Col("id").Integer()),
		migrate.InventadoAgora("antigos", "x"),
	)
}
`)
	_, err := ParseFile(filepath.Join(raiz, "create_table", "2020_01_01_000001_antiga.go"))
	if err == nil {
		t.Fatal("método inexistente deveria ser recusado")
	}
	if strings.Contains(err.Error(), "gokit special") {
		t.Errorf("método inventado não deveria receber o conselho de view:\n%s", err)
	}
}
