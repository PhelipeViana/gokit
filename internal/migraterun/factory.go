package migraterun

// Executor das factories.
//
// Diferença essencial em relação ao seed: seed é dado de produção e a regra de
// dono impede sobrescrever o que a aplicação criou. Factory é dado descartável
// de teste — a tabela é limpa e repovoada. Por isso os dois caminhos são
// separados, mesmo parecendo próximos.

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
	"github.com/PhelipeViana/gokit/internal/factorygo"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
	migrate "github.com/PhelipeViana/gokit/migration"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// plano é uma factory pronta para executar, já casada com a tabela declarada
// nas migrations.
type plano struct {
	Arquivo  factorygo.Arquivo
	Forma    acao.Operacao // o CreateTable da tabela
	Conhece  bool          // a tabela existe no corpus de migrations
	Pais     []string      // tabelas que precisam ser populadas antes
	Identity string        // coluna auto-incremento, se houver
}

// Tabela devolve o nome em caixa alta, usado como chave interna para casar
// factory com migration sem depender de como cada banco normaliza nomes.
func (p plano) Tabela() string { return strings.ToUpper(p.Arquivo.Tabela) }

// Fisica devolve o nome como a migration o declarou, que é o que existe no
// banco. A factory pode escrever CIDADES em caixa alta, mas o Postgres criou
// cidades em minúsculas — só o nome da migration serve para consultar.
func (p plano) Fisica() string {
	if p.Forma.Table != "" {
		return p.Forma.Table
	}
	return p.Arquivo.Tabela
}

// colunaFisica traduz o nome escrito na factory para o nome declarado na
// migration. Mesma razão que Fisica: caixa alta só funciona no Oracle.
func (p plano) colunaFisica(nome string) string {
	return colunaFisicaEm(p.Forma.Columns, nome)
}

// colunaFisicaEm traduz o que a factory escreveu para o nome que a migration
// declarou, aceitando as DUAS formas de endereçar coluna:
//
//	"estado_id"                  texto — casa por caixa
//	core.Column.Cidades.EstadoId referência — casa pelo identificador Go
//
// A segunda é necessária porque EqualFold("estado_id", "EstadoId") é falso: o
// underscore desaparece na normalização. Resolver aqui, e não no parser da
// factory, evita dar catálogo ao parser — a forma da tabela já está nesta camada.
func colunaFisicaEm(colunas []acao.ColunaDefinicao, nome string) string {
	for _, column := range colunas {
		if strings.EqualFold(column.Name, nome) {
			return column.Name
		}
	}
	for _, column := range colunas {
		if migrationgo.ExportedIdentifier(column.Name) == nome {
			return column.Name
		}
	}
	return nome
}

// colunaReferenciadaPor devolve a coluna do PAI que a FK da tabela aponta. É o
// que permite omitir a coluna no Reference: a migration já declarou o destino.
func colunaReferenciadaPor(forma acao.Operacao, pai string) string {
	for _, column := range forma.Columns {
		if strings.EqualFold(column.ReferenceTable, pai) && column.ReferenceColumn != "" {
			return column.ReferenceColumn
		}
	}
	return ""
}

func factoryRoot(root string, state config.ConfigState) string {
	pasta := state.Config.Output.Factory
	if pasta == "" {
		pasta = "database/factories"
	}
	return filepath.Join(root, filepath.FromSlash(pasta))
}

// tableForeignKeys monta o grafo filho -> pais a partir das migrations. A
// origem é o corpus em AST, não o catálogo do banco: assim a ordenação é a
// mesma nos quatro dialetos e funciona antes mesmo do banco existir.
func tableForeignKeys(root string, state config.ConfigState) (map[string][]string, error) {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if err != nil {
		return nil, describeLoadError(err)
	}

	grafo := map[string]map[string]bool{}
	anota := func(filho, pai string) {
		filho, pai = strings.ToUpper(filho), strings.ToUpper(pai)
		// Autorreferência não é dependência de ordem: a linha aponta para
		// outra da mesma tabela, que já estará lá.
		if filho == "" || pai == "" || filho == pai {
			return
		}
		if grafo[filho] == nil {
			grafo[filho] = map[string]bool{}
		}
		grafo[filho][pai] = true
	}

	for _, file := range files {
		for _, operation := range file.Plan.Operations {
			switch operation.Kind {
			case string(acao.CreateTable), string(acao.AddColumn), string(acao.AddForeignKey):
				for _, column := range operation.Columns {
					anota(operation.Table, column.ReferenceTable)
				}
				// Expandir move a coluna para o campo singular fora do CreateTable.
				if operation.Column != nil {
					anota(operation.Table, operation.Column.ReferenceTable)
				}
				if operation.ForeignKey != nil {
					anota(operation.Table, operation.ForeignKey.ReferenceTable)
				}
			}
		}
	}

	resultado := map[string][]string{}
	for filho, pais := range grafo {
		lista := make([]string, 0, len(pais))
		for pai := range pais {
			lista = append(lista, pai)
		}
		sort.Strings(lista)
		resultado[filho] = lista
	}
	return resultado, nil
}

