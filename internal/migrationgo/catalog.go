package migrationgo

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/PhelipeViana/gokit/internal/i18n"
)

// Caminhos do catálogo.
//
// O catálogo mora no pacote core unificado, ao lado das entidades da ORM, para que
// a aplicação tenha um import só. Os caminhos antigos continuam sendo LIDOS: a
// mudança de pacote já aconteceu uma vez (de `table` para `alias`) e a saída foi
// exatamente esta — ler o antigo e mesclar, para que projeto que ainda não
// regenerou continue funcionando.
//
// A ordem importa: o caminho novo é o último, então ele tem a palavra final.
func caminhoDoCatalogo(projectRoot string) string {
	return filepath.Join(projectRoot, "internal", "gokit", "core", "table.gen.go")
}

func caminhoDoCatalogoDeViews(projectRoot string) string {
	return filepath.Join(projectRoot, "internal", "gokit", "core", "view.gen.go")
}

func caminhoDoCatalogoDeColunas(projectRoot string) string {
	return filepath.Join(projectRoot, "internal", "gokit", "core", "column.gen.go")
}

// colunasPorTabela acumula, por tabela, TODA coluna que já apareceu no corpus.
//
// A leitura é por AST tolerante — o parser do Go, não o do DSL —, igual à do
// catálogo de tabelas: assim ela funciona com corpus que ainda não descreve um
// schema válido, e não depende de a migration estar preenchida.
//
// Acumula e nunca remove, pela mesma razão das tabelas: a migration que derrubou
// uma coluna cita o nome dela, e precisa continuar compilando.
//
// A atribuição coluna→tabela sai da ÁRVORE da chamada: em
// CreateTable("cidades", Col("id"), Col("nome")) os Col são argumentos do
// CreateTable, então basta descer na subárvore de cada operação que recebe tabela.
//
// O erro da varredura sobe. Cada arquivo ilegível já é tolerado dentro da função
// (justamente porque o corpus pode não compilar), mas falha na RAIZ é outra coisa: o
// mapa sai vazio, o catálogo de colunas é reescrito sem nenhuma coluna, e toda
// migration que cita core.Column.X.Y para de compilar. Isso não pode passar por
// "nenhuma coluna encontrada".
func colunasPorTabela(migrationsFolder string, tabelas map[string]string) (map[string]map[string]bool, error) {
	acumulado := map[string]map[string]bool{}

	// operacoesComTabela são as que trazem tabela no 1º argumento e coluna dentro.
	operacoesComTabela := map[string]bool{
		"CreateTable": true, "AddColumn": true, "AlterColumn": true, "DropColumn": true,
	}

	erroDaVarredura := filepath.WalkDir(migrationsFolder, func(caminho string, entrada fs.DirEntry, err error) error {
		if err != nil || entrada.IsDir() || !strings.HasSuffix(entrada.Name(), ".go") {
			return nil
		}
		arquivo, parseErr := parser.ParseFile(token.NewFileSet(), caminho, nil, 0)
		if parseErr != nil {
			// Arquivo que ainda não é Go válido não interrompe o catálogo: ele é
			// justamente o que precisa do catálogo para passar a compilar.
			return nil
		}
		ast.Inspect(arquivo, func(no ast.Node) bool {
			chamada, ok := no.(*ast.CallExpr)
			if !ok || len(chamada.Args) == 0 {
				return true
			}
			seletor, ok := chamada.Fun.(*ast.SelectorExpr)
			if !ok || !operacoesComTabela[seletor.Sel.Name] {
				return true
			}
			tabela := tabelaDoArgumento(chamada.Args[0], tabelas)
			if tabela == "" {
				return true
			}
			chave := strings.ToLower(tabela)
			if acumulado[chave] == nil {
				acumulado[chave] = map[string]bool{}
			}
			for _, coluna := range colunasNaSubarvore(chamada) {
				acumulado[chave][coluna] = true
			}
			return true
		})
		return nil
	})
	// Pasta que ainda não existe é projeto novo, não falha: o catálogo sai vazio porque
	// não há corpus, e isso é a verdade. Qualquer outro erro esconderia corpus real.
	if erroDaVarredura != nil && !os.IsNotExist(erroDaVarredura) {
		return nil, erroDaVarredura
	}
	return acumulado, nil
}

