package migrationgo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PhelipeViana/gokit/internal/astparser"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/migration/acao"
)

var tableEntry = regexp.MustCompile(`(?m)^\s*(?:var\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::|=)\s*migrate\.Table\("([^"]+)"\),?`)
// O `(?::|=)` e a vírgula opcional fazem o padrão casar tanto com a declaração
// solta (`var X = migrate.RegisteredView("v")`, do pacote antigo) quanto com o
// campo de struct (`X: migrate.RegisteredView("v"),`, do agrupador core.View). O
// padrão de tabelas já era assim; alinhar os dois evita que a view fique sem
// catálogo depois da mudança de forma.
var viewEntry = regexp.MustCompile(`(?m)^\s*(?:var\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::|=)\s*migrate\.(?:RegisteredView|View)\("([^"]+)"\),?`)
var migrationIDEntry = regexp.MustCompile(`^(\d{4}_\d{2}_\d{2}_\d{6})`)

// seedTimeLayout espelha migrate.SeedTimeLayout. O parser não importa o pacote
// público para evitar ciclo, então o formato é repetido aqui.
const seedTimeLayout = "2006-01-02 15:04:05"

// ParseFile avalia o arquivo Go de migration usando AST e retorna suas operações declarativas.
func ParseFile(path string) ([]acao.Operacao, error) {
	// loadCatalog devolve o mapa do cache compartilhado, então trabalhamos
	// sobre uma cópia antes de mesclar os catálogos.
	catalog := map[string]string{}
	for name, alias := range loadCatalog(filepath.Join(filepath.Dir(path), "dsl.gen.go")) {
		catalog[name] = alias
	}
	views := map[string]string{}
	if root := projectRoot(filepath.Dir(path)); root != "" {
		// catalogoAcumulado já mescla os caminhos legados com o atual, na ordem em que
		// o atual tem a palavra final.
		for name, alias := range catalogoAcumulado(root) {
			catalog[name] = alias
		}
		for _, caminho := range append(caminhosLegadosDeViews(root), caminhoDoCatalogoDeViews(root)) {
			for name, view := range loadViewCatalog(caminho) {
				views[name] = view
			}
		}
	}

	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, 0)
	if err != nil {
		return nil, err
	}

	var operations []acao.Operacao
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			// Seed não mora mais dentro da migration: uma migration aplicada é
			// imutável, então corrigir o dado exigiria mexer no passado.
			if function.Name != nil && strings.HasPrefix(function.Name.Name, "Seeder") {
				return nil, i18n.Errf("mgp_seeder_wrong_place", "database/seeds")
			}
			found, err := evalDefinitionFunction(set, function, catalog, views, path)
			if err != nil {
				return nil, err
			}
			operations = append(operations, found...)
			continue
		}
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.VAR {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, expression := range value.Values {
				literal, ok := expression.(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, element := range literal.Elts {
					operation, err := evalOperation(element, catalog, views, path)
					if err != nil {
						return nil, positionError(set, element, err)
					}
					operations = append(operations, acao.Expandir(operation)...)
				}
			}
		}
	}

	if len(operations) == 0 {
		return nil, i18n.Errf("mgp_no_operation")
	}
	for index := range operations {
		if operations[index].Kind == string(acao.CreateTable) && operations[index].AliasName == "" {
			if strings.HasSuffix(filepath.Base(path), "_migration.go") {
				operations[index].AliasName = tableIdentifier(operations[index].Table)
			} else {
				return nil, i18n.Errf("mgp_createtable_needs_alias")
			}
		}
	}
	return operations, nil
}