// loadFactories lê as factories e as casa com o schema das migrations.
func loadFactories(root string, state config.ConfigState) ([]plano, error) {
	arquivos, err := factorygo.CarregarPasta(factoryRoot(root, state))
	if err != nil {
		return nil, err
	}
	if len(arquivos) == 0 {
		return nil, nil
	}

	formas, err := tableShapes(root, state)
	if err != nil {
		return nil, err
	}
	fks, err := tableForeignKeys(root, state)
	if err != nil {
		return nil, err
	}

	planos := make([]plano, 0, len(arquivos))
	for _, arquivo := range arquivos {
		forma, conhece := formas[strings.ToLower(arquivo.Tabela)]

		atual := plano{Arquivo: arquivo, Forma: forma, Conhece: conhece}
		for _, column := range forma.Columns {
			if column.AutoIncrement {
				atual.Identity = column.Name
				break
			}
		}

		// A referência é escrita em caixa alta na factory, mas quem vai à consulta
		// é o nome que a migration declarou — no Postgres e no MySQL a caixa
		// importa. A tradução acontece aqui, uma vez, e não a cada linha.
		for posicao := range atual.Arquivo.Campos {
			link := atual.Arquivo.Campos[posicao].Referencia
			if link == nil {
				continue
			}
			// Coluna omitida: sai da FK declarada na migration, que é quem sabe para
			// onde a referência aponta. Escrever a coluna à mão passa a ser só o caso
			// de schema legado, sem FK declarada.
			if link.Column == "" {
				link.Column = colunaReferenciadaPor(forma, link.Table)
			}
			paiForma, existe := formas[strings.ToLower(link.Table)]
			if !existe {
				continue
			}
			link.Table = paiForma.Table
			link.Column = colunaFisicaEm(paiForma.Columns, link.Column)
			if link.Column == "" {
				// Sem FK declarada e sem coluna escrita, a chave primária do pai é o
				// único alvo razoável.
				for _, column := range paiForma.Columns {
					if column.PrimaryKey {
						link.Column = column.Name
						break
					}
				}
			}
		}

		// Um pai pode vir da FK declarada na migration ou de uma Reference
		// escrito na factory. Os dois valem.
		vistos := map[string]bool{}
		for _, pai := range append(fks[atual.Tabela()], arquivo.Referencias()...) {
			pai = strings.ToUpper(pai)
			if pai != atual.Tabela() && !vistos[pai] {
				vistos[pai] = true
				atual.Pais = append(atual.Pais, pai)
			}
		}
		sort.Strings(atual.Pais)
		planos = append(planos, atual)
	}
	return planos, nil
}

// ordenaFactories põe as tabelas pai antes das filhas.
//
// Ciclos existem de verdade em schema legado (A referencia B e B referencia A
// com coluna anulável). Em vez de falhar, o ciclo é rompido no ponto de menor
// dependência e a ordem restante é preservada — o INSERT ainda funciona porque
// a coluna do ciclo aceita nulo.
func ordenaFactories(planos []plano) ([]plano, []string) {
	porTabela := map[string]plano{}
	pendentes := map[string]bool{}
	for _, atual := range planos {
		porTabela[atual.Tabela()] = atual
		pendentes[atual.Tabela()] = true
	}

	var ordenados []plano
	var ciclos []string

	for len(pendentes) > 0 {
		var prontos []string
		for tabela := range pendentes {
			livre := true
			for _, pai := range porTabela[tabela].Pais {
				// Autorreferência não é dependência de ordem: a linha aponta
				// para outra da mesma tabela, que já estará lá.
				if pai != tabela && pendentes[pai] {
					livre = false
					break
				}
			}
			if livre {
				prontos = append(prontos, tabela)
			}
		}

		if len(prontos) == 0 {
			// Ciclo: escolhe a tabela com menos dependências pendentes.
			melhor, menor := "", 1<<30
			for tabela := range pendentes {
				contagem := 0
				for _, pai := range porTabela[tabela].Pais {
					if pai != tabela && pendentes[pai] {
						contagem++
					}
				}
				if contagem < menor || (contagem == menor && tabela < melhor) {
					melhor, menor = tabela, contagem
				}
			}
			ciclos = append(ciclos, melhor)
			prontos = []string{melhor}
		}

		sort.Strings(prontos)
		for _, tabela := range prontos {
			ordenados = append(ordenados, porTabela[tabela])
			delete(pendentes, tabela)
		}
	}
	return ordenados, ciclos
}