// tabelaDoArgumento resolve o 1º argumento de uma operação: literal ("cidades")
// ou referência de catálogo (core.Table.Cidades / alias.Cidades), que é traduzida
// pelo catálogo de tabelas.
func tabelaDoArgumento(expressao ast.Expr, tabelas map[string]string) string {
	if nome, ok := quotedLiteral(expressao); ok {
		return nome
	}
	seletor, ok := expressao.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if fisico, tem := tabelas[seletor.Sel.Name]; tem {
		return fisico
	}
	return ""
}

// colunasNaSubarvore acha todo Col("x") dentro de uma chamada.
func colunasNaSubarvore(no ast.Node) []string {
	var saida []string
	ast.Inspect(no, func(interno ast.Node) bool {
		chamada, ok := interno.(*ast.CallExpr)
		if !ok || len(chamada.Args) == 0 {
			return true
		}
		seletor, ok := chamada.Fun.(*ast.SelectorExpr)
		if !ok || seletor.Sel.Name != "Col" {
			return true
		}
		if nome, ok := quotedLiteral(chamada.Args[0]); ok && nome != "" {
			saida = append(saida, nome)
		}
		return true
	})
	return saida
}

// caminhosLegadosDoCatalogo são as versões anteriores, só para leitura.
func caminhosLegadosDoCatalogo(projectRoot string) []string {
	return []string{
		filepath.Join(projectRoot, "internal", "gokit", "core", "migration", "table", "dsl.gen.go"),
		filepath.Join(projectRoot, "internal", "gokit", "core", "migration", "alias", "dsl.gen.go"),
	}
}

func caminhosLegadosDeViews(projectRoot string) []string {
	return []string{
		filepath.Join(projectRoot, "internal", "gokit", "core", "migration", "view", "dsl.gen.go"),
	}
}

// catalogoAcumulado mescla os catálogos legados com o atual, nessa ordem. É o que
// preserva o alias de tabela já derrubada: o catálogo acumula e nunca remove,
// porque a migration que derrubou a tabela cita o alias dela e precisa continuar
// compilando.
func catalogoAcumulado(projectRoot string) map[string]string {
	acumulado := map[string]string{}
	for _, caminho := range append(caminhosLegadosDoCatalogo(projectRoot), caminhoDoCatalogo(projectRoot)) {
		// loadCatalog devolve o mapa do cache compartilhado — copiar antes de mesclar.
		for chave, valor := range loadCatalog(caminho) {
			acumulado[chave] = valor
		}
	}
	return acumulado
}

// identificadorPorFisico mapeia nome físico (em minúsculas) para o identificador Go
// que o catálogo de tabelas já usa para ele.
//
// Existe porque o identificador NÃO pode ser derivado duas vezes. O catálogo de
// tabelas desempata colisão dando apelido; o de colunas recebe nome físico literal,
// vindo do texto da migration. Derivando por conta própria, os dois discordavam: o de
// tabelas escrevia EventosBkp435037 e EventosBkp435037Alias2, e o de colunas escrevia
// EventosBkp435037 duas vezes — arquivo que não compila, com `validate` passando,
// porque validate não compila nada.
//
// Tabela sem comentário `// physical:` não passou por desempate, então o valor do
// catálogo já É o nome físico.
func identificadorPorFisico(projectRoot string) map[string]string {
	porFisico := map[string]string{}
	caminhos := append(caminhosLegadosDoCatalogo(projectRoot), caminhoDoCatalogo(projectRoot))
	for _, caminho := range caminhos {
		dados, err := os.ReadFile(caminho)
		if err != nil {
			continue
		}
		for _, casado := range tableEntryComFisico.FindAllStringSubmatch(string(dados), -1) {
			identificador, apelido, fisico := casado[1], casado[2], casado[3]
			if fisico == "" {
				fisico = apelido
			}
			porFisico[strings.ToLower(fisico)] = identificador
		}
	}
	return porFisico
}

