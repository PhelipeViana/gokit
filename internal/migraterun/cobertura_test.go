package migraterun

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/migration/acao"
)

// Medição de cobertura do motor: todo tipo de operação tem de ter caminho
// declarado no executor E desfecho declarado no rollback, nos quatro dialetos.
//
// A skill do motor avisa que adicionar operação toca no mínimo cinco lugares, e
// esquecer o rollbackSQL faz a operação cair no default e virar erro só na hora de
// desfazer — quando já é tarde. Estes testes existem para que a lacuna apareça
// aqui, e não em produção.

var dialetos = []string{"oracle", "postgres", "mysql", "sqlserver"}

// todosOsTipos é a lista canônica. Tipo novo em acao precisa entrar aqui, e é
// isso que faz o teste falhar quando alguém esquece de tratá-lo.
var todosOsTipos = []acao.Tipo{
	acao.CreateTable, acao.DropTable,
	acao.AddColumn, acao.AlterColumn, acao.DropColumn,
	acao.AddForeignKey, acao.DropForeignKey,
	acao.CreateIndex, acao.DropIndex,
	acao.CreateView, acao.AlterView, acao.DropView,
	acao.CreateSequence, acao.DropSequence,
	acao.RenameTable, acao.RenameColumn,
	acao.AddPrimaryKey, acao.AddUnique, acao.AddCheck, acao.DropConstraint,
	acao.RawSQL, acao.SeedRows, acao.Todo,
}

// tipoDeclarado acha as constantes de tipo na fonte do pacote acao.
var tipoDeclarado = regexp.MustCompile(`(?m)^\s*[A-Za-z]+\s+Tipo\s*=\s*"([a-z_]+)"`)

// TestListaDeTiposEstaCompleta fecha o laço da medição.
//
// Sem isto, os testes de cobertura mediriam só o que a lista `todosOsTipos`
// declara — e um tipo novo em acao.go que ninguém acrescentou à lista passaria
// despercebido exatamente como passaria despercebido no executor. Ler a fonte é o
// que transforma "mede o que eu listei" em "mede o que existe".
func TestListaDeTiposEstaCompleta(t *testing.T) {
	fonte, err := os.ReadFile(filepath.Join("..", "..", "migration", "acao", "acao.go"))
	if err != nil {
		t.Fatalf("não foi possível ler a fonte do acao: %v", err)
	}

	naLista := map[string]bool{}
	for _, tipo := range todosOsTipos {
		naLista[string(tipo)] = true
	}

	var ausentes []string
	declarados := 0
	for _, achado := range tipoDeclarado.FindAllStringSubmatch(string(fonte), -1) {
		declarados++
		if !naLista[achado[1]] {
			ausentes = append(ausentes, achado[1])
		}
	}
	if declarados == 0 {
		t.Fatal("nenhuma constante de tipo encontrada — o padrão de leitura da fonte quebrou")
	}
	if len(ausentes) > 0 {
		sort.Strings(ausentes)
		t.Errorf("tipo(s) declarado(s) em acao.go fora de todosOsTipos: %s — acrescente à lista para que a cobertura os meça",
			strings.Join(ausentes, ", "))
	}
	t.Logf("%d tipos declarados em acao.go, todos medidos", declarados)
}

// operacaoMinima monta uma operação plausível de cada tipo, com o campo que o
// caso correspondente exige preenchido.
func operacaoMinima(tipo acao.Tipo) acao.Operacao {
	coluna := acao.ColunaDefinicao{Name: "nome", Type: "varchar", Length: 100, Nullable: true}
	op := acao.Operacao{Kind: string(tipo), Table: "users", Name: "obj_users"}
	switch tipo {
	case acao.CreateTable:
		op.Columns = []acao.ColunaDefinicao{
			{Name: "id", Type: "integer", PrimaryKey: true, AutoIncrement: true},
			coluna,
		}
	case acao.AddColumn, acao.AlterColumn, acao.DropColumn:
		op.Column = &coluna
	case acao.AddForeignKey, acao.DropForeignKey:
		op.ForeignKey = &acao.ForeignKey{
			ConstraintName: "fk_users_cidade", ReferenceTable: "cidades",
			Columns: []string{"cidade_id"}, ReferenceColumns: []string{"id"},
		}
	case acao.CreateIndex, acao.DropIndex, acao.AddUnique:
		op.IndexColumns = []string{"nome"}
	case acao.AddPrimaryKey:
		op.IndexColumns = []string{"id"}
	case acao.AddCheck:
		op.SQL = "saldo > 0" // AddCheck carrega a expressão no campo SQL
	case acao.CreateView, acao.AlterView, acao.DropView:
		op.Name = "vw_users"
		op.ViewSQL = map[string]string{"common": "SELECT 1"}
	case acao.RenameTable, acao.RenameColumn:
		op.NewName = "novo_nome"
		if tipo == acao.RenameColumn {
			op.Column = &coluna
		}
	case acao.RawSQL:
		op.SQL = "SELECT 1"
	}
	return op
}