// FactoryTables lista as tabelas que têm factory, para a seleção na TUI.
func FactoryTables(root string, state config.ConfigState) ([]string, error) {
	planos, err := loadFactories(root, state)
	if err != nil {
		return nil, err
	}
	tabelas := make([]string, 0, len(planos))
	for _, atual := range planos {
		tabelas = append(tabelas, atual.Arquivo.Tabela)
	}
	sort.Strings(tabelas)
	return tabelas, nil
}

// FactoryValidate confere as factories sem tocar no banco.
func FactoryValidate(root string, state config.ConfigState) error {
	planos, err := loadFactories(root, state)
	if err != nil {
		return err
	}
	if len(planos) == 0 {
		fmt.Println(cliui.Muted(i18n.T("fac_none_found_in") + factoryRoot(root, state)))
		return nil
	}

	// Os domínios declarados em AddCheck: é com eles que a conferência de valor
	// sabe que "ABERTO" é aceito e "X" não. Falha aqui não impede o resto da
	// validação — sem os checks, só essa regra deixa de ser conferida.
	checks, err := tableCheckValues(root, state)
	if err != nil {
		checks = map[string]map[string][]string{}
	}

	var problemas []string
	ativas, linhas := 0, 0

	for _, atual := range planos {
		if !atual.Conhece {
			problemas = append(problemas, i18n.Tf("fac_table_not_created", filepath.Base(atual.Arquivo.Caminho), atual.Arquivo.Tabela))
			continue
		}

		// Coluna que não existe na tabela derruba o INSERT inteiro; melhor
		// avisar aqui.
		// As duas formas de endereçar coluna contam como declarada: o texto
		// ("cidade_id", casado por caixa) e a referência de catálogo
		// (core.Column.Users.CidadeId, casada pelo identificador Go). Sem a segunda,
		// o validador acusaria como inexistente justamente a coluna de nome
		// composto, que é onde o underscore desaparece na normalização.
		declaradas := map[string]bool{}
		for _, column := range atual.Forma.Columns {
			declaradas[strings.ToUpper(column.Name)] = true
			declaradas[strings.ToUpper(migrationgo.ExportedIdentifier(column.Name))] = true
		}
		for _, coluna := range atual.Arquivo.Colunas() {
			if !declaradas[strings.ToUpper(coluna)] {
				problemas = append(problemas, i18n.Tf("fac_column_missing", filepath.Base(atual.Arquivo.Caminho), coluna, atual.Arquivo.Tabela))
			}
		}

		// Gera as linhas em memória e confere o que o banco recusaria pelo VALOR.
		// Sem isso, tamanho estourado, tipo incompatível e chave repetida só
		// aparecem no meio do INSERT, com a mensagem do driver.
		for _, problema := range conferirValoresDaFactory(atual, checks[atual.Tabela()]) {
			problemas = append(problemas, problema.String())
		}

		if atual.Arquivo.Ruler.Active {
			ativas++
			linhas += quantidadeDeLinhas(atual)
		}
	}

	if len(problemas) > 0 {
		return cliui.NewUserError(
			i18n.Tf("fac_problems", len(problemas), strings.Join(problemas, "\n  - ")),
			i18n.T("fac_problems_fix"),
		)
	}

	ordenados, ciclos := ordenaFactories(planos)
	fmt.Printf(i18n.T("fac_summary"),
		cliui.Success("✓ OK"), len(ordenados), ativas, linhas)
	if len(ciclos) > 0 {
		fmt.Println(cliui.Muted(i18n.T("fac_fk_cycle") + strings.Join(ciclos, ", ")))
	}
	return nil
}

