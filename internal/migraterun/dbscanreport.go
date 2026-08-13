package migraterun

// Relatório do esquema lido do banco, e o diff contra o corpus de migrations.
//
// A leitura vem antes da escrita por um motivo prático: os quatro catálogos
// descrevem a mesma tabela com nomes diferentes (`varchar2` no Oracle,
// `nvarchar` no SQL Server, `character varying` no Postgres), caixa diferente
// (Oracle dobra para maiúsculas) e precisão diferente. Se a leitura divergir, a
// migration gerada a partir dela também diverge — e aí o corpus deixa de ser
// único, que é a premissa do gokit.

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
)

// MigrateScan lê o banco ativo e relata: o que existe lá e não está declarado no
// corpus, o que está declarado e não existe lá, e o que o gokit entendeu de cada
// tabela nova. Não escreve nada.
func MigrateScan(root string, state config.ConfigState, detalhar bool) error {
	tabelas, err := lerEsquemaAtivo(state)
	if err != nil {
		return err
	}

	declaradas, err := tableShapes(root, state)
	if err != nil {
		return err
	}
	// tableShapes indexa em minúsculas pelo nome declarado; o banco pode ter
	// caixa diferente, então a comparação é sempre em minúsculas.
	noCorpus := map[string]bool{}
	for _, forma := range declaradas {
		noCorpus[strings.ToLower(forma.Table)] = true
	}

	var soNoBanco, soNoCorpus []string
	for _, nome := range NomesOrdenados(tabelas) {
		if !noCorpus[nome] {
			soNoBanco = append(soNoBanco, nome)
		}
	}
	for nome := range noCorpus {
		if _, existe := tabelas[nome]; !existe {
			soNoCorpus = append(soNoCorpus, nome)
		}
	}
	sort.Strings(soNoCorpus)

	// A view entra no mesmo diff: o scan dizer "em sincronia" enquanto o import
	// encontra view para escrever seria contradição entre dois comandos que olham o
	// mesmo banco.
	viewsPendentes, err := viewsForaDoCorpus(root, state)
	if err != nil {
		return err
	}

	fmt.Printf("\n%s\n", cliui.Muted(i18n.Tf("scan_header", state.ActiveClient, state.ActiveDialect, len(tabelas))))

	if len(soNoBanco) == 0 && len(soNoCorpus) == 0 && len(viewsPendentes) == 0 {
		fmt.Printf("\n  %s %s\n\n", cliui.Success("✓ OK"), i18n.T("scan_in_sync"))
		return nil
	}

	if len(soNoBanco) > 0 {
		fmt.Printf("\n%s\n", i18n.Tf("scan_only_in_db", len(soNoBanco)))
		for _, nome := range soNoBanco {
			tabela := tabelas[nome]
			fmt.Printf("  %s %-30s %s\n", cliui.Warning("+"), tabela.Nome,
				cliui.Muted(i18n.Tf("scan_table_summary", len(tabela.Colunas), len(tabela.PrimaryKey), len(tabela.ForeignKeys))))
			if detalhar {
				imprimirColunas(tabela)
			}
		}
		if avisadas := tabelasComIdentidadeForaDaChave(pendentesDe(tabelas, soNoBanco)); len(avisadas) > 0 {
			fmt.Printf("\n%s\n", i18n.Tf("imp_identity_warning", len(avisadas)))
			for _, aviso := range avisadas {
				fmt.Printf("  %s %s\n", cliui.Muted("!"), aviso)
			}
		}
		fmt.Printf("\n%s\n", cliui.Muted(i18n.T("scan_hint_import")))
	}

	if len(viewsPendentes) > 0 {
		nomes := make([]string, 0, len(viewsPendentes))
		for nome := range viewsPendentes {
			nomes = append(nomes, nome)
		}
		sort.Strings(nomes)
		fmt.Printf("\n%s\n", i18n.Tf("scan_views_only_in_db", len(nomes)))
		for _, nome := range nomes {
			linhas := strings.Count(viewsPendentes[nome].SQL, "\n") + 1
			fmt.Printf("  %s %-30s %s\n", cliui.Warning("+"), nome,
				cliui.Muted(i18n.Tf("scan_view_summary", linhas)))
			if detalhar {
				fmt.Printf("      %s\n", cliui.Muted(primeiraLinha(viewsPendentes[nome].SQL)))
			}
		}
		fmt.Printf("\n%s\n", cliui.Muted(i18n.T("scan_hint_view")))
	}

	if len(soNoCorpus) > 0 {
		fmt.Printf("\n%s\n", i18n.Tf("scan_only_in_corpus", len(soNoCorpus)))
		for _, nome := range soNoCorpus {
			fmt.Printf("  %s %s\n", cliui.Muted("-"), nome)
		}
		fmt.Printf("\n%s\n", cliui.Muted(i18n.T("scan_hint_pending")))
	}

	fmt.Println()
	return nil
}

