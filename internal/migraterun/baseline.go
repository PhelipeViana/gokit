package migraterun

// Baseline: marcar migration como já aplicada SEM executá-la.
//
// Existe por causa do import. Depois de `migrate import`, o corpus descreve um banco
// que já está do jeito descrito — aplicar aquelas migrations recriaria o que existe.
// Hoje isso só "passa" porque `create_table` faz no-op silencioso em objeto
// existente, e o `create_view` — que é rigoroso — falha com "a view já existe".
// Ou seja, o corpus importado não aplica no banco de onde saiu.
//
// A diferença deste baseline em relação ao de outras ferramentas é que ele NÃO
// confia na palavra de quem roda: antes de marcar, ele CONFERE no banco que cada
// objeto declarado existe de verdade. Baseline sem verificação é a mentira mais
// perigosa que este motor poderia gravar — histórico dizendo "aplicada" sobre
// schema que não está lá, e ninguém descobre até a migration seguinte falhar por
// falta de uma coluna que nunca foi criada.
//
// Migration que não passa na verificação continua PENDENTE. Ela não é erro: é
// trabalho para o `migrate run`.

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// resultadoDoBaseline é o veredito de uma migration pendente.
type resultadoDoBaseline struct {
	Arquivo    string
	Objetos    []string
	Faltando   []string
	Verificada bool
	// Indice aponta para a posição em files. Guardar é melhor que refazer o filtro
	// na hora de gravar: refazer é o tipo de duplicação que sai de sincronia.
	Indice int
}

// MigrateBaseline confere as migrations pendentes contra o banco e, com
// confirmação, marca como aplicadas as que descrevem objetos que já existem.
func MigrateBaseline(root string, state config.ConfigState, confirmar bool) error {
	if state.Config == nil {
		return i18n.Errf("scan_no_config")
	}
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if err != nil {
		return err
	}

	connection := state.Config.Connections[state.ActiveClient]
	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	if driver == "" {
		return i18n.Errf("run_dialect_unsupported", connection.Dialect)
	}

	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	historyTable := state.Config.Migrate.Table
	if err := ensureHistory(ctx, db, dialect, connection.Schema, historyTable); err != nil {
		return i18n.Errf("run_create_failed", historyTable, err)
	}
	history, batch, err := loadHistory(ctx, db, dialect, connection.Schema, historyTable)
	if err != nil {
		return err
	}

	// O retrato do banco é lido UMA vez: um corpus legado tem centenas de
	// migrations, e conferir objeto por objeto com consulta própria seria centenas
	// de idas ao catálogo.
	tabelas, err := LerEsquemaDoBanco(ctx, db, dialect, connection.Schema)
	if err != nil {
		return err
	}
	views, err := LerViewsDoBanco(ctx, db, dialect, connection.Schema)
	if err != nil {
		return err
	}

	var resultados []resultadoDoBaseline
	for posicao, file := range files {
		if _, aplicada := history[file.ID]; aplicada {
			continue
		}
		if _, aplicada := history[file.Name]; aplicada {
			continue
		}
		resultado := conferirNoBanco(file, tabelas, views)
		resultado.Indice = posicao
		resultados = append(resultados, resultado)
	}

	if len(resultados) == 0 {
		fmt.Printf("\n  %s %s\n\n", cliui.Success("✓ OK"), i18n.T("bas_sem_pendentes"))
		return nil
	}

	verificadas, pendentes := 0, 0
	fmt.Printf("\n%s\n", i18n.Tf("bas_cabecalho", len(resultados), state.ActiveClient, dialect))
	for _, resultado := range resultados {
		if resultado.Verificada {
			verificadas++
			fmt.Printf("  %s %-52s %s\n", cliui.Success("✓"), resultado.Arquivo,
				cliui.Muted(i18n.Tf("bas_objetos_existem", len(resultado.Objetos))))
			continue
		}
		pendentes++
		fmt.Printf("  %s %-52s %s\n", cliui.Warning("→"), resultado.Arquivo,
			cliui.Muted(i18n.Tf("bas_objetos_faltando", strings.Join(resultado.Faltando, ", "))))
	}

	if verificadas == 0 {
		fmt.Printf("\n%s\n\n", i18n.T("bas_nada_a_marcar"))
		return nil
	}
	if !confirmar {
		fmt.Printf("\n%s\n\n", i18n.Tf("bas_precisa_confirmar", verificadas, pendentes))
		return nil
	}

	batch++
	marcadas := 0
	for _, resultado := range resultados {
		if !resultado.Verificada {
			continue
		}
		file := files[resultado.Indice]
		if err := insertHistory(ctx, db, dialect, connection.Schema, historyTable, file, batch); err != nil {
			return i18n.Errf("run_register_failed", file.Name, err)
		}
		marcadas++
	}

	fmt.Printf("\n  %s %s\n\n", cliui.Success("✓ OK"), i18n.Tf("bas_marcadas", marcadas, pendentes))
	return nil
}