func quantidadeDeLinhas(atual plano) int {
	if atual.Arquivo.Ruler.Count <= 0 {
		return 1
	}
	return atual.Arquivo.Ruler.Count
}

// FactoryRun popula as tabelas. targets vazio significa todas as ativas.
// FactoryRun popula as tabelas com dados fake.
//
// forcar repovoa mesmo tabela que já tem dados. Fora dele, tabela com linha é
// pulada: a factory começa com DELETE, e dado que ela não produziu não é dela
// para apagar.
func FactoryRun(root string, state config.ConfigState, targets []string, forcar bool) error {
	planos, err := loadFactories(root, state)
	if err != nil {
		return err
	}
	if len(planos) == 0 {
		fmt.Println(cliui.Muted(i18n.T("fac_none_found_in") + factoryRoot(root, state)))
		return nil
	}

	selecionados, err := selecionaFactories(planos, targets)
	if err != nil {
		return err
	}
	if len(selecionados) == 0 {
		fmt.Println(cliui.Muted(i18n.T("fac_none_active")))
		return nil
	}

	dialect := state.ActiveDialect
	connection := state.Config.Connections[state.ActiveClient]
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	if driver == "" {
		return i18n.Errf("run_dialect_unsupported", dialect)
	}
	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	// Uma factory de tabela que ainda não foi migrada é ignorada, não é erro:
	// o corpus é grande e nem todo ambiente tem tudo aplicado.
	var existentes []plano
	for _, atual := range selecionados {
		existe, err := tableExists(ctx, db, dialect, connection.Schema, atual.Fisica())
		if err != nil {
			return i18n.Errf("fac_check_table_failed", atual.Fisica(), err)
		}
		if existe {
			existentes = append(existentes, atual)
			continue
		}
		fmt.Println(cliui.Muted("  - " + atual.Arquivo.Tabela + i18n.T("fac_skipped_no_table")))
	}
	if len(existentes) == 0 {
		fmt.Println(cliui.Muted(i18n.T("fac_no_table_exists")))
		return nil
	}

	// Tabela que já tem dados NÃO é repovoada.
	//
	// A factory é simulador de dados para teste, e o povoamento dela começa com um
	// DELETE. Se a tabela já tem linha — vinda de seed, de migration ou da própria
	// aplicação —, rodar apagaria dado que a factory não produziu. Sem esta guarda,
	// `factory run` num banco em uso é destrutivo por padrão.
	//
	// Quem realmente quer regerar diz em voz alta com --force.
	var vazias []plano
	for _, atual := range existentes {
		if forcar {
			vazias = append(vazias, atual)
			continue
		}
		temDados, err := tabelaTemDados(ctx, db, dialect, connection.Schema, atual.Fisica())
		if err != nil {
			return i18n.Errf("fac_check_rows_failed", atual.Fisica(), err)
		}
		if temDados {
			fmt.Println(cliui.Muted("  - " + atual.Arquivo.Tabela + i18n.T("fac_skipped_has_rows")))
			continue
		}
		vazias = append(vazias, atual)
	}
	if len(vazias) == 0 {
		fmt.Println(cliui.Muted(i18n.T("fac_nothing_empty")))
		return nil
	}

	ordenados, ciclos := ordenaFactories(vazias)
	if len(ciclos) > 0 {
		fmt.Println(cliui.Muted(i18n.T("fac_fk_cycle") + strings.Join(ciclos, ", ")))
	}

	// Limpa na ordem inversa: filha antes de pai, senão a FK barra o DELETE. Só as
	// tabelas que passaram pela guarda acima chegam aqui — nas outras não há o que
	// limpar, porque não serão repovoadas.
	if err := limpaFactories(ctx, db, dialect, connection.Schema, ordenados); err != nil {
		return err
	}

	inseridas := map[string][]map[string]any{}
	total := 0
	for _, atual := range ordenados {
		linhas, err := executaFactory(ctx, db, dialect, connection.Schema, atual, inseridas)
		if err != nil {
			return i18n.Errf("fac_run_failed", atual.Arquivo.Tabela, err)
		}
		inseridas[atual.Tabela()] = linhas
		total += len(linhas)
		fmt.Printf(i18n.T("fac_row_line"), cliui.Success("✓"), atual.Arquivo.Tabela, len(linhas))
	}

	fmt.Printf(i18n.T("fac_total"), cliui.Success("✓ OK"), len(ordenados), total)
	return nil
}