func imprimirColunas(tabela TabelaDoBanco) {
	chave := map[string]bool{}
	for _, coluna := range tabela.PrimaryKey {
		chave[strings.ToLower(coluna)] = true
	}
	pai := map[string]FKDoBanco{}
	for _, fk := range tabela.ForeignKeys {
		pai[strings.ToLower(fk.Coluna)] = fk
	}

	for _, coluna := range tabela.Colunas {
		marcas := []string{}
		if chave[strings.ToLower(coluna.Nome)] {
			marcas = append(marcas, "PK")
		}
		if coluna.Identity {
			marcas = append(marcas, "identity")
		}
		if !coluna.Nulo {
			marcas = append(marcas, "not null")
		}
		if coluna.Default != "" {
			marcas = append(marcas, "default="+coluna.Default)
		}
		if fk := pai[strings.ToLower(coluna.Nome)]; fk.TabelaPai != "" {
			marcas = append(marcas, "→ "+strings.ToLower(fk.TabelaPai)+"."+strings.ToLower(fk.ColunaPai))
		}
		fmt.Printf("      %-28s %-18s %s\n", strings.ToLower(coluna.Nome),
			descreveTipo(coluna), cliui.Muted(strings.Join(marcas, ", ")))
	}

	// Índice sai depois das colunas, porque pode cobrir várias delas.
	for _, indice := range tabela.Indices {
		tipo := "índice"
		if indice.Unico {
			tipo = "índice único"
		}
		fmt.Printf("      %s\n", cliui.Muted(fmt.Sprintf("%s %s (%s)", tipo, indice.Nome, strings.Join(indice.Colunas, ", "))))
	}
}

// descreveTipo mostra o tipo como o banco o guarda, com o tamanho que importa.
// É deliberadamente o tipo CRU: é o que permite comparar os quatro e ver onde
// eles divergem antes de traduzir para o DSL.
func descreveTipo(coluna ColunaDoBanco) string {
	switch {
	case coluna.Escala > 0:
		return fmt.Sprintf("%s(%d,%d)", coluna.Tipo, coluna.Precisao, coluna.Escala)
	case coluna.Tamanho > 0:
		return fmt.Sprintf("%s(%d)", coluna.Tipo, coluna.Tamanho)
	case coluna.Precisao > 0:
		return fmt.Sprintf("%s(%d)", coluna.Tipo, coluna.Precisao)
	}
	return coluna.Tipo
}

func lerEsquemaAtivo(state config.ConfigState) (map[string]TabelaDoBanco, error) {
	if state.Config == nil {
		return nil, i18n.Errf("scan_no_config")
	}
	connection := state.Config.Connections[state.ActiveClient]
	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	if driver == "" {
		return nil, i18n.Errf("run_dialect_unsupported", connection.Dialect)
	}

	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return LerEsquemaDoBanco(ctx, db, dialect, connection.Schema)
}

// pendentesDe recorta as tabelas nomeadas, mantendo a ordem dos nomes.
func pendentesDe(tabelas map[string]TabelaDoBanco, nomes []string) []TabelaDoBanco {
	recorte := make([]TabelaDoBanco, 0, len(nomes))
	for _, nome := range nomes {
		if tabela, existe := tabelas[nome]; existe {
			recorte = append(recorte, tabela)
		}
	}
	return recorte
}

// primeiraLinha resume a definição da view numa linha, para o relatório caber na
// tela sem esconder que existe SQL ali.
func primeiraLinha(sql string) string {
	linha := strings.TrimSpace(strings.ReplaceAll(strings.SplitN(strings.TrimSpace(sql), "\n", 2)[0], "\r", ""))
	if len(linha) > 100 {
		return linha[:100] + "…"
	}
	return linha
}