// evalSeederFunction lê `func Seeder() migrate.Rows { return migrate.Rows{...} }`.
func evalSeederFunction(set *token.FileSet, function *ast.FuncDecl) ([]acao.Linha, error) {
	fail := func(node ast.Node, format string, arguments ...any) ([]acao.Linha, error) {
		return nil, positionError(set, node, fmt.Errorf(format, arguments...))
	}
	if function.Body == nil || len(function.Body.List) != 1 {
		return fail(function, "%s", i18n.T("mgp_seeder_body"))
	}
	statement, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(statement.Results) != 1 {
		return fail(function, "%s", i18n.T("mgp_seeder_one_list"))
	}
	literal, ok := statement.Results[0].(*ast.CompositeLit)
	if !ok {
		return fail(statement, "%s", i18n.T("mgp_seeder_literal"))
	}

	rows := make([]acao.Linha, 0, len(literal.Elts))
	for index, element := range literal.Elts {
		rowLiteral, ok := element.(*ast.CompositeLit)
		if !ok {
			return fail(element, i18n.T("mgp_row_literal"), index+1)
		}
		row := acao.Linha{}
		for _, field := range rowLiteral.Elts {
			pair, ok := field.(*ast.KeyValueExpr)
			if !ok {
				return fail(field, i18n.T("mgp_row_format"), index+1)
			}
			column, err := astparser.StringLiteral(pair.Key)
			if err != nil {
				return fail(pair.Key, i18n.T("mgp_row_bad_column"), index+1)
			}
			if _, repeated := row[column]; repeated {
				return fail(pair.Key, i18n.T("mgp_row_dup_column"), index+1, column)
			}
			value, err := seedLiteral(pair.Value)
			if err != nil {
				return fail(pair.Value, i18n.T("mgp_row_column_detail"), index+1, column, err)
			}
			row[column] = value
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// seedLiteral avalia um valor de seed. Só literais são aceitos: o arquivo é
// lido por AST e nunca executado, então não há como resolver expressões.
func seedLiteral(expression ast.Expr) (any, error) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		switch value.Kind {
		case token.STRING:
			return strconv.Unquote(value.Value)
		case token.INT:
			return strconv.ParseInt(value.Value, 0, 64)
		case token.FLOAT:
			return strconv.ParseFloat(value.Value, 64)
		}
	case *ast.Ident:
		switch value.Name {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil":
			return nil, nil
		}
	case *ast.CallExpr:
		// Única chamada aceita num valor de seed: migrate.Time("...").
		selector, ok := value.Fun.(*ast.SelectorExpr)
		if !ok || astparser.IdentName(selector.X) != "migrate" || selector.Sel.Name != "Time" {
			return nil, i18n.Errf("mgp_only_time_call")
		}
		if len(value.Args) != 1 {
			return nil, i18n.Errf("mgp_time_one_text")
		}
		text, err := astparser.StringLiteral(value.Args[0])
		if err != nil {
			return nil, i18n.Errf("mgp_time_quoted")
		}
		parsed, err := time.Parse(seedTimeLayout, text)
		if err != nil {
			return nil, i18n.Errf("mgp_time_format", text, seedTimeLayout)
		}
		return parsed, nil
	case *ast.UnaryExpr:
		if value.Op == token.SUB {
			inner, err := seedLiteral(value.X)
			if err != nil {
				return nil, err
			}
			switch number := inner.(type) {
			case int64:
				return -number, nil
			case float64:
				return -number, nil
			}
		}
	}
	return nil, i18n.Errf("mgp_bad_value")
}

// seedTimeLayouts são os formatos aceitos num valor de data/hora. O primeiro é
// o canônico; os demais cobrem o que costuma sair de um dump.
var seedTimeLayouts = []string{
	seedTimeLayout,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	time.RFC3339,
	"2006-01-02",
}

// CoerceValue ajusta um valor ao tipo declarado da coluna. As factories usam
// a mesma regra dos seeds: quem gera o dado informa o valor, o motor resolve
// como cada driver quer recebê-lo.
func CoerceValue(value any, kind string) (any, error) {
	return coerceSeedValue(value, kind)
}

// coerceSeedRows ajusta cada valor ao tipo declarado da coluna no CreateTable.
//
// A conversão vive aqui e não no arquivo de seed porque é conhecimento do
// motor, não do autor: só o Oracle exige data com bind tipado (uma string cai
// no NLS_TIMESTAMP_FORMAT da sessão e estoura ORA-01843), e só o driver do
// Postgres recusa número em coluna de texto. Quem escreve o seed informa o
// valor; o gokit resolve o resto.
func coerceSeedRows(rows []acao.Linha, columns []acao.ColunaDefinicao) error {
	kinds := make(map[string]string, len(columns))
	for _, column := range columns {
		kinds[strings.ToLower(column.Name)] = column.Type
	}
	for index, row := range rows {
		for name, value := range row {
			coerced, err := coerceSeedValue(value, kinds[strings.ToLower(name)])
			if err != nil {
				return i18n.Errf("mgp_row_column_wrap", index+1, name, err)
			}
			row[name] = coerced
		}
	}
	return nil
}

func coerceSeedValue(value any, kind string) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch kind {
	case "date", "datetime", "timestamp":
		text, ok := value.(string)
		if !ok {
			return value, nil // já é time.Time (migrate.Time) ou outro tipo
		}
		for _, layout := range seedTimeLayouts {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed, nil
			}
		}
		return nil, i18n.Errf("mgp_bad_date", text, seedTimeLayout)

	case "string", "char", "text":
		// Número em coluna de texto: o pgx recusa, os outros três convertem.
		// int e int64 aparecem os dois: o seed literal produz int64, as funções
		// Fake* das factories produzem int.
		switch typed := value.(type) {
		case int:
			return strconv.Itoa(typed), nil
		case int64:
			return strconv.FormatInt(typed, 10), nil
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64), nil
		}

	case "int", "integer", "decimal":
		// Número entre aspas em coluna numérica: mesma história ao contrário.
		if text, ok := value.(string); ok {
			if number, err := strconv.ParseInt(text, 10, 64); err == nil {
				return number, nil
			}
			if number, err := strconv.ParseFloat(text, 64); err == nil {
				return number, nil
			}
		}

	case "boolean":
		// 0/1 em coluna booleana: MySQL, Oracle e SQL Server aceitam o número, mas
		// o pgx recusa ("unable to encode 0 into binary format for bool"). Como as
		// factories produzem o booleano com FakeIntIndex(index, 0, 1), a conversão
		// é conhecimento do motor — não do autor da factory.
		switch typed := value.(type) {
		case bool:
			return typed, nil
		case int:
			return typed != 0, nil
		case int64:
			return typed != 0, nil
		case float64:
			return typed != 0, nil
		case string:
			if b, err := strconv.ParseBool(typed); err == nil {
				return b, nil
			}
			return nil, i18n.Errf("mgp_bad_bool", typed)
		}
	}
	return value, nil
}