// IdentifierByPhysical expõe o mapa nome físico → identificador Go do catálogo.
//
// É a MESMA decisão que o catálogo de tabelas tomou, e existe para que nenhum outro
// gerador derive o identificador por conta própria. Quem derivou de novo já discordou
// duas vezes: o catálogo de colunas e o mapeamento da ORM, os dois emitindo a mesma
// declaração duas vezes quando dois nomes físicos colapsam no mesmo identificador.
func IdentifierByPhysical(projectRoot string) map[string]string {
	return identificadorPorFisico(projectRoot)
}

// RefreshCatalog rebuilds the table catalog in GoKit's Core area from the tables already known
// by the catalog and from migrate.CreateTable calls written in migration files.
func RefreshCatalog(projectRoot string, migrationsFolder string) error {
	// O catálogo reflete o CORPUS, e nada além dele. Não há semente do arquivo anterior:
	// a migration é a fonte, e guardar entrada que o corpus não declara mais é guardar
	// verdade fora dela.
	//
	// A acumulação que existia aqui dizia proteger a migration que DERRUBOU uma tabela e
	// cita o apelido dela. Não era preciso: o `CreateTable` daquela tabela continua no
	// corpus, então o apelido continua derivável.
	//
	// E o custo era alto. Esta função gravava por NOME, tratando apelido e nome físico
	// como a mesma coisa e sem guarda de colisão — então `eventos_bkp435037` e
	// `eventos_bkp_435037` colapsavam num identificador só, em silêncio. Como ela roda no
	// passo `refresh_catalog` do reload, que é do Grupo 3, ela era o ÚLTIMO escritor: o
	// comando que existe para "cuidar de tudo" desfazia o desempate de apelido.
	//
	// Agora deriva apelido → físico, igual ao executor, e grava por WriteCoreCatalog, que
	// tem a guarda de colisão.
	aliases := map[string]string{}
	// apelidados marca o nome físico que já veio com `.Alias(...)`, para o passe de
	// fallback não sobrescrever o apelido declarado por um derivado.
	apelidados := map[string]bool{}

	err := filepath.WalkDir(migrationsFolder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || entry.Name() == "dsl.gen.go" {
			return nil
		}

		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return i18n.Errf("cat_read_tables_failed", entry.Name(), parseErr)
		}
		// Primeiro os que declaram apelido: `CreateTable("x", ...).Alias("y")` é uma
		// chamada de `Alias` cujo receptor é o `CreateTable`, então o nó externo é o
		// Alias. Este passe tem de vir antes do fallback.
		ast.Inspect(file, func(node ast.Node) bool {
			fisico, apelido, ok := createTableComApelido(node)
			if !ok {
				return true
			}
			aliases[apelido] = fisico
			apelidados[fisico] = true
			return true
		})

		// Depois os que não declaram: o apelido vira o identificador derivado, que é o
		// mesmo default que o parser aplica no arquivo `_migration.go` legado.
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || identName(selector.X) != "migrate" || selector.Sel.Name != "CreateTable" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			nome, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr != nil || nome == "" || apelidados[nome] {
				return true
			}
			aliases[tableIdentifier(nome)] = nome
			return true
		})
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(caminhoDoCatalogo(projectRoot)), 0o755); err != nil {
		return err
	}
	return WriteCoreCatalog(projectRoot, aliases)
}

// createTableComApelido reconhece `migrate.CreateTable("fisico", ...).Alias("apelido")`
// e devolve o par.
//
// Existe como função porque o mesmo reconhecimento é feito em dois lugares — aqui e no
// PrimeCoreCatalog — e ter duas cópias do padrão AST era garantia de divergirem.
func createTableComApelido(node ast.Node) (fisico, apelido string, ok bool) {
	aliasCall, certo := node.(*ast.CallExpr)
	if !certo || len(aliasCall.Args) != 1 {
		return "", "", false
	}
	aliasSelector, certo := aliasCall.Fun.(*ast.SelectorExpr)
	if !certo || aliasSelector.Sel.Name != "Alias" {
		return "", "", false
	}
	createCall, certo := aliasSelector.X.(*ast.CallExpr)
	if !certo || len(createCall.Args) == 0 {
		return "", "", false
	}
	createSelector, certo := createCall.Fun.(*ast.SelectorExpr)
	if !certo || identName(createSelector.X) != "migrate" || createSelector.Sel.Name != "CreateTable" {
		return "", "", false
	}
	fisico, fisicoOK := quotedLiteral(createCall.Args[0])
	apelido, apelidoOK := quotedLiteral(aliasCall.Args[0])
	if !fisicoOK || !apelidoOK || fisico == "" || apelido == "" {
		return "", "", false
	}
	return fisico, apelido, true
}

