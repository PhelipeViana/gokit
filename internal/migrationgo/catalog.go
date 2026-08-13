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
func colunasPorTabela(migrationsFolder string, tabelas map[string]string) map[string]map[string]bool {
	acumulado := map[string]map[string]bool{}

	// operacoesComTabela são as que trazem tabela no 1º argumento e coluna dentro.
	operacoesComTabela := map[string]bool{
		"CreateTable": true, "AddColumn": true, "AlterColumn": true, "DropColumn": true,
	}

	_ = filepath.WalkDir(migrationsFolder, func(caminho string, entrada fs.DirEntry, err error) error {
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
	return acumulado
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

// RefreshCatalog rebuilds the table catalog in GoKit's Core area from the tables already known
// by the catalog and from migrate.CreateTable calls written in migration files.
func RefreshCatalog(projectRoot string, migrationsFolder string) error {
	catalogPath := caminhoDoCatalogo(projectRoot)
	tables := catalogoAcumulado(projectRoot)

	known := make(map[string]bool, len(tables))
	for _, name := range tables {
		known[name] = true
	}

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
			if name, unquoteErr := strconv.Unquote(literal.Value); unquoteErr == nil && name != "" {
				known[name] = true
			}
			return true
		})
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	names := make([]string, 0, len(known))
	for name := range known {
		names = append(names, name)
	}
	sort.Strings(names)

	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		return err
	}
	return writeCatalog(catalogPath, names)
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
func grupoDeCatalogo(nome, construtor string, entradas map[string]string, fisico map[string]string) string {
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
		fmt.Fprintf(&source, "\t%s migrate.%s\n", chave, construtor)
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
func escreverCatalogo(path, nome, construtor string, entradas, fisico map[string]string, nota string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var source strings.Builder
	source.WriteString("// Code generated by GoKit. DO NOT EDIT.\npackage core\n\n")
	if len(entradas) > 0 {
		source.WriteString("import migrate \"github.com/PhelipeViana/gokit/migration\"\n\n")
	}
	source.WriteString(nota)
	source.WriteString(grupoDeCatalogo(nome, construtor, entradas, fisico))

	formatted, err := format.Source([]byte(source.String()))
	if err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.WriteFile(path, formatted, 0o644)
}

func writeCatalog(path string, names []string) error {
	entradas := make(map[string]string, len(names))
	for _, name := range names {
		entradas[exportedIdentifier(name)] = name
	}
	return escreverCatalogo(path, "Table", "Table", entradas, nil, "")
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
	return escreverCatalogo(caminhoDoCatalogo(projectRoot), "Table", "Table",
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
			aliasCall, ok := node.(*ast.CallExpr)
			if !ok || len(aliasCall.Args) != 1 {
				return true
			}
			aliasSelector, ok := aliasCall.Fun.(*ast.SelectorExpr)
			if !ok || aliasSelector.Sel.Name != "Alias" {
				return true
			}
			createCall, ok := aliasSelector.X.(*ast.CallExpr)
			if !ok || len(createCall.Args) == 0 {
				return true
			}
			createSelector, ok := createCall.Fun.(*ast.SelectorExpr)
			if !ok || identName(createSelector.X) != "migrate" || createSelector.Sel.Name != "CreateTable" {
				return true
			}
			physical, physicalOK := quotedLiteral(createCall.Args[0])
			alias, aliasOK := quotedLiteral(aliasCall.Args[0])
			if physicalOK && aliasOK && physical != "" && alias != "" {
				aliases[alias] = physical
			}
			return true
		})
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
	porTabela := colunasPorTabela(migrationsFolder, tabelas)

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
		entradas = append(entradas, entrada{identificador: ExportedIdentifier(tabela), colunas: colunas})
	}

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
	return escreverCatalogo(caminhoDoCatalogoDeViews(projectRoot), "View", "RegisteredView",
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