// conferirNoBanco decide se a migration pode ser marcada como aplicada.
//
// A regra é conservadora de propósito: só é verificada a migration cujos objetos o
// gokit sabe conferir e que estão TODOS presentes. Operação que este conferidor não
// sabe avaliar — raw_sql, por exemplo — derruba a verificação, porque marcar sem
// saber é o que não pode acontecer.
func conferirNoBanco(file migrationFile, tabelas map[string]TabelaDoBanco, views map[string]ViewDoBanco) resultadoDoBaseline {
	resultado := resultadoDoBaseline{Arquivo: file.Name, Verificada: true}

	for _, operation := range file.Plan.Operations {
		switch acao.Tipo(operation.Kind) {
		case acao.CreateTable:
			nome := strings.ToLower(operation.Table)
			resultado.Objetos = append(resultado.Objetos, nome)
			tabela, existe := tabelas[nome]
			if !existe {
				resultado.Faltando = append(resultado.Faltando, "tabela "+nome)
				resultado.Verificada = false
				continue
			}
			// Tabela existir não basta: se a migration declara coluna que o banco não
			// tem, ela NÃO foi aplicada e marcar esconderia a diferença.
			for _, coluna := range operation.Columns {
				if !colunaExisteNoRetrato(tabela, coluna.Name) {
					resultado.Faltando = append(resultado.Faltando, nome+"."+strings.ToLower(coluna.Name))
					resultado.Verificada = false
				}
			}

		case acao.AddColumn:
			nome := strings.ToLower(operation.Table)
			tabela, existe := tabelas[nome]
			if !existe {
				resultado.Faltando = append(resultado.Faltando, "tabela "+nome)
				resultado.Verificada = false
				continue
			}
			for _, coluna := range colunasDaOperacao(operation) {
				alvo := nome + "." + strings.ToLower(coluna.Name)
				resultado.Objetos = append(resultado.Objetos, alvo)
				if !colunaExisteNoRetrato(tabela, coluna.Name) {
					resultado.Faltando = append(resultado.Faltando, alvo)
					resultado.Verificada = false
				}
			}


		case acao.CreateIndex, acao.AddForeignKey, acao.AddPrimaryKey, acao.AddUnique, acao.AddCheck:
			// Estes o conferidor sabe que existem no banco pela leitura, mas conferir
			// nome de constraint e de índice entre quatro catálogos é outro trabalho.
			// Enquanto não for feito, a tabela existir é o que se pode afirmar — e a
			// operação não impede o baseline, porque ela só acrescenta a um objeto que
			// já foi conferido acima.

		case acao.Todo:
			// TODO não faz nada no banco, então não há o que conferir.

		default:
			// Operação que este conferidor não sabe avaliar: não marcar é a escolha
			// segura. Aparece no relatório como faltando, com o nome da operação, para
			// quem lê entender por que a migration ficou pendente.
			resultado.Faltando = append(resultado.Faltando, i18n.Tf("bas_operacao_nao_conferivel", operation.Kind))
			resultado.Verificada = false
		}
	}

	sort.Strings(resultado.Faltando)
	return resultado
}

// colunasDaOperacao devolve as colunas de uma operação, cobrindo as duas formas em
// que o plano as guarda: `Expandir` move a coluna para o campo singular.
func colunasDaOperacao(operation acao.Operacao) []acao.ColunaDefinicao {
	if operation.Column != nil {
		return []acao.ColunaDefinicao{*operation.Column}
	}
	return operation.Columns
}

func colunaExisteNoRetrato(tabela TabelaDoBanco, coluna string) bool {
	alvo := strings.ToLower(coluna)
	for _, atual := range tabela.Colunas {
		if strings.ToLower(atual.Nome) == alvo {
			return true
		}
	}
	return false
}
