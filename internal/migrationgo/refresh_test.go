package migrationgo

// Regressão do catálogo de tabelas: quem escreve o `table.gen.go` tem de escrever a
// MESMA coisa, sempre.
//
// Havia dois caminhos com semânticas diferentes. `WriteCoreCatalog` deriva do corpus e
// grava por APELIDO, com guarda de colisão. `RefreshCatalog` semeava do catálogo
// anterior e gravava por NOME, tratando apelido e nome físico como a mesma coisa, sem
// guarda nenhuma.
//
// Qual dos dois vence dependia da ORDEM: no `migrate run` o primeiro roda depois e
// conserta; no `reload` o passo `refresh_catalog` é do Grupo 3 e roda por ÚLTIMO. Ou
// seja, o comando que existe para "cuidar de tudo" era o que desfazia o desempate de
// apelido — em silêncio, porque a escrita por nome não detecta colisão.
//
// O critério que resolve é o do projeto: a migration é a fonte. O catálogo reflete o
// corpus, e o apelido que o corpus declara é o que vale.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// escreverFixtureDoCorpus monta um corpus mínimo com o caso que morde: dois nomes
// físicos que colapsam no mesmo identificador Go, um deles desempatado por apelido.
func escreverFixtureDoCorpus(t *testing.T) (raiz, pastaDeMigrations string) {
	t.Helper()
	raiz = t.TempDir()
	pastaDeMigrations = filepath.Join(raiz, "internal", "gokit", "migrate", "create_table")
	if err := os.MkdirAll(pastaDeMigrations, 0o755); err != nil {
		t.Fatal(err)
	}

	// `eventos_bkp435037` e `eventos_bkp_435037` geram os dois `EventosBkp435037`.
	// O segundo recebeu apelido no corpus, que é onde o desempate mora.
	migrations := map[string]string{
		"2026_01_01_000001_a.go": `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration_2026_01_01_000001_A() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("eventos_bkp435037", migrate.Col("id").Integer()).Alias("eventos_bkp435037"),
	)
}
`,
		"2026_01_01_000002_b.go": `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration_2026_01_01_000002_B() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("eventos_bkp_435037", migrate.Col("id").Integer()).Alias("eventos_bkp_435037_alias2"),
	)
}
`,
	}
	for nome, corpo := range migrations {
		if err := os.WriteFile(filepath.Join(pastaDeMigrations, nome), []byte(corpo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return raiz, filepath.Dir(pastaDeMigrations)
}

// O catálogo escrito pelo RefreshCatalog tem de preservar o apelido do corpus e manter
// os DOIS identificadores. Antes ele colapsava os dois em um, silenciosamente.
func TestRefreshCatalogPreservaApelidoEOsDoisIdentificadores(t *testing.T) {
	raiz, pastaDeMigrations := escreverFixtureDoCorpus(t)

	if err := RefreshCatalog(raiz, pastaDeMigrations); err != nil {
		t.Fatalf("RefreshCatalog: %v", err)
	}

	dados, err := os.ReadFile(caminhoDoCatalogo(raiz))
	if err != nil {
		t.Fatalf("ler o catálogo: %v", err)
	}
	catalogo := string(dados)

	for _, esperado := range []string{
		`EventosBkp435037:`,
		`EventosBkp435037Alias2:`,
		`migrate.Table("eventos_bkp435037")`,
		`migrate.Table("eventos_bkp_435037_alias2")`,
	} {
		if !strings.Contains(catalogo, esperado) {
			t.Errorf("o catálogo deveria conter %q\n\n%s", esperado, catalogo)
		}
	}

	// O nome FÍSICO da tabela apelidada só sobrevive no comentário — é dele que a
	// resolução nome físico → identificador depende.
	if !strings.Contains(catalogo, "// physical: eventos_bkp_435037") {
		t.Errorf("o comentário physical da tabela apelidada deveria estar no catálogo\n\n%s", catalogo)
	}
}

// O catálogo reflete o CORPUS: tabela que saiu do corpus sai do catálogo.
//
// Antes o RefreshCatalog semeava do catálogo anterior e nunca removia, então a entrada
// sobrevivia à remoção da migration — guardando verdade fora da migration, que é a
// fonte. O caso legítimo que a acumulação dizia proteger (migration que DERRUBOU a
// tabela cita o apelido dela) não precisa dela: o `CreateTable` daquela tabela continua
// no corpus, então o apelido continua derivável.
func TestRefreshCatalogRefleteOCorpusENaoOArquivoAnterior(t *testing.T) {
	raiz, pastaDeMigrations := escreverFixtureDoCorpus(t)
	if err := RefreshCatalog(raiz, pastaDeMigrations); err != nil {
		t.Fatalf("primeira geração: %v", err)
	}

	// Tira uma migration do corpus e regera.
	if err := os.Remove(filepath.Join(pastaDeMigrations, "create_table", "2026_01_01_000002_b.go")); err != nil {
		t.Fatal(err)
	}
	// O cache do leitor é por modTime+tamanho; o arquivo mudou de conteúdo, então uma
	// segunda leitura não pode devolver o mapa antigo.
	if err := RefreshCatalog(raiz, pastaDeMigrations); err != nil {
		t.Fatalf("segunda geração: %v", err)
	}

	dados, err := os.ReadFile(caminhoDoCatalogo(raiz))
	if err != nil {
		t.Fatal(err)
	}
	catalogo := string(dados)
	if strings.Contains(catalogo, "EventosBkp435037Alias2") {
		t.Errorf("a tabela que saiu do corpus não deveria seguir no catálogo\n\n%s", catalogo)
	}
	if !strings.Contains(catalogo, "EventosBkp435037:") {
		t.Errorf("a tabela que continua no corpus deveria seguir no catálogo\n\n%s", catalogo)
	}
}