// grupoDeCatalogo emite o agrupador do pacote core:
//
//	var Table = struct {
//		Users migrate.Table
//	}{
//		Users: migrate.Table("users"), // physical: users
//	}
//
// O agrupador existe porque o pacote é um só: `core.Users` é a ENTIDADE (Model,
// Column, Relation) e `core.Table.Users` é a identidade FÍSICA. Sem ele os dois
// colidiriam no mesmo nome.
//
// A forma de uma atribuição por linha não é estética: é o que o regex de leitura
// do catálogo já reconhece, então escrita e leitura seguem casadas.
//
// fisico devolve o nome físico de cada entrada (só para o comentário); pode ser nil.
func grupoDeCatalogo(nome, tipo, construtor string, entradas map[string]string, fisico map[string]string) string {
	chaves := make([]string, 0, len(entradas))
	for chave := range entradas {
		chaves = append(chaves, chave)
	}
	sort.Strings(chaves)

	var source strings.Builder
	if len(chaves) == 0 {
		// Mantém o pacote válido e o nome utilizável em projeto ainda sem tabela.
		fmt.Fprintf(&source, "var %s = struct{}{}\n", nome)
		return source.String()
	}
	fmt.Fprintf(&source, "var %s = struct {\n", nome)
	for _, chave := range chaves {
		// O TIPO do campo, não o construtor: para view os dois diferem.
		fmt.Fprintf(&source, "\t%s migrate.%s\n", chave, tipo)
	}
	source.WriteString("}{\n")
	for _, chave := range chaves {
		comentario := ""
		if fisico != nil && fisico[chave] != "" && fisico[chave] != entradas[chave] {
			comentario = " // physical: " + fisico[chave]
		}
		fmt.Fprintf(&source, "\t%s: migrate.%s(%q),%s\n", chave, construtor, entradas[chave], comentario)
	}
	source.WriteString("}\n")
	return source.String()
}

// escreverCatalogo grava o arquivo do agrupador no pacote core.
// escreverCatalogo grava um agrupador de catálogo.
//
// `tipo` e `construtor` são parâmetros SEPARADOS porque não são a mesma coisa. Para
// tabela coincidem — `migrate.Table` é tipo e conversão ao mesmo tempo. Para view
// não: o tipo é `migrate.View` e o construtor é `migrate.RegisteredView`, porque View
// tem campo privado e não aceita conversão direta. Enquanto os dois vinham do mesmo
// argumento, o view.gen.go saía com `X migrate.RegisteredView` — usando uma função
// como tipo — e não compilava. Não aparecia porque o único projeto de teste tinha
// ZERO views, e aí o agrupador degenera para `struct{}{}`.
func escreverCatalogo(path, nome, tipo, construtor string, entradas, fisico map[string]string, nota string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var source strings.Builder
	source.WriteString("// Code generated by GoKit. DO NOT EDIT.\npackage core\n\n")
	if len(entradas) > 0 {
		source.WriteString("import migrate \"github.com/PhelipeViana/gokit/migration\"\n\n")
	}
	source.WriteString(nota)
	source.WriteString(grupoDeCatalogo(nome, tipo, construtor, entradas, fisico))

	formatted, err := format.Source([]byte(source.String()))
	if err != nil {
		return err
	}
	// Apaga antes de escrever para vencer arquivo marcado como somente-leitura, o que
	// acontece em pasta gerada versionada. O erro é descartado porque o WriteFile logo
	// abaixo é quem sabe se o arquivo pôde ou não ser gravado.
	_ = os.Remove(path)
	return os.WriteFile(path, formatted, 0o644)
}