// selecionaFactories resolve o que rodar. Pedir uma tabela traz junto os pais
// dela, senão o INSERT esbarra na chave estrangeira.
func selecionaFactories(planos []plano, targets []string) ([]plano, error) {
	if len(targets) == 0 {
		var ativas []plano
		for _, atual := range planos {
			if atual.Arquivo.Ruler.Active {
				ativas = append(ativas, atual)
			}
		}
		return ativas, nil
	}

	porTabela := map[string]plano{}
	for _, atual := range planos {
		porTabela[atual.Tabela()] = atual
	}

	escolhidos := map[string]bool{}
	var incluir func(tabela string, caminho []string) error
	incluir = func(tabela string, caminho []string) error {
		tabela = strings.ToUpper(tabela)
		if escolhidos[tabela] {
			return nil
		}
		atual, existe := porTabela[tabela]
		if !existe {
			// Pai sem factory não é erro: a tabela pode já estar populada.
			if len(caminho) > 0 {
				return nil
			}
			return cliui.NewUserError(
				i18n.Tf("fac_missing_for_table", tabela),
				i18n.Tf("fac_missing_for_table_fix", strings.ToLower(tabela)),
			)
		}
		escolhidos[tabela] = true
		for _, pai := range atual.Pais {
			if err := incluir(pai, append(caminho, tabela)); err != nil {
				return err
			}
		}
		return nil
	}

	for _, target := range targets {
		if err := incluir(target, nil); err != nil {
			return nil, err
		}
	}

	// As filhas entram junto, transitivamente. Sem elas o DELETE que precede o
	// INSERT esbarra na chave estrangeira das linhas que já estão lá — pedir
	// uma tabela e receber uma violação de FK não ajudaria ninguém.
	for mudou := true; mudou; {
		mudou = false
		for _, atual := range planos {
			if escolhidos[atual.Tabela()] {
				continue
			}
			for _, pai := range atual.Pais {
				if escolhidos[pai] {
					escolhidos[atual.Tabela()] = true
					mudou = true
					break
				}
			}
		}
	}

	var selecionados []plano
	for _, atual := range planos {
		if escolhidos[atual.Tabela()] {
			selecionados = append(selecionados, atual)
		}
	}
	return selecionados, nil
}

// tabelaTemDados responde se a tabela tem ao menos uma linha.
//
// COUNT(*) e não um SELECT com limite: a sintaxe de limitar linha é diferente nos
// quatro dialetos, e aqui o custo é irrelevante — roda uma vez por tabela, antes
// de povoar dado de teste.
func tabelaTemDados(ctx context.Context, db *sql.DB, dialect, schema, tabela string) (bool, error) {
	var total int64
	consulta := "SELECT COUNT(*) FROM " + qualified(dialect, schema, tabela)
	if err := db.QueryRowContext(ctx, consulta).Scan(&total); err != nil {
		return false, err
	}
	return total > 0, nil
}

func limpaFactories(ctx context.Context, db *sql.DB, dialect, schema string, ordenados []plano) error {
	for posicao := len(ordenados) - 1; posicao >= 0; posicao-- {
		alvo := qualified(dialect, schema, ordenados[posicao].Fisica())
		if _, err := db.ExecContext(ctx, "DELETE FROM "+alvo); err != nil {
			return cliui.NewUserError(
				i18n.Tf("fac_truncate_failed", ordenados[posicao].Arquivo.Tabela, err),
				i18n.T("fac_truncate_failed_fix"),
			)
		}
	}
	return nil
}