// ParseSeedFile lê um arquivo de seed avulso — database/seeds/<tabela>/<ts>_seeder.go.
// A tabela vem da pasta; o arquivo só carrega as linhas.
func ParseSeedFile(path string, columns []acao.ColunaDefinicao) ([]acao.Linha, error) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, 0)
	if err != nil {
		return nil, err
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name == nil || !strings.HasPrefix(function.Name.Name, "Seeder") {
			continue
		}
		rows, err := evalSeederFunction(set, function)
		if err != nil {
			return nil, err
		}
		if err := coerceSeedRows(rows, columns); err != nil {
			return nil, err
		}
		return rows, nil
	}
	return nil, i18n.Errf("mgp_no_seeder_func")
}

func loadViewCatalog(path string) map[string]string {
	return loadCatalogWith(path, viewEntry)
}

func projectRoot(path string) string {
	current, _ := filepath.Abs(path)
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		if _, err := os.Stat(filepath.Join(current, "internal", "gokit", "gokit.json")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func evalDefinitionFunction(set *token.FileSet, function *ast.FuncDecl, catalog, views map[string]string, path string) ([]acao.Operacao, error) {
	if function.Body == nil || function.Name == nil || len(function.Body.List) != 1 {
		return nil, nil
	}
	statement, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(statement.Results) != 1 {
		return nil, nil
	}
	call, ok := statement.Results[0].(*ast.CallExpr)
	if !ok {
		return nil, nil
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if ok && astparser.IdentName(selector.X) == "migrate" && selector.Sel.Name == "Define" {
		if len(call.Args) == 0 {
			return nil, positionError(set, call, i18n.Errf("mgp_define_needs_action"))
		}
		// Define é variádico: as ações são aplicadas na ordem declarada e
		// compartilham o mesmo registro de histórico.
		operations := make([]acao.Operacao, 0, len(call.Args))
		for _, argument := range call.Args {
			operation, err := evalOperation(argument, catalog, views, path)
			if err != nil {
				return nil, positionError(set, argument, err)
			}
			operations = append(operations, acao.Expandir(operation)...)
		}
		return operations, nil
	}
	operation, err := evalOperation(statement.Results[0], catalog, views, path)
	if err != nil {
		return nil, positionError(set, statement.Results[0], err)
	}
	return acao.Expandir(operation), nil
}

// catalogCache evita reler e re-parsear os mesmos dsl.gen.go uma vez por
// migration: em um corpus de centenas de arquivos era o mesmo catálogo lido
// centenas de vezes. A chave inclui o mtime, então uma regravação do catálogo
// durante a execução invalida a entrada.
var catalogCache sync.Map

type catalogEntry struct {
	modTime time.Time
	size    int64
	values  map[string]string
}

func loadCatalog(path string) map[string]string {
	return loadCatalogWith(path, tableEntry)
}

func loadCatalogWith(path string, pattern *regexp.Regexp) map[string]string {
	info, statErr := os.Stat(path)
	cacheKey := path + "|" + pattern.String()
	if statErr == nil {
		if cached, found := catalogCache.Load(cacheKey); found {
			entry := cached.(catalogEntry)
			if entry.modTime.Equal(info.ModTime()) && entry.size == info.Size() {
				return entry.values
			}
		}
	}
	result := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
		result[match[1]] = match[2]
	}
	if statErr == nil {
		catalogCache.Store(cacheKey, catalogEntry{modTime: info.ModTime(), size: info.Size(), values: result})
	}
	return result
}

func tableIdentifier(name string) string {
	parts := strings.Split(name, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}

func evalOperation(expression ast.Expr, catalog, views map[string]string, path string) (acao.Operacao, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return acao.Operacao{}, i18n.Errf("mgp_expect_method")
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if ok && selector.Sel.Name == "Alias" {
		if len(call.Args) != 1 {
			return acao.Operacao{}, i18n.Errf("mgp_alias_one_arg")
		}
		operation, err := evalOperation(selector.X, catalog, views, path)
		if err != nil {
			return acao.Operacao{}, err
		}
		if operation.Kind != string(acao.CreateTable) {
			return acao.Operacao{}, i18n.Errf("mgp_alias_only_create")
		}
		operation.AliasName, err = astparser.StringLiteral(call.Args[0])
		if err == nil {
			err = acao.Validar(operation)
		}
		return operation, err
	}
	if !ok || astparser.IdentName(selector.X) != "migrate" {
		return evalLegacyOperation(expression)
	}
	method := selector.Sel.Name
	if method == "TODO" {
		if err := expectArgs(call, 0); err != nil {
			return acao.Operacao{}, err
		}
		return acao.Operacao{Kind: string(acao.Todo)}, nil
	}
	if method == "CreateTable" {
		if len(call.Args) < 2 {
			return acao.Operacao{}, i18n.Errf("mgp_createtable_args")
		}
		table, err := astparser.StringLiteral(call.Args[0])
		if err != nil {
			return acao.Operacao{}, err
		}
		return columnOperation(acao.CreateTable, table, call.Args[1:])
	}
	if method == "CreateView" || method == "AlterView" || method == "DropView" {
		if err := expectArgs(call, 1); err != nil {
			return acao.Operacao{}, err
		}
		name, err := viewReference(call.Args[0], views)
		if err != nil {
			return acao.Operacao{}, err
		}
		kind := map[string]acao.Tipo{"CreateView": acao.CreateView, "AlterView": acao.AlterView, "DropView": acao.DropView}[method]
		operation := acao.Operacao{Kind: string(kind), Name: name}
		if kind != acao.DropView {
			operation.ViewSQL, err = loadVersionedViewSQL(path, name)
			if err != nil {
				return acao.Operacao{}, err
			}
		}
		return operation, nil
	}
	if method == "CreateSequence" || method == "DropSequence" {
		if err := expectArgs(call, 1); err != nil {
			return acao.Operacao{}, err
		}
		name, err := astparser.StringLiteral(call.Args[0])
		if err != nil {
			return acao.Operacao{}, err
		}
		kind := map[string]acao.Tipo{"DropView": acao.DropView, "CreateSequence": acao.CreateSequence, "DropSequence": acao.DropSequence}[method]
		return acao.Operacao{Kind: string(kind), Name: name}, nil
	}
	if method == "SQL" {
		if err := expectArgs(call, 2); err != nil {
			return acao.Operacao{}, err
		}
		dialect, err := astparser.StringLiteral(call.Args[0])
		if err != nil {
			return acao.Operacao{}, err
		}
		statement, err := astparser.StringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		return acao.Operacao{Kind: string(acao.RawSQL), Dialect: dialect, SQL: statement}, nil
	}
	if len(call.Args) == 0 {
		return acao.Operacao{}, i18n.Errf("mgp_needs_alias_ref", method)
	}
	table, err := tableReference(call.Args[0], catalog)
	if err != nil {
		return acao.Operacao{}, err
	}
	switch method {
	case "DropTable":
		return acao.Nova(acao.DropTable, table), nil
	case "AddColumn":
		return columnOperation(acao.AddColumn, table, call.Args[1:])
	case "AlterColumn":
		return columnOperation(acao.AlterColumn, table, call.Args[1:])
	case "DropColumn":
		return columnOperation(acao.DropColumn, table, call.Args[1:])
	case "AddForeignKey":
		return columnOperation(acao.AddForeignKey, table, call.Args[1:])
	case "DropForeignKey":
		return columnOperation(acao.DropForeignKey, table, call.Args[1:])
	case "RenameTable":
		if len(call.Args) != 2 {
			return acao.Operacao{}, i18n.Errf("mgp_renametable_args")
		}
		name, err := astparser.StringLiteral(call.Args[1])
		return acao.Operacao{Kind: string(acao.RenameTable), Table: table, NewName: name}, err
	case "RenameColumn":
		if len(call.Args) != 3 {
			return acao.Operacao{}, i18n.Errf("mgp_renamecolumn_args")
		}
		oldName, err := astparser.StringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		newName, err := astparser.StringLiteral(call.Args[2])
		return acao.Operacao{Kind: string(acao.RenameColumn), Table: table, Column: &acao.ColunaDefinicao{Name: oldName}, NewName: newName}, err
	case "AddPrimaryKey", "AddUnique":
		if len(call.Args) < 3 {
			return acao.Operacao{}, i18n.Errf("mgp_needs_alias_name_columns", method)
		}
		name, err := astparser.StringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		columns, err := stringArguments(call.Args[2:])
		kind := acao.AddPrimaryKey
		if method == "AddUnique" {
			kind = acao.AddUnique
		}
		return acao.Operacao{Kind: string(kind), Table: table, Name: name, IndexColumns: columns}, err
	case "AddCompositeForeignKey":
		if len(call.Args) < 4 {
			return acao.Operacao{}, i18n.Errf("mgp_composite_fk_args")
		}
		name, err := astparser.StringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		referenceTable, err := astparser.StringLiteral(call.Args[2])
		if err != nil {
			return acao.Operacao{}, err
		}
		mappings, err := stringArguments(call.Args[3:])
		if err != nil {
			return acao.Operacao{}, err
		}
		foreignKey := &acao.ForeignKey{ConstraintName: name, ReferenceTable: referenceTable}
		for _, mapping := range mappings {
			parts := strings.SplitN(mapping, ":", 2)
			if len(parts) != 2 {
				return acao.Operacao{}, i18n.Errf("mgp_mapping_format", mapping)
			}
			foreignKey.Columns = append(foreignKey.Columns, parts[0])
			foreignKey.ReferenceColumns = append(foreignKey.ReferenceColumns, parts[1])
		}
		return acao.Operacao{Kind: string(acao.AddForeignKey), Table: table, ForeignKey: foreignKey}, nil
	case "AddCheck":
		if len(call.Args) != 3 {
			return acao.Operacao{}, i18n.Errf("mgp_addcheck_args")
		}
		name, err := astparser.StringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		expression, err := astparser.StringLiteral(call.Args[2])
		return acao.Operacao{Kind: string(acao.AddCheck), Table: table, Name: name, SQL: expression}, err
	case "DropConstraint":
		if len(call.Args) != 2 {
			return acao.Operacao{}, i18n.Errf("mgp_dropconstraint_args")
		}
		name, err := astparser.StringLiteral(call.Args[1])
		return acao.Operacao{Kind: string(acao.DropConstraint), Table: table, Name: name}, err
	case "CreateIndex", "CreateUniqueIndex":
		if len(call.Args) < 3 {
			return acao.Operacao{}, i18n.Errf("mgp_needs_alias_name_columns", method)
		}
		name, err := astparser.StringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		columns := []string{}
		for _, arg := range call.Args[2:] {
			value, err := astparser.StringLiteral(arg)
			if err != nil {
				return acao.Operacao{}, err
			}
			columns = append(columns, value)
		}
		return acao.Operacao{Kind: string(acao.CreateIndex), Table: table, Name: name, IndexColumns: columns, Unique: method == "CreateUniqueIndex"}, nil
	case "DropIndex":
		if len(call.Args) != 2 {
			return acao.Operacao{}, i18n.Errf("mgp_dropindex_args")
		}
		name, err := astparser.StringLiteral(call.Args[1])
		return acao.Operacao{Kind: string(acao.DropIndex), Table: table, Name: name}, err
	default:
		return acao.Operacao{}, i18n.Errf("mgp_method_unsupported", method)
	}
}

func columnOperation(kind acao.Tipo, table string, expressions []ast.Expr) (acao.Operacao, error) {
	items := make([]acao.Item, 0, len(expressions))
	for _, expression := range expressions {
		column, err := evalColumn(expression)
		if err != nil {
			return acao.Operacao{}, err
		}
		items = append(items, column)
	}
	op := acao.Nova(kind, table, items...)
	if kind != acao.CreateTable {
		for _, expanded := range acao.Expandir(op) {
			if err := acao.Validar(expanded); err != nil {
				return acao.Operacao{}, err
			}
		}
	}
	return op, nil
}

// referenciaDeCatalogo aceita as duas formas de citar uma tabela ou view:
//
//	alias.Users        // pacote dedicado (forma antiga, ainda válida)
//	core.Table.Users   // pacote core unificado (forma nova)
//
// A segunda é um seletor ANINHADO — core.Table.Users é SelectorExpr{X:
// SelectorExpr{core, Table}, Sel: Users} —, então validar só o IdentName de
// selector.X devolve vazio e recusaria a referência. Reconhecer as duas é o que
// permite o corpus antigo continuar válido enquanto a forma nova entra: sem isso
// haveria um dia de virada em que nenhuma migration compila.
//
// grupo é o nome do agrupador na forma nova ("Table" ou "View"); pacotes são os
// identificadores aceitos na forma antiga.
func referenciaDeCatalogo(expression ast.Expr, grupo string, pacotes []string) (string, bool) {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	// Forma nova: o meio do caminho é o agrupador, e o que vem antes dele é o
	// pacote — que pode receber qualquer apelido no import, então não é validado.
	if interno, aninhado := selector.X.(*ast.SelectorExpr); aninhado {
		if interno.Sel.Name == grupo && astparser.IdentName(interno.X) != "" {
			return selector.Sel.Name, true
		}
		return "", false
	}
	// Forma antiga: o pacote dedicado, citado pelo nome.
	nome := astparser.IdentName(selector.X)
	for _, aceito := range pacotes {
		if nome == aceito {
			return selector.Sel.Name, true
		}
	}
	return "", false
}

func tableReference(expression ast.Expr, catalog map[string]string) (string, error) {
	referencia, ok := referenciaDeCatalogo(expression, "Table", []string{"alias", "table"})
	if !ok {
		return "", i18n.Errf("mgp_use_alias_ref")
	}
	name, ok := catalog[referencia]
	if !ok {
		return "", i18n.Errf("mgp_alias_not_in_catalog", referencia)
	}
	return name, nil
}

func viewReference(expression ast.Expr, catalog map[string]string) (string, error) {
	referencia, ok := referenciaDeCatalogo(expression, "View", []string{"view"})
	if !ok {
		return "", i18n.Errf("mgp_use_view_ref")
	}
	name, ok := catalog[referencia]
	if !ok {
		return "", i18n.Errf("mgp_view_not_in_catalog", referencia)
	}
	return name, nil
}

func loadVersionedViewSQL(migrationPath, viewName string) (map[string]string, error) {
	match := migrationIDEntry.FindStringSubmatch(filepath.Base(migrationPath))
	if len(match) == 0 {
		return nil, i18n.Errf("mgp_view_needs_timestamp")
	}
	id := match[1]
	root := projectRoot(filepath.Dir(migrationPath))
	for current := filepath.Dir(migrationPath); current != filepath.Dir(current); current = filepath.Dir(current) {
		folder := filepath.Join(current, "views", viewName, id)
		if info, err := os.Stat(folder); err == nil && info.IsDir() {
			return readViewSQLFolder(folder)
		}
		if root != "" && samePath(current, root) {
			break
		}
	}
	return nil, i18n.Errf("mgp_view_sql_missing", viewName, id)
}

func readViewSQLFolder(folder string) (map[string]string, error) {
	allowed := map[string]bool{"common": true, "oracle": true, "postgres": true, "mysql": true, "sqlserver": true}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			continue
		}
		dialect := strings.TrimSuffix(strings.ToLower(entry.Name()), ".sql")
		if !allowed[dialect] {
			return nil, i18n.Errf("mgp_view_file_invalid", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(folder, entry.Name()))
		if err != nil {
			return nil, err
		}
		query := strings.TrimSpace(strings.TrimSuffix(string(data), ";"))
		if query == "" {
			return nil, i18n.Errf("mgp_file_empty", filepath.Join(folder, entry.Name()))
		}
		result[dialect] = query
	}
	// common.sql é opcional: o executor já escolhe <dialeto>.sql quando existe e
	// só recorre a common.sql como fallback. Exigi-lo aqui impedia views
	// escritas para um único banco.
	if len(result) == 0 {
		return nil, i18n.Errf("mgp_no_sql_file", folder)
	}
	return result, nil
}

func samePath(left, right string) bool {
	leftAbs, _ := filepath.Abs(left)
	rightAbs, _ := filepath.Abs(right)
	return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}

func evalLegacyOperation(expression ast.Expr) (acao.Operacao, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok || astparser.IdentName(call.Fun) != "nova" || len(call.Args) < 2 {
		return acao.Operacao{}, i18n.Errf("mgp_expect_method")
	}
	selector, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok || astparser.IdentName(selector.X) != "acao" {
		return acao.Operacao{}, i18n.Errf("mgp_legacy_action")
	}
	table, err := astparser.StringLiteral(call.Args[1])
	if err != nil {
		return acao.Operacao{}, err
	}
	kind := map[string]acao.Tipo{"CreateTable": acao.CreateTable, "DropTable": acao.DropTable, "AddColumn": acao.AddColumn, "AlterColumn": acao.AlterColumn, "DropColumn": acao.DropColumn, "AddForeignKey": acao.AddForeignKey, "DropForeignKey": acao.DropForeignKey}[selector.Sel.Name]
	return columnOperation(kind, table, call.Args[2:])
}

func evalColumn(expression ast.Expr) (acao.Coluna, error) {
	chain, err := astparser.EvaluateCallChain(expression)
	if err != nil {
		return acao.Coluna{}, err
	}

	if chain.RootFunc != "col" && chain.RootFunc != "coluna" && chain.RootFunc != "Col" {
		return acao.Coluna{}, i18n.Errf("mgp_expect_col")
	}

	if len(chain.RootArgs) != 1 {
		return acao.Coluna{}, i18n.Errf("mgp_col_needs_name")
	}

	colName, ok := chain.RootArgs[0].(string)
	if !ok {
		return acao.Coluna{}, i18n.Errf("mgp_col_bad_name")
	}

	column := acao.NovaColuna(colName)
	for _, call := range chain.Calls {
		switch call.Method {
		case "Integer":
			column = column.Integer()
		case "BigInteger":
			column = column.BigInteger()
		case "Int":
			column = column.Int()
		case "Varchar":
			if len(call.Args) != 1 {
				return acao.Coluna{}, i18n.Errf("mgp_varchar_size")
			}
			size, ok := call.Args[0].(int)
			if !ok {
				return acao.Coluna{}, i18n.Errf("mgp_varchar_size_numeric")
			}
			column = column.Varchar(size)
		case "Char":
			if len(call.Args) != 1 {
				return acao.Coluna{}, i18n.Errf("mgp_char_size")
			}
			size, ok := call.Args[0].(int)
			if !ok {
				return acao.Coluna{}, i18n.Errf("mgp_char_size_numeric")
			}
			column = column.Char(size)
		case "Text":
			column = column.Text()
		case "Boolean":
			column = column.Boolean()
		case "Decimal":
			if len(call.Args) != 2 {
				return acao.Coluna{}, i18n.Errf("mgp_decimal_args")
			}
			p, ok1 := call.Args[0].(int)
			s, ok2 := call.Args[1].(int)
			if !ok1 || !ok2 {
				return acao.Coluna{}, i18n.Errf("mgp_decimal_numeric")
			}
			column = column.Decimal(p, s)
		case "Date":
			column = column.Date()
		case "DateTime":
			column = column.DateTime()
		case "Timestamp":
			column = column.Timestamp()
		case "Binary":
			column = column.Binary()
		case "PrimaryKey":
			column = column.PrimaryKey()
		case "AutoIncrement":
			column = column.AutoIncrement()
		case "Nullable":
			column = column.Nullable()
		case "NotNull":
			column = column.NotNull()
		case "Unique":
			column = column.Unique()
		case "Index":
			column = column.Index()
		case "Default":
			if len(call.Args) != 1 {
				return acao.Coluna{}, i18n.Errf("mgp_default_value")
			}
			val, ok := call.Args[0].(string)
			if !ok {
				return acao.Coluna{}, i18n.Errf("mgp_default_text")
			}
			column = column.Default(val)
		case "DefaultExpr":
			if len(call.Args) != 1 {
				return acao.Coluna{}, i18n.Errf("mgp_defaultexpr_value")
			}
			val, ok := call.Args[0].(string)
			if !ok {
				return acao.Coluna{}, i18n.Errf("mgp_defaultexpr_text")
			}
			column = column.DefaultExpr(val)
		case "References":
			if len(call.Args) != 2 {
				return acao.Coluna{}, i18n.Errf("mgp_references_args")
			}
			tbl, ok1 := call.Args[0].(string)
			cl, ok2 := call.Args[1].(string)
			if !ok1 || !ok2 {
				return acao.Coluna{}, i18n.Errf("mgp_references_text")
			}
			column = column.References(tbl, cl)
		case "Constraint":
			if len(call.Args) != 1 {
				return acao.Coluna{}, i18n.Errf("mgp_constraint_name")
			}
			name, ok := call.Args[0].(string)
			if !ok {
				return acao.Coluna{}, i18n.Errf("mgp_constraint_name_text")
			}
			column = column.Constraint(name)
		case "OnDeleteCascade":
			column = column.OnDeleteCascade()
		default:
			return acao.Coluna{}, i18n.Errf("mgp_col_method_unsupported", call.Method)
		}
	}
	return column, nil
}

func stringArguments(expressions []ast.Expr) ([]string, error) {
	values := make([]string, 0, len(expressions))
	for _, expression := range expressions {
		value, err := astparser.StringLiteral(expression)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func expectArgs(c *ast.CallExpr, n int) error {
	if len(c.Args) != n {
		return i18n.Errf("mgp_needs_n_args", callName(c), n)
	}
	return nil
}

func callName(c *ast.CallExpr) string {
	if s, ok := c.Fun.(*ast.SelectorExpr); ok {
		return s.Sel.Name
	}
	return astparser.IdentName(c.Fun)
}

func positionError(set *token.FileSet, node ast.Node, err error) error {
	p := set.Position(node.Pos())
	return fmt.Errorf("%s:%d: %w", p.Filename, p.Line, err)
}