func writeCatalog(path string, names []string) error {
	entradas := make(map[string]string, len(names))
	for _, name := range names {
		entradas[exportedIdentifier(name)] = name
	}
	return escreverCatalogo(path, "Table", "Table", "Table", entradas, nil, "")
}

// WriteCoreCatalog writes the project-owned alias catalog in GoKit's
// protected core area. Keys are stable aliases and values are physical names.
func WriteCoreCatalog(projectRoot string, aliases map[string]string) error {
	names := make([]string, 0, len(aliases))
	for alias := range aliases {
		names = append(names, alias)
	}
	sort.Strings(names)

	// Aliases distintos podem colapsar no mesmo identificador Go
	// (ex.: "milPst" e "mil_pst" viram MilPst), o que geraria um catálogo que
	// não compila. Barramos aqui, com a mensagem apontando os dois culpados.
	claimed := make(map[string]string, len(names))
	entradas := make(map[string]string, len(names))
	fisico := make(map[string]string, len(names))
	for _, alias := range names {
		identifier := exportedIdentifier(alias)
		if previous, taken := claimed[identifier]; taken {
			return i18n.Errf("cat_alias_collision", previous, alias, identifier)
		}
		claimed[identifier] = alias
		// A CHAVE do catálogo é o alias, e o nome físico entra só como comentário —
		// é o alias que a migration cita, e o motor resolve para o físico depois.
		entradas[identifier] = alias
		fisico[identifier] = aliases[alias]
	}
	return escreverCatalogo(caminhoDoCatalogo(projectRoot), "Table", "Table", "Table",
		entradas, fisico, i18n.T("cat_gen_alias_note"))
}

// PrimeCoreCatalog rebuilds the minimum alias catalog directly from
// CreateTable(...).Alias(...) syntax. It does not depend on the current catalog,
// so Reload can recover automatically if a generated catalog is missing or
// incomplete.
func PrimeCoreCatalog(projectRoot string, paths []string) error {
	aliases := map[string]string{}
	for _, path := range paths {
		if !strings.HasSuffix(strings.ToLower(path), ".go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return i18n.Errf("cat_read_aliases_failed", filepath.Base(path), err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			// O reconhecimento do padrão é compartilhado com o RefreshCatalog: duas
			// cópias do mesmo caminho de AST divergiriam na primeira mudança do DSL.
			physical, alias, ok := createTableComApelido(node)
			if !ok {
				return true
			}
			aliases[alias] = physical
			return true
		})
		continue
	}
	return WriteCoreCatalog(projectRoot, aliases)
}

