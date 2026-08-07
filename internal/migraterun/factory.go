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

	"gokit/internal/cliui"
	"gokit/internal/config"
	"gokit/internal/factorygo"
	"gokit/internal/migrationgo"
	"gokit/migration/acao"
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
	for _, column := range p.Forma.Columns {
		if strings.EqualFold(column.Name, nome) {
			return column.Name
		}
	}
	return nome
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

		// O Vinculo é escrito em caixa alta na factory, mas quem vai à consulta
		// é o nome que a migration declarou — no Postgres e no MySQL a caixa
		// importa. A tradução acontece aqui, uma vez, e não a cada linha.
		for posicao := range atual.Arquivo.Campos {
			link := atual.Arquivo.Campos[posicao].Vinculo
			if link == nil {
				continue
			}
			paiForma, existe := formas[strings.ToLower(link.Table)]
			if !existe {
				continue
			}
			link.Table = paiForma.Table
			for _, column := range paiForma.Columns {
				if strings.EqualFold(column.Name, link.Column) {
					link.Column = column.Name
					break
				}
			}
		}

		// Um pai pode vir da FK declarada na migration ou de um Vinculo
		// escrito na factory. Os dois valem.
		vistos := map[string]bool{}
		for _, pai := range append(fks[atual.Tabela()], arquivo.Vinculos()...) {
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
		fmt.Println(cliui.Muted("Nenhuma factory encontrada em " + factoryRoot(root, state)))
		return nil
	}

	var problemas []string
	ativas, linhas := 0, 0

	for _, atual := range planos {
		if !atual.Conhece {
			problemas = append(problemas, fmt.Sprintf(
				"%s: a tabela %s não é criada por nenhuma migration",
				filepath.Base(atual.Arquivo.Caminho), atual.Arquivo.Tabela))
			continue
		}

		// Coluna que não existe na tabela derruba o INSERT inteiro; melhor
		// avisar aqui.
		declaradas := map[string]bool{}
		for _, column := range atual.Forma.Columns {
			declaradas[strings.ToUpper(column.Name)] = true
		}
		for _, coluna := range atual.Arquivo.Colunas() {
			if !declaradas[strings.ToUpper(coluna)] {
				problemas = append(problemas, fmt.Sprintf(
					"%s: a coluna %s não existe em %s",
					filepath.Base(atual.Arquivo.Caminho), coluna, atual.Arquivo.Tabela))
			}
		}

		if atual.Arquivo.Ruler.Active {
			ativas++
			linhas += quantidadeDeLinhas(atual)
		}
	}

	if len(problemas) > 0 {
		return cliui.NewUserError(
			fmt.Sprintf("%d problema(s) nas factories:\n  - %s", len(problemas), strings.Join(problemas, "\n  - ")),
			"Rode `gokit factory create <tabela>` para regerar a factory a partir da migration.",
		)
	}

	ordenados, ciclos := ordenaFactories(planos)
	fmt.Printf("  %s %d factory(ies), %d ativa(s), %d linha(s) a gerar\n",
		cliui.Success("✓ OK"), len(ordenados), ativas, linhas)
	if len(ciclos) > 0 {
		fmt.Println(cliui.Muted("  Ciclo de chave estrangeira rompido em: " + strings.Join(ciclos, ", ")))
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
func FactoryRun(root string, state config.ConfigState, targets []string) error {
	planos, err := loadFactories(root, state)
	if err != nil {
		return err
	}
	if len(planos) == 0 {
		fmt.Println(cliui.Muted("Nenhuma factory encontrada em " + factoryRoot(root, state)))
		return nil
	}

	selecionados, err := selecionaFactories(planos, targets)
	if err != nil {
		return err
	}
	if len(selecionados) == 0 {
		fmt.Println(cliui.Muted("Nenhuma factory ativa para executar."))
		return nil
	}

	dialect := state.ActiveDialect
	connection := state.Config.Connections[state.ActiveClient]
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	if driver == "" {
		return fmt.Errorf("dialeto não suportado: %s", dialect)
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
			return fmt.Errorf("verificar a tabela %s: %w", atual.Fisica(), err)
		}
		if existe {
			existentes = append(existentes, atual)
			continue
		}
		fmt.Println(cliui.Muted("  - " + atual.Arquivo.Tabela + " ignorada: a tabela não existe no banco"))
	}
	if len(existentes) == 0 {
		fmt.Println(cliui.Muted("Nenhuma tabela das factories selecionadas existe no banco."))
		return nil
	}

	ordenados, ciclos := ordenaFactories(existentes)
	if len(ciclos) > 0 {
		fmt.Println(cliui.Muted("  Ciclo de chave estrangeira rompido em: " + strings.Join(ciclos, ", ")))
	}

	// Limpa na ordem inversa: filha antes de pai, senão a FK barra o DELETE.
	if err := limpaFactories(ctx, db, dialect, connection.Schema, ordenados); err != nil {
		return err
	}

	inseridas := map[string][]map[string]any{}
	total := 0
	for _, atual := range ordenados {
		linhas, err := executaFactory(ctx, db, dialect, connection.Schema, atual, inseridas)
		if err != nil {
			return fmt.Errorf("factory de %s: %w", atual.Arquivo.Tabela, err)
		}
		inseridas[atual.Tabela()] = linhas
		total += len(linhas)
		fmt.Printf("  %s %-42s %d linha(s)\n", cliui.Success("✓"), atual.Arquivo.Tabela, len(linhas))
	}

	fmt.Printf("\n  %s %d tabela(s), %d linha(s) inserida(s)\n", cliui.Success("✓ OK"), len(ordenados), total)
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
				fmt.Sprintf("Não existe factory para a tabela %s.", tabela),
				"Rode `gokit factory create "+strings.ToLower(tabela)+"` para criá-la.",
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

func limpaFactories(ctx context.Context, db *sql.DB, dialect, schema string, ordenados []plano) error {
	for posicao := len(ordenados) - 1; posicao >= 0; posicao-- {
		alvo := qualified(dialect, schema, ordenados[posicao].Fisica())
		if _, err := db.ExecContext(ctx, "DELETE FROM "+alvo); err != nil {
			return cliui.NewUserError(
				fmt.Sprintf("Não foi possível limpar %s: %v", ordenados[posicao].Arquivo.Tabela, err),
				"Alguma tabela filha fora da seleção referencia estas linhas. Rode as factories sem filtro ou inclua a tabela filha.",
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

		if err := resolveVinculos(ctx, transaction, dialect, schema, atual, index, linha, inseridas); err != nil {
			return nil, err
		}

		if err := coageLinha(linha, tipos); err != nil {
			return nil, fmt.Errorf("linha %d: %w", index+1, err)
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
			return nil, fmt.Errorf("verificar a coluna de identidade de %s: %w", atual.Fisica(), err)
		}
		if !existe {
			fmt.Println(cliui.Muted(fmt.Sprintf(
				"  - %s: a sequência não foi ressincronizada; a coluna %s não existe no banco (renomeada por migrate.SQL?)",
				atual.Fisica(), atual.Identity)))
		} else if err := resyncIdentity(ctx, db, dialect, schema, atual.Fisica(), atual.Identity); err != nil {
			return nil, fmt.Errorf("ressincronizar a sequência de %s.%s: %w", atual.Fisica(), atual.Identity, err)
		}
	}
	return geradas, nil
}

// resolveVinculos preenche as colunas declaradas com Vinculo, usando as linhas
// que acabaram de ser inseridas na tabela pai. Se o pai não estava na seleção,
// busca no banco.
func resolveVinculos(ctx context.Context, transaction *sql.Tx, dialect, schema string, atual plano, index int, linha map[string]any, inseridas map[string][]map[string]any) error {
	for _, campo := range atual.Arquivo.Campos {
		if campo.Vinculo == nil {
			continue
		}
		pai := strings.ToUpper(campo.Vinculo.Table)
		coluna := campo.Vinculo.Column

		if linhas := inseridas[pai]; len(linhas) > 0 {
			escolhida := linhas[index%len(linhas)]
			if valor, existe := valorDaLinha(escolhida, coluna); existe {
				linha[campo.Coluna] = valor
				continue
			}
		}

		valor, err := valorExistenteNoBanco(ctx, transaction, dialect, schema, campo.Vinculo.Table, coluna, index)
		if err != nil {
			return err
		}
		linha[campo.Coluna] = valor
	}
	return nil
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

// valorExistenteNoBanco pega um valor real da tabela pai. Sem isso o Vinculo
// só funcionaria quando o pai fosse populado na mesma execução.
func valorExistenteNoBanco(ctx context.Context, transaction *sql.Tx, dialect, schema, tabela, coluna string, index int) (any, error) {
	comando := fmt.Sprintf("SELECT %s FROM %s WHERE %s IS NOT NULL",
		quote(dialect, coluna), qualified(dialect, schema, tabela), quote(dialect, coluna))

	rows, err := transaction.QueryContext(ctx, comando)
	if err != nil {
		return nil, cliui.NewUserError(
			fmt.Sprintf("Não foi possível ler %s.%s para resolver o vínculo: %v", tabela, coluna, err),
			"Confira se a tabela e a coluna do Vinculo estão escritas como na migration.",
		)
	}
	defer rows.Close()

	var valores []any
	for rows.Next() && len(valores) <= index+1 {
		var valor any
		if err := rows.Scan(&valor); err != nil {
			return nil, err
		}
		valores = append(valores, valor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(valores) == 0 {
		return nil, cliui.NewUserError(
			fmt.Sprintf("A tabela %s está vazia e o vínculo com %s.%s não pode ser resolvido.", tabela, tabela, coluna),
			"Rode a factory de "+strings.ToLower(tabela)+" antes, ou execute sem filtro para que o gokit ordene sozinho.",
		)
	}
	return valores[index%len(valores)], nil
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
			return fmt.Errorf("coluna %s: %w", coluna, err)
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
		solucao = "A tabela pai não tem a linha referenciada. Use migrate.Vinculo(\"TABELA_PAI\", \"COLUNA\") nessa coluna em vez de um valor fake."
	case strings.Contains(baixo, "check constraint"), strings.Contains(baixo, "ora-02290"):
		solucao = "O valor gerado não passa no CHECK da coluna. Troque por migrate.FakeChoiceIndex(index, ...) com os valores que o CHECK aceita."
	case strings.Contains(baixo, "too large"), strings.Contains(baixo, "ora-12899"), strings.Contains(baixo, "too long"), strings.Contains(baixo, "truncated"):
		solucao = "O valor gerado é maior que a coluna. Passe o tamanho da coluna na função Fake*, por exemplo migrate.FakeUniqueText(index, \"Prefixo\", 30)."
	case strings.Contains(baixo, "unique"), strings.Contains(baixo, "ora-00001"), strings.Contains(baixo, "duplicate"):
		solucao = "Duas linhas geraram o mesmo valor numa coluna única. Use a variante por índice, como migrate.FakeUniqueText ou migrate.FakeIntIndex."
	case strings.Contains(baixo, "cannot be null"), strings.Contains(baixo, "ora-01400"), strings.Contains(baixo, "null value in column"):
		solucao = "Uma coluna obrigatória ficou de fora do Data da factory."
	default:
		solucao = "Confira o Data da factory contra as colunas declaradas na migration."
	}

	return cliui.NewUserError(
		fmt.Sprintf("Falha ao inserir a linha %d de %s: %v\n  Valores: %s",
			index+1, atual.Arquivo.Tabela, err, resumoDeValores(colunas, valores)),
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