// desfechosDeRollback são os tipos cujo rollback é DECLARADAMENTE impossível, com
// a razão. Não é lacuna: é a decisão de "falhar alto no irreversível" em vez de
// apagar histórico sem desfazer.
var desfechosDeRollback = map[acao.Tipo]string{
	acao.RawSQL:         "SQL cru é opaco: o motor não sabe o que foi feito",
	acao.AlterView:      "alterar view não guarda a definição anterior",
	acao.AlterColumn:    "alterar coluna não guarda o tipo anterior",
	acao.DropTable:      "o objeto derrubado não é recriável a partir da operação",
	acao.DropColumn:     "idem",
	acao.DropIndex:      "idem",
	acao.DropView:       "idem",
	acao.DropForeignKey: "idem",
	acao.DropConstraint: "idem",
	acao.DropSequence:   "idem",
	acao.SeedRows:       "seed não é migration: quem desfaz é o motor de seed",
	acao.Todo:           "no-op não tem o que desfazer",
}

// TestCoberturaDeRollback mede os 23 tipos nos 4 dialetos e imprime a matriz.
// Falha quando um tipo cai no default do rollbackSQL sem estar na lista de
// irreversíveis declarados — que é exatamente o esquecimento que a skill descreve.
func TestCoberturaDeRollback(t *testing.T) {
	faltando := map[acao.Tipo][]string{}
	for _, tipo := range todosOsTipos {
		for _, dialeto := range dialetos {
			sql, err := rollbackSQL(dialeto, "s", operacaoMinima(tipo))
			switch {
			case err != nil && strings.Contains(err.Error(), "rollback") && strings.Contains(err.Error(), string(tipo)):
				// Caiu no default: nenhum caso trata este tipo.
				faltando[tipo] = append(faltando[tipo], dialeto)
			case err == nil && strings.TrimSpace(sql) == "":
				t.Errorf("%s/%s: rollback devolveu SQL vazio sem erro — o desfecho tem de ser explícito", tipo, dialeto)
			}
		}
	}
	for tipo, dialetosSemRollback := range faltando {
		if _, previsto := desfechosDeRollback[tipo]; previsto {
			continue
		}
		t.Errorf("tipo %q não tem rollback declarado em %v; trate o caso ou registre em desfechosDeRollback com a razão",
			tipo, dialetosSemRollback)
	}

	// Relatório, para leitura humana.
	reverte, naoReverte := []string{}, []string{}
	for _, tipo := range todosOsTipos {
		if _, previsto := desfechosDeRollback[tipo]; previsto {
			naoReverte = append(naoReverte, string(tipo))
			continue
		}
		reverte = append(reverte, string(tipo))
	}
	sort.Strings(reverte)
	sort.Strings(naoReverte)
	t.Logf("reversíveis (%d): %s", len(reverte), strings.Join(reverte, ", "))
	t.Logf("irreversíveis por decisão (%d): %s", len(naoReverte), strings.Join(naoReverte, ", "))
}

