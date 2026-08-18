package migraterun

// Regressão da regra de escrita: ler o banco só em projeto EM BRANCO.
//
// A leitura ESCREVE o corpus, e o corpus é a fonte de tudo. Importar por cima de um corpus
// existente mistura duas gerações do leitor no mesmo lugar, e depois não há como atribuir
// uma divergência à origem ou à versão que escreveu.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/config"
)

func estadoComCorpusEm(pasta string) config.ConfigState {
	return config.ConfigState{
		Config: &config.Config{Output: config.OutputConfig{Migrate: pasta}},
	}
}

// Pasta ausente é o caso NORMAL do projeto em branco, não falha de leitura.
func TestCorpusVazioQuandoAPastaNaoExiste(t *testing.T) {
	raiz := t.TempDir()
	nomes, err := migrationsNoCorpus(raiz, estadoComCorpusEm("internal/gokit/migrate"))
	if err != nil {
		t.Fatalf("pasta ausente não deveria ser erro: %v", err)
	}
	if len(nomes) != 0 {
		t.Errorf("esperava corpus vazio, veio %v", nomes)
	}
}

// Conta ARQUIVO, não tabela declarada: migration que não cria tabela (raw_sql, todo, drop)
// também é escrita que já existe, e contar tabelas deixaria essas passarem pela guarda.
func TestCorpusContaArquivoAindaQueNaoDeclareTabela(t *testing.T) {
	raiz := t.TempDir()
	pasta := filepath.Join(raiz, "internal", "gokit", "migrate", "raw_sql")
	if err := os.MkdirAll(pasta, 0o755); err != nil {
		t.Fatal(err)
	}
	corpo := `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration_2020_01_01_000001_SoSQL() migrate.Definition {
	return migrate.Define(migrate.SQL("oracle", "BEGIN NULL; END;"))
}
`
	if err := os.WriteFile(filepath.Join(pasta, "2020_01_01_000001_so_sql.go"), []byte(corpo), 0o644); err != nil {
		t.Fatal(err)
	}
	// Arquivo que não é .go não conta — README e .sql convivem na pasta.
	if err := os.WriteFile(filepath.Join(pasta, "notas.md"), []byte("nada\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	nomes, err := migrationsNoCorpus(raiz, estadoComCorpusEm("internal/gokit/migrate"))
	if err != nil {
		t.Fatal(err)
	}
	if len(nomes) != 1 || nomes[0] != "2020_01_01_000001_so_sql.go" {
		t.Errorf("esperava só a migration .go, veio %v", nomes)
	}
}

// A guarda precisa recusar ANTES de tocar o banco: a mensagem tem de sair mesmo sem
// conexão nenhuma configurada, senão o usuário veria erro de conexão em vez da regra.
func TestImportRecusaCorpusNaoVazioAntesDeAbrirConexao(t *testing.T) {
	raiz := t.TempDir()
	pasta := filepath.Join(raiz, "internal", "gokit", "migrate", "create_table")
	if err := os.MkdirAll(pasta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasta, "2020_01_01_000001_x.go"), []byte("package migrations\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sem `connections`: se a guarda não vier primeiro, o erro será de dialeto/conexão.
	estado := estadoComCorpusEm("internal/gokit/migrate")
	err := MigrateImport(raiz, estado, false)
	if err == nil {
		t.Fatal("corpus não vazio deveria recusar o import")
	}
	if !strings.Contains(err.Error(), "EM BRANCO") {
		t.Errorf("a recusa deveria citar a regra do projeto em branco, veio: %v", err)
	}
}