// executaFactory gera e insere as linhas de uma factory.
func executaFactory(ctx context.Context, db *sql.DB, dialect, schema string, atual plano, inseridas map[string][]map[string]any) ([]map[string]any, error) {
	quantidade := quantidadeDeLinhas(atual)
	alvo := qualified(dialect, schema, atual.Fisica())

	tipos := map[string]string{}
	for _, column := range atual.Forma.Columns {
		tipos[strings.ToUpper(column.Name)] = column.Type
	}

	transaction, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback()

	// IDENTITY_INSERT é por sessão e, uma vez ligado, o SQL Server passa a
	// exigir valor explícito em toda linha. Como algumas factories escrevem a
	// PK e outras não, a chave é alternada linha a linha.
	identityLigado := false
	alternaIdentity := func(ligar bool) error {
		if dialect != "sqlserver" || atual.Identity == "" || identityLigado == ligar {
			return nil
		}
		verbo := "OFF"
		if ligar {
			verbo = "ON"
		}
		if _, err := transaction.ExecContext(ctx, "SET IDENTITY_INSERT "+alvo+" "+verbo); err != nil {
			return fmt.Errorf("SET IDENTITY_INSERT %s: %w", verbo, err)
		}
		identityLigado = ligar
		return nil
	}

	geradas := make([]map[string]any, 0, quantidade)
	for index := 0; index < quantidade; index++ {
		linha, err := atual.Arquivo.Linha(index)
		if err != nil {
			return nil, err
		}

		if err := resolveReferencias(ctx, transaction, dialect, schema, atual, index, linha, inseridas); err != nil {
			return nil, err
		}

		if err := coageLinha(linha, tipos); err != nil {
			return nil, i18n.Errf("fac_row_failed", index+1, err)
		}

		_, escreveIdentity := linha[atual.Identity]
		if atual.Identity == "" {
			escreveIdentity = false
		}
		if err := alternaIdentity(escreveIdentity); err != nil {
			return nil, err
		}

		colunas := make([]string, 0, len(linha))
		fisicas := make([]string, 0, len(linha))
		valores := make([]any, 0, len(linha))
		marcadores := make([]string, 0, len(linha))
		for _, coluna := range atual.Arquivo.Colunas() {
			valor, existe := linha[coluna]
			if !existe {
				continue
			}
			colunas = append(colunas, coluna)
			fisicas = append(fisicas, atual.colunaFisica(coluna))
			valores = append(valores, valor)
			marcadores = append(marcadores, placeholder(dialect, len(marcadores)+1))
		}

		comando := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			alvo, strings.Join(quoteAll(dialect, fisicas), ", "), strings.Join(marcadores, ", "))

		if _, err := transaction.ExecContext(ctx, comando, valores...); err != nil {
			return nil, erroDeInsercao(atual, index, colunas, valores, err)
		}
		geradas = append(geradas, linha)
	}

	if err := alternaIdentity(false); err != nil {
		return nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, err
	}

	// A sequência precisa passar a apontar depois do maior valor escrito,
	// senão o próximo INSERT da aplicação colide com a linha da factory.
	if atual.Identity != "" {
		// Um migrate.SQL que renomeia a coluna é opaco ao AST, então a forma
		// declarada pode citar um nome que o banco não tem mais. Avisar é
		// melhor que abortar: as linhas já foram gravadas.
		existe, err := columnExists(ctx, db, dialect, schema, atual.Fisica(), atual.Identity)
		if err != nil {
			return nil, i18n.Errf("fac_identity_check_failed", atual.Fisica(), err)
		}
		if !existe {
			fmt.Println(cliui.Muted(i18n.Tf("fac_sequence_skipped", atual.Fisica(), atual.Identity)))
		} else if err := resyncIdentity(ctx, db, dialect, schema, atual.Fisica(), atual.Identity); err != nil {
			return nil, i18n.Errf("fac_sequence_failed", atual.Fisica(), atual.Identity, err)
		}
	}
	return geradas, nil
}

// resolveReferencias preenche as colunas declaradas com Reference, usando as linhas
// que acabaram de ser inseridas na tabela pai. Se o pai não estava na seleção,
// busca no banco.
func resolveReferencias(ctx context.Context, transaction *sql.Tx, dialect, schema string, atual plano, index int, linha map[string]any, inseridas map[string][]map[string]any) error {
	for _, campo := range atual.Arquivo.Campos {
		if campo.Referencia == nil {
			continue
		}
		pai := strings.ToUpper(campo.Referencia.Table)
		coluna := campo.Referencia.Column

		if linhas := inseridas[pai]; len(linhas) > 0 {
			// A restrição de seeder vem primeiro: se o autor listou IDs, só eles
			// valem — desde que existam entre as linhas do pai.
			if valor, restrito := valorRestrito(campo.Referencia, linhas, coluna, index); restrito {
				linha[campo.Coluna] = valor
				continue
			}
			escolhida := linhas[index%len(linhas)]
			if valor, existe := valorDaLinha(escolhida, coluna); existe {
				linha[campo.Coluna] = valor
				continue
			}
		}

		valor, err := valorExistenteNoBanco(ctx, transaction, dialect, schema, campo.Referencia.Table, coluna, index, campo.Referencia)
		if err != nil {
			return err
		}
		linha[campo.Coluna] = valor
	}
	return nil
}