// WriteCoreColumnCatalog escreve o catálogo de colunas no pacote core.
//
// A forma é um agrupador de dois níveis — core.Column.<Tabela>.<Coluna> —, irmão
// do core.Table. Ficam separados de propósito: assim `core.Table.X` continua
// idêntico ao que as migrations já escrevem (zero migração), e uma coluna chamada
// "table" não colide com o nome da tabela.
func WriteCoreColumnCatalog(projectRoot, migrationsFolder string) error {
	tabelas := catalogoAcumulado(projectRoot)
	porTabela, err := colunasPorTabela(migrationsFolder, tabelas)
	if err != nil {
		return err
	}

	// Identificador de tabela → o de coluna, já normalizado, com a guarda de
	// colisão. Duas colunas que colapsam no mesmo identificador não têm .Alias()
	// para desempatar; a saída é renomear na migration, e a mensagem diz isso.
	type entrada struct {
		identificador string
		colunas       map[string]string // identificador Go -> nome físico
	}
	entradas := make([]entrada, 0, len(porTabela))
	tabelasOrdenadas := make([]string, 0, len(porTabela))
	for tabela := range porTabela {
		tabelasOrdenadas = append(tabelasOrdenadas, tabela)
	}
	sort.Strings(tabelasOrdenadas)

	// O identificador da tabela vem do catálogo de TABELAS, que é quem decidiu o
	// desempate. Derivar aqui de novo fazia os dois arquivos discordarem.
	identificadores := identificadorPorFisico(projectRoot)
	ocupados := map[string]string{}

	for _, tabela := range tabelasOrdenadas {
		colunas := map[string]string{}
		nomes := make([]string, 0, len(porTabela[tabela]))
		for coluna := range porTabela[tabela] {
			nomes = append(nomes, coluna)
		}
		sort.Strings(nomes)
		for _, coluna := range nomes {
			identificador := ExportedIdentifier(coluna)
			if anterior, ocupado := colunas[identificador]; ocupado {
				return i18n.Errf("cat_column_collision", tabela, anterior, coluna, identificador)
			}
			colunas[identificador] = coluna
		}
		identificadorDaTabela, temNoCatalogo := identificadores[strings.ToLower(tabela)]
		if !temNoCatalogo {
			// Tabela que o catálogo ainda não conhece: o corpus foi escrito à mão e o
			// RefreshCatalog não rodou. Derivar é o certo aqui — é o mesmo que o
			// catálogo de tabelas vai derivar quando rodar.
			identificadorDaTabela = ExportedIdentifier(tabela)
		}
		// Guarda de colisão no nível da TABELA. Com o identificador vindo do catálogo
		// isto não deve acontecer; se acontecer, é erro alto e não arquivo quebrado —
		// duas structs com o mesmo nome não compilam, e o compilador aponta para um
		// arquivo gerado que ninguém escreveu.
		if anterior, ocupado := ocupados[identificadorDaTabela]; ocupado {
			return i18n.Errf("cat_table_collision", anterior, tabela, identificadorDaTabela)
		}
		ocupados[identificadorDaTabela] = tabela
		entradas = append(entradas, entrada{identificador: identificadorDaTabela, colunas: colunas})
	}
	// A ordem de saída passa a ser a do identificador, não a do nome físico: o apelido
	// pode reordenar, e diff estável entre execuções vale mais que a ordem original.
	sort.Slice(entradas, func(i, j int) bool { return entradas[i].identificador < entradas[j].identificador })

	var corpo strings.Builder
	corpo.WriteString("// Code generated by GoKit. DO NOT EDIT.\npackage core\n\n")
	if len(entradas) > 0 {
		corpo.WriteString("import migrate \"github.com/PhelipeViana/gokit/migration\"\n\n")
	}
	corpo.WriteString(i18n.T("cat_gen_column_note"))
	if len(entradas) == 0 {
		corpo.WriteString("var Column = struct{}{}\n")
	} else {
		// O tipo anônimo é declarado duas vezes — na struct e no literal — porque um
		// agrupador aninhado não tem nome. Sai formatado pelo go/format no fim.
		corpo.WriteString("var Column = struct {\n")
		for _, e := range entradas {
			fmt.Fprintf(&corpo, "\t%s struct {\n", e.identificador)
			for _, id := range chavesOrdenadas(e.colunas) {
				fmt.Fprintf(&corpo, "\t\t%s migrate.ColumnName\n", id)
			}
			corpo.WriteString("\t}\n")
		}
		corpo.WriteString("}{\n")
		for _, e := range entradas {
			fmt.Fprintf(&corpo, "\t%s: struct {\n", e.identificador)
			for _, id := range chavesOrdenadas(e.colunas) {
				fmt.Fprintf(&corpo, "\t\t%s migrate.ColumnName\n", id)
			}
			corpo.WriteString("\t}{\n")
			for _, id := range chavesOrdenadas(e.colunas) {
				fmt.Fprintf(&corpo, "\t\t%s: %q,\n", id, e.colunas[id])
			}
			corpo.WriteString("\t},\n")
		}
		corpo.WriteString("}\n")
	}

	formatado, err := format.Source([]byte(corpo.String()))
	if err != nil {
		return err
	}
	caminho := caminhoDoCatalogoDeColunas(projectRoot)
	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		return err
	}
	// Mesmo caso do catálogo de tabelas: o Remove é para vencer somente-leitura, e o
	// WriteFile abaixo é quem reporta.
	_ = os.Remove(caminho)
	return os.WriteFile(caminho, formatado, 0o644)
}

