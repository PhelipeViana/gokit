package migrationgo

// Regressão do catálogo GERADO: o arquivo tem de compilar.
//
// Os dois defeitos que estes testes travam passaram por `migrate validate` sem um
// arranhão, porque validate lê por AST e nunca compila. Só apareceram ao rodar
// `go build` no projeto normalizado, com 754 tabelas e 552 views vindas de um schema
// legado de verdade.
//
// Por isso os testes fazem TYPE-CHECK, e não comparação de texto: asserção de string
// aceitaria `X migrate.RegisteredView` de novo, que é sintaticamente perfeito e não
// compila. O que quebrava era o tipo, então é o tipo que precisa ser conferido.

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// conferirTiposDoCatalogo type-checka o arquivo gerado contra um stub do pacote
// migration.
//
// O stub existe porque o pacote de verdade não pode ser importado por um teste que
// roda dentro do próprio módulo sem depender do disco. Ele declara exatamente o que o
// catálogo usa, com a MESMA forma do original: Table e ColumnName são tipos nomeados
// sobre string (aceitam conversão), View tem campo privado (não aceita) e
// RegisteredView é função. É essa diferença que o gerador precisa respeitar.
func conferirTiposDoCatalogo(t *testing.T, fonte string) {
	t.Helper()

	const stub = `package migration

type Table string
type ColumnName string
type View struct{ name string }

func RegisteredView(name string) View { return View{name: name} }
`

	conjunto := token.NewFileSet()
	arquivoStub, err := parser.ParseFile(conjunto, "migration.go", stub, 0)
	if err != nil {
		t.Fatalf("stub do pacote migration não parseia: %v", err)
	}
	configuracao := types.Config{Importer: importer.Default()}
	pacoteMigration, err := configuracao.Check("github.com/PhelipeViana/gokit/migration",
		conjunto, []*ast.File{arquivoStub}, nil)
	if err != nil {
		t.Fatalf("stub do pacote migration não type-checka: %v", err)
	}

	arquivoGerado, err := parser.ParseFile(conjunto, "catalogo.gen.go", fonte, 0)
	if err != nil {
		t.Fatalf("o catálogo gerado não parseia: %v\n%s", err, fonte)
	}
	comStub := types.Config{
		Importer: importadorFixo{caminho: "github.com/PhelipeViana/gokit/migration", pacote: pacoteMigration},
	}
	if _, err := comStub.Check("core", conjunto, []*ast.File{arquivoGerado}, nil); err != nil {
		t.Fatalf("o catálogo gerado NÃO compila: %v\n%s", err, fonte)
	}
}

type importadorFixo struct {
	caminho string
	pacote  *types.Package
}

func (i importadorFixo) Import(caminho string) (*types.Package, error) {
	if caminho == i.caminho {
		return i.pacote, nil
	}
	return importer.Default().Import(caminho)
}

// O catálogo de views precisa declarar o campo com o TIPO (migrate.View) e construir
// com a FUNÇÃO (migrate.RegisteredView). Usando o mesmo nome nos dois lugares, o
// arquivo saía com uma função no lugar do tipo.
//
// Nunca apareceu porque o projeto de teste tinha zero views — e com zero entradas o
// agrupador degenera para `struct{}{}`, onde tipo nenhum é escrito.
func TestCatalogoDeViewsComEntradasCompila(t *testing.T) {
	raiz := t.TempDir()
	views := map[string]bool{
		"vwsegurado_orgao_branco": true,
		"wkf_status":              true,
		"algarismo_romano":        true,
	}
	if err := WriteCoreViewCatalog(raiz, views); err != nil {
		t.Fatalf("escrever o catálogo de views: %v", err)
	}

	dados, err := os.ReadFile(caminhoDoCatalogoDeViews(raiz))
	if err != nil {
		t.Fatalf("ler o catálogo de views: %v", err)
	}
	fonte := string(dados)
	conferirTiposDoCatalogo(t, fonte)

	// Além do type-check, a forma esperada: tipo no campo, função no valor.
	if !strings.Contains(fonte, "migrate.View\n") {
		t.Error("o campo deveria ser declarado com o tipo migrate.View")
	}
	if !strings.Contains(fonte, "migrate.RegisteredView(") {
		t.Error("o valor deveria ser construído com migrate.RegisteredView")
	}
}