// valorRestrito aplica o Seeder quando o pai foi populado na mesma execução: usa
// só os valores listados, e apenas os que REALMENTE existem entre as linhas dele.
// O caminho equivalente para pai não repovoado é valorExistenteNoBanco.
//
// Devolve restrito=false quando não há restrição, ou quando nenhum dos valores
// listados existe. O chamador cai no comportamento padrão — é o "em caso de erro
// é ignorado e segue o fluxo": FK apontando para linha inexistente seria pior que
// FK apontando para outra linha válida.
//
// A escolha é derivada do ÍNDICE: os valores são percorridos na ordem declarada.
// Um valor é fixo por construção. Não há sorteio — a mesma factory produz o mesmo
// resultado, e é a comparação entre os quatro bancos que sustenta as baterias.
func valorRestrito(link *migrate.Link, linhas []map[string]any, coluna string, index int) (any, bool) {
	if link == nil || link.Modo == "" || len(link.Valores) == 0 {
		return nil, false
	}

	existentes := make([]any, 0, len(link.Valores))
	for _, desejado := range link.Valores {
		for _, linha := range linhas {
			valor, tem := valorDaLinha(linha, coluna)
			if tem && mesmoValor(valor, desejado) {
				existentes = append(existentes, valor)
				break
			}
		}
	}
	if len(existentes) == 0 {
		return nil, false
	}

	posicao := index % len(existentes)
	return existentes[posicao], true
}

// mesmoValor compara o que o autor escreveu com o que veio da linha. O texto é o
// denominador comum: o literal do arquivo pode ser int64 e a coluna devolver
// int32, string ou float dependendo do driver.
func mesmoValor(a, b any) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func valorDaLinha(linha map[string]any, coluna string) (any, bool) {
	if valor, existe := linha[coluna]; existe {
		return valor, true
	}
	for nome, valor := range linha {
		if strings.EqualFold(nome, coluna) {
			return valor, true
		}
	}
	return nil, false
}

// valorExistenteNoBanco pega um valor real da tabela pai. Sem isso a Reference
// só funcionaria quando o pai fosse populado na mesma execução.
//
// É também aqui que o Seeder normalmente age: o caso típico é o pai já ter dado
// de seed e por isso NÃO ser repovoado, então ele nem aparece em `inseridas`.
func valorExistenteNoBanco(ctx context.Context, transaction *sql.Tx, dialect, schema, tabela, coluna string, index int, link *migrate.Link) (any, error) {
	if listados := valoresListados(link); len(listados) > 0 {
		valores, err := leValoresDoPai(ctx, transaction, dialect, schema, tabela, coluna, listados, len(listados))
		if err != nil {
			return nil, err
		}
		if len(valores) > 0 {
			return valores[index%len(valores)], nil
		}
		// Nenhum dos valores listados existe no pai: a restrição é ignorada em
		// silêncio e o fluxo segue no comportamento padrão.
	}

	valores, err := leValoresDoPai(ctx, transaction, dialect, schema, tabela, coluna, nil, index+2)
	if err != nil {
		return nil, err
	}
	if len(valores) == 0 {
		return nil, cliui.NewUserError(
			i18n.Tf("fac_link_parent_empty", tabela, tabela, coluna),
			i18n.Tf("fac_link_parent_empty_fix", strings.ToLower(tabela)),
		)
	}
	return valores[index%len(valores)], nil
}

// valoresListados devolve os valores que o autor escreveu no Seeder, ou nada
// quando o vínculo não tem restrição.
func valoresListados(link *migrate.Link) []any {
	if link == nil || link.Modo == "" {
		return nil
	}
	return link.Valores
}