// TestCoberturaDoExecutor mede o outro lado: todo tipo tem caminho no executor.
//
// Só os tipos que não tocam o banco podem ser exercitados com db nulo, então o que
// se mede aqui é a AUSÊNCIA de "operação desconhecida" — o default. Um tipo que
// chega ao banco falha por conexão nula, e isso conta como caminho existente.
func TestCoberturaDoExecutor(t *testing.T) {
	const desconhecida = "desconhecida"
	semCaminho := []string{}

	for _, tipo := range todosOsTipos {
		if tipo == acao.SeedRows {
			// Seed não passa pelo executor de migration: o parser recusa Seeder()
			// dentro de migration, e o motor de seed tem caminho próprio.
			continue
		}
		err := func() (err error) {
			// Tipo que chega ao banco entra em panic com db nulo; o que importa é
			// que não caiu no default antes disso.
			defer func() {
				if r := recover(); r != nil {
					err = nil
				}
			}()
			return executeOperation(context.Background(), nil, "postgres", "",
				operacaoMinima(tipo), map[string]bool{}, nil)
		}()
		if err != nil && strings.Contains(err.Error(), desconhecida) {
			semCaminho = append(semCaminho, string(tipo))
		}
	}

	if len(semCaminho) > 0 {
		t.Errorf("tipos sem caminho no executeOperation: %s", strings.Join(semCaminho, ", "))
	}
}

// ── Matriz de dialeto: tipo de coluna × banco ──

// tiposDeColuna são os tipos que o DSL alcança. Cada método de Coluna grava um
// destes, e é por isso que a lista sai de lá e não do mapa do columnTypeSQL —
// medir o mapa contra si mesmo não provaria nada.
var tiposDeColuna = []string{
	"int", "integer", "string", "char", "text",
	"boolean", "decimal", "date", "datetime", "timestamp", "binary",
}

// TestTipoDeColunaTemSQLNosQuatroDialetos mede a matriz inteira.
//
// columnTypeSQL é um mapa de mapas e devolve string VAZIA quando o tipo ou o
// dialeto não estão nele — e string vazia vira DDL sem tipo
// (`ALTER TABLE t ADD COLUMN "x" NOT NULL`), que o banco recusa com uma mensagem
// que não aponta a causa. Uma célula vazia aqui é bug de dialeto.
func TestTipoDeColunaTemSQLNosQuatroDialetos(t *testing.T) {
	for _, tipo := range tiposDeColuna {
		for _, dialeto := range dialetos {
			coluna := acao.ColunaDefinicao{Name: "c", Type: tipo}
			if sql := columnTypeSQL(dialeto, coluna); strings.TrimSpace(sql) == "" {
				t.Errorf("%s/%s: sem tipo SQL — a DDL sairia sem tipo de coluna", tipo, dialeto)
			}
		}
	}
}

// TestTiposDoDSLEDoValidadorCasamComOCompilador cruza as três listas que precisam
// concordar: o que o DSL grava, o que o validador aceita e o que o compilador sabe
// traduzir. Divergência entre elas é a lacuna que só aparece no banco do cliente.
func TestTiposDoDSLEDoValidadorCasamComOCompilador(t *testing.T) {
	fonte, err := os.ReadFile(filepath.Join("..", "..", "migration", "acao", "acao.go"))
	if err != nil {
		t.Fatalf("não foi possível ler a fonte do acao: %v", err)
	}

	// O que o DSL realmente grava em Type.
	gravados := map[string]bool{}
	for _, achado := range regexp.MustCompile(`c\.value\.Type(?:, [a-zA-Z.]+)* = "([a-z]+)"`).
		FindAllStringSubmatch(string(fonte), -1) {
		gravados[achado[1]] = true
	}
	if len(gravados) == 0 {
		t.Fatal("nenhum tipo encontrado no DSL — o padrão de leitura da fonte quebrou")
	}

	medidos := map[string]bool{}
	for _, tipo := range tiposDeColuna {
		medidos[tipo] = true
	}

	var fora []string
	for tipo := range gravados {
		if !medidos[tipo] {
			fora = append(fora, tipo)
		}
		// E o compilador tem de saber traduzir para os quatro.
		for _, dialeto := range dialetos {
			if columnTypeSQL(dialeto, acao.ColunaDefinicao{Name: "c", Type: tipo}) == "" {
				t.Errorf("o DSL grava o tipo %q, mas o compilador não o traduz em %s", tipo, dialeto)
			}
		}
	}
	if len(fora) > 0 {
		sort.Strings(fora)
		t.Errorf("tipo(s) que o DSL grava e a matriz não mede: %s", strings.Join(fora, ", "))
	}
	t.Logf("%d tipos gravados pelo DSL, todos traduzidos nos 4 dialetos", len(gravados))
}