// Com zero views o agrupador vira struct{}{} — que também tem de compilar, e é o caso
// que mascarou o defeito acima.
func TestCatalogoDeViewsVazioCompila(t *testing.T) {
	raiz := t.TempDir()
	if err := WriteCoreViewCatalog(raiz, map[string]bool{}); err != nil {
		t.Fatalf("escrever o catálogo de views vazio: %v", err)
	}
	dados, err := os.ReadFile(caminhoDoCatalogoDeViews(raiz))
	if err != nil {
		t.Fatalf("ler o catálogo de views: %v", err)
	}
	conferirTiposDoCatalogo(t, string(dados))
}

// Dois nomes físicos que colapsam no mesmo identificador Go: o catálogo de TABELAS
// desempata com apelido, e o de COLUNAS tem de respeitar esse desempate em vez de
// derivar o identificador de novo.
//
// Medido em schema real: eventos_bkp435037 e eventos_bkp_435037 geram os dois
// EventosBkp435037. O catálogo de tabelas escrevia ...Alias2 para o segundo, e o de
// colunas escrevia o mesmo nome duas vezes — struct redeclarada.
func TestCatalogoDeColunasRespeitaOApelidoDaTabela(t *testing.T) {
	raiz := t.TempDir()
	pastaDeMigrations := filepath.Join(raiz, "migrate")
	if err := os.MkdirAll(pastaDeMigrations, 0o755); err != nil {
		t.Fatal(err)
	}

	// O catálogo de tabelas, como o import o escreve: o segundo recebeu apelido, e o
	// nome físico só sobrevive no comentário.
	catalogoDeTabelas := `// Code generated by GoKit. DO NOT EDIT.
package core

import migrate "github.com/PhelipeViana/gokit/migration"

var Table = struct {
	EventosBkp435037       migrate.Table
	EventosBkp435037Alias2 migrate.Table
}{
	EventosBkp435037:       migrate.Table("eventos_bkp435037"),
	EventosBkp435037Alias2: migrate.Table("eventos_bkp_435037_alias2"), // physical: eventos_bkp_435037
}
`
	if err := os.MkdirAll(filepath.Dir(caminhoDoCatalogo(raiz)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caminhoDoCatalogo(raiz), []byte(catalogoDeTabelas), 0o644); err != nil {
		t.Fatal(err)
	}

	// As migrations do import citam o nome FÍSICO literal, não a referência de
	// catálogo — é isso que faz as chaves chegarem sem o desempate aplicado.
	migrations := map[string]string{
		"20260814_a.go": `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration() migrate.Plan {
	return migrate.Declare(
		migrate.CreateTable("eventos_bkp435037", migrate.Col("id").Int(), migrate.Col("valor").Int()),
	)
}
`,
		"20260814_b.go": `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration() migrate.Plan {
	return migrate.Declare(
		migrate.CreateTable("eventos_bkp_435037", migrate.Col("id").Int(), migrate.Col("outro").Int()),
	)
}
`,
	}
	for nome, corpo := range migrations {
		if err := os.WriteFile(filepath.Join(pastaDeMigrations, nome), []byte(corpo), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := WriteCoreColumnCatalog(raiz, pastaDeMigrations); err != nil {
		t.Fatalf("escrever o catálogo de colunas: %v", err)
	}
	dados, err := os.ReadFile(caminhoDoCatalogoDeColunas(raiz))
	if err != nil {
		t.Fatalf("ler o catálogo de colunas: %v", err)
	}
	fonte := string(dados)

	// O type-check é a asserção que importa: struct redeclarada não compila.
	conferirTiposDoCatalogo(t, fonte)

	// E os dois grupos têm de existir, cada um com a coluna que é dele.
	for _, esperado := range []string{"EventosBkp435037 struct", "EventosBkp435037Alias2 struct"} {
		if !strings.Contains(fonte, esperado) {
			t.Errorf("faltou o grupo %q no catálogo de colunas", esperado)
		}
	}
	if !strings.Contains(fonte, `Valor: "valor"`) || !strings.Contains(fonte, `Outro: "outro"`) {
		t.Error("cada tabela deveria manter as próprias colunas")
	}
}