// leValoresDoPai busca valores reais da coluna do pai, parando em `limite`. Com
// `listados` preenchido, devolve só os que existem — e na ordem em que o autor os
// declarou, não na ordem em que o banco resolveu entregá-los: sem isso os quatro
// dialetos escolheriam alvos diferentes para a mesma linha.
func leValoresDoPai(ctx context.Context, transaction *sql.Tx, dialect, schema, tabela, coluna string, listados []any, limite int) ([]any, error) {
	comando := fmt.Sprintf("SELECT %s FROM %s WHERE %s IS NOT NULL",
		quote(dialect, coluna), qualified(dialect, schema, tabela), quote(dialect, coluna))

	rows, err := transaction.QueryContext(ctx, comando)
	if err != nil {
		return nil, cliui.NewUserError(
			i18n.Tf("fac_link_read_failed", tabela, coluna, err),
			i18n.T("fac_link_read_failed_fix"),
		)
	}
	defer rows.Close()

	encontrados := make([]any, 0, limite)
	for rows.Next() && len(encontrados) < limite {
		var valor any
		if err := rows.Scan(&valor); err != nil {
			return nil, err
		}
		if len(listados) > 0 && !contemValor(listados, valor) {
			continue
		}
		encontrados = append(encontrados, valor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(listados) == 0 {
		return encontrados, nil
	}
	return naOrdemDeclarada(listados, encontrados), nil
}

func contemValor(lista []any, valor any) bool {
	for _, item := range lista {
		if mesmoValor(item, valor) {
			return true
		}
	}
	return false
}

// naOrdemDeclarada reordena o que veio do banco pela ordem do arquivo.
func naOrdemDeclarada(listados, encontrados []any) []any {
	ordenados := make([]any, 0, len(encontrados))
	for _, desejado := range listados {
		for _, valor := range encontrados {
			if mesmoValor(valor, desejado) {
				ordenados = append(ordenados, valor)
				break
			}
		}
	}
	return ordenados
}

// coageLinha ajusta cada valor ao tipo declarado da coluna, do mesmo jeito que
// o seed faz: sem isso o pgx recusa um número em coluna de texto e o Oracle
// recusa um texto em coluna de data.
func coageLinha(linha map[string]any, tipos map[string]string) error {
	for coluna, valor := range linha {
		tipo, conhecido := tipos[strings.ToUpper(coluna)]
		if !conhecido {
			continue
		}
		convertido, err := migrationgo.CoerceValue(valor, tipo)
		if err != nil {
			return i18n.Errf("fac_column_failed", coluna, err)
		}
		linha[coluna] = convertido
	}
	return nil
}

func quoteAll(dialect string, colunas []string) []string {
	citadas := make([]string, len(colunas))
	for posicao, coluna := range colunas {
		citadas[posicao] = quote(dialect, coluna)
	}
	return citadas
}

// erroDeInsercao traduz a falha do driver para algo acionável. O erro cru dos
// quatro bancos cita o nome da constraint, não a coluna.
func erroDeInsercao(atual plano, index int, colunas []string, valores []any, err error) error {
	texto := err.Error()
	baixo := strings.ToLower(texto)

	var solucao string
	switch {
	case strings.Contains(baixo, "foreign key"), strings.Contains(baixo, "ora-02291"), strings.Contains(baixo, "violates foreign key"):
		solucao = i18n.T("fac_advice_fk")
	case strings.Contains(baixo, "check constraint"), strings.Contains(baixo, "ora-02290"):
		solucao = i18n.T("fac_advice_check")
	case strings.Contains(baixo, "too large"), strings.Contains(baixo, "ora-12899"), strings.Contains(baixo, "too long"), strings.Contains(baixo, "truncated"):
		solucao = i18n.T("fac_advice_too_long")
	case strings.Contains(baixo, "unique"), strings.Contains(baixo, "ora-00001"), strings.Contains(baixo, "duplicate"):
		solucao = i18n.T("fac_advice_unique")
	case strings.Contains(baixo, "cannot be null"), strings.Contains(baixo, "ora-01400"), strings.Contains(baixo, "null value in column"):
		solucao = i18n.T("fac_advice_not_null")
	default:
		solucao = i18n.T("fac_advice_generic")
	}

	return cliui.NewUserError(
		i18n.Tf("fac_insert_failed", index+1, atual.Arquivo.Tabela, err, resumoDeValores(colunas, valores)),
		solucao,
	)
}

func resumoDeValores(colunas []string, valores []any) string {
	partes := make([]string, 0, len(colunas))
	for posicao, coluna := range colunas {
		texto := fmt.Sprintf("%v", valores[posicao])
		if len(texto) > 40 {
			texto = texto[:37] + "..."
		}
		partes = append(partes, coluna+"="+texto)
	}
	return strings.Join(partes, ", ")
}