// CatalogNames devolve os nomes do catálogo de tabelas, em ordem estável.
//
// Existe para que ninguém mais escreva um regex próprio sobre o arquivo gerado: a
// forma do catálogo mudou duas vezes, e cada leitor paralelo é um lugar que
// esquece de acompanhar. Aqui a leitura já vem com cache e com o merge dos
// caminhos legados.
func CatalogNames(projectRoot string) []string {
	return chavesOrdenadas(catalogoAcumulado(projectRoot))
}

// ViewNames devolve os nomes do catálogo de views, em ordem estável.
func ViewNames(projectRoot string) []string {
	return chavesOrdenadas(catalogoDeViews(projectRoot))
}

// ViewPhysicalName resolve o nome físico de uma view pelo nome de catálogo.
func ViewPhysicalName(projectRoot, nome string) (string, bool) {
	fisico, tem := catalogoDeViews(projectRoot)[nome]
	return fisico, tem
}

func catalogoDeViews(projectRoot string) map[string]string {
	acumulado := map[string]string{}
	for _, caminho := range append(caminhosLegadosDeViews(projectRoot), caminhoDoCatalogoDeViews(projectRoot)) {
		for chave, valor := range loadViewCatalog(caminho) {
			acumulado[chave] = valor
		}
	}
	return acumulado
}

func chavesOrdenadas(entradas map[string]string) []string {
	chaves := make([]string, 0, len(entradas))
	for chave := range entradas {
		chaves = append(chaves, chave)
	}
	sort.Strings(chaves)
	return chaves
}

func quotedLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

// WriteCoreViewCatalog writes the project-owned autocomplete catalog for views.
func WriteCoreViewCatalog(projectRoot string, views map[string]bool) error {
	entradas := make(map[string]string, len(views))
	for name := range views {
		entradas[ViewIdentifier(name)] = name
	}
	return escreverCatalogo(caminhoDoCatalogoDeViews(projectRoot), "View", "View", "RegisteredView",
		entradas, nil, i18n.T("cat_gen_view_note"))
}

// ViewIdentifier converts a physical view name to the catalog's Go name.
// The conventional vw prefix is kept as its own word for readable autocomplete.
func ViewIdentifier(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(normalized, "vw") && len(normalized) > 2 && normalized[2] != '_' {
		normalized = "vw_" + normalized[2:]
	}
	return exportedIdentifier(normalized)
}

// ExportedIdentifier converte um nome físico no identificador Go exportado que o
// código gerado usa.
//
// É a ÚNICA normalização do gokit, e mora aqui porque este é o pacote mais baixo:
// o gerador da ORM (migraterun) importa migrationgo, nunca o contrário.
//
// Havia duas antes, com comportamentos diferentes: `MIL_PST` virava `MilPst` num
// lado e `MILPST` no outro, e `valor total` virava `ValorTotal` num e
// `Valor total` — identificador inválido — no outro. Enquanto o catálogo só tinha
// tabelas isso não machucava, porque o validador de nome físico barra os casos
// divergentes. Com coluna no catálogo, o mesmo nome geraria dois identificadores
// no MESMO pacote, e nenhum compilador acusaria.
//
// A regra é a mais tolerante das duas: separa por _, - e espaço, e normaliza a
// caixa de cada parte.
func ExportedIdentifier(value string) string {
	partes := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i := range partes {
		if partes[i] != "" {
			partes[i] = strings.ToUpper(partes[i][:1]) + partes[i][1:]
		}
	}
	return strings.Join(partes, "")
}

// UnexportedIdentifier é a mesma regra com a inicial minúscula, para o nome dos
// tipos internos do código gerado (usersEntity, usersColumnSet).
func UnexportedIdentifier(value string) string {
	exportado := ExportedIdentifier(value)
	if exportado == "" {
		return ""
	}
	return strings.ToLower(exportado[:1]) + exportado[1:]
}

func exportedIdentifier(value string) string { return ExportedIdentifier(value) }

func identName(e ast.Expr) string {
	if i, ok := e.(*ast.Ident); ok {
		return i.Name
	}
	return ""
}
