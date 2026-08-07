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

	"gokit/internal/astparser"
	"gokit/migration/acao"
)

var tableEntry = regexp.MustCompile(`(?m)^\s*(?:var\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::|=)\s*migrate\.Table\("([^"]+)"\),?`)
var viewEntry = regexp.MustCompile(`(?m)^\s*(?:var\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*migrate\.(?:RegisteredView|View)\("([^"]+)"\)`)
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
		core := loadCatalog(filepath.Join(root, "internal", "gokit", "core", "migration", "alias", "dsl.gen.go"))
		legacyCore := loadCatalog(filepath.Join(root, "internal", "gokit", "core", "migration", "table", "dsl.gen.go"))
		for name, alias := range legacyCore {
			catalog[name] = alias
		}
		for name, alias := range core {
			catalog[name] = alias
		}
		views = loadViewCatalog(filepath.Join(root, "internal", "gokit", "core", "migration", "view", "dsl.gen.go"))
	}

	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, 0)
	if err != nil {
		return nil, err
	}

	var operations []acao.Operacao
	var seedRows []acao.Linha
	var seedFound bool
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			if function.Name != nil && function.Name.Name == "Seeder" {
				rows, err := evalSeederFunction(set, function)
				if err != nil {
					return nil, err
				}
				seedRows, seedFound = rows, true
				continue
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
		return nil, fmt.Errorf("nenhuma operação migrate.* encontrada")
	}
	if seedFound {
		seed, err := bindSeederToTable(operations, seedRows)
		if err != nil {
			return nil, err
		}
		// O seed entra no fim: a tabela precisa existir antes das linhas.
		operations = append(operations, seed)
	}
	for index := range operations {
		if operations[index].Kind == string(acao.CreateTable) && operations[index].AliasName == "" {
			if strings.HasSuffix(filepath.Base(path), "_migration.go") {
				operations[index].AliasName = tableIdentifier(operations[index].Table)
			} else {
				return nil, fmt.Errorf("CreateTable exige .Alias(\"apelido\")")
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
		return fail(function, "Seeder() deve conter apenas `return migrate.Rows{...}`")
	}
	statement, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(statement.Results) != 1 {
		return fail(function, "Seeder() deve retornar exatamente uma lista migrate.Rows")
	}
	literal, ok := statement.Results[0].(*ast.CompositeLit)
	if !ok {
		return fail(statement, "Seeder() deve retornar um literal migrate.Rows{...}")
	}

	rows := make([]acao.Linha, 0, len(literal.Elts))
	for index, element := range literal.Elts {
		rowLiteral, ok := element.(*ast.CompositeLit)
		if !ok {
			return fail(element, "a linha %d deve ser um literal {\"coluna\": valor, ...}", index+1)
		}
		row := acao.Linha{}
		for _, field := range rowLiteral.Elts {
			pair, ok := field.(*ast.KeyValueExpr)
			if !ok {
				return fail(field, "a linha %d deve usar o formato \"coluna\": valor", index+1)
			}
			column, err := astparser.StringLiteral(pair.Key)
			if err != nil {
				return fail(pair.Key, "a linha %d tem um nome de coluna inválido; use \"coluna\": valor", index+1)
			}
			if _, repeated := row[column]; repeated {
				return fail(pair.Key, "a linha %d repete a coluna %q", index+1, column)
			}
			value, err := seedLiteral(pair.Value)
			if err != nil {
				return fail(pair.Value, "linha %d, coluna %q: %v", index+1, column, err)
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
			return nil, fmt.Errorf("só migrate.Time(\"...\") é aceito como chamada em valor de seed")
		}
		if len(value.Args) != 1 {
			return nil, fmt.Errorf("migrate.Time exige exatamente um texto")
		}
		text, err := astparser.StringLiteral(value.Args[0])
		if err != nil {
			return nil, fmt.Errorf("migrate.Time exige um texto entre aspas")
		}
		parsed, err := time.Parse(seedTimeLayout, text)
		if err != nil {
			return nil, fmt.Errorf("migrate.Time(%q) fora do formato %s", text, seedTimeLayout)
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
	return nil, fmt.Errorf("valor inválido; use texto, número, true, false ou nil")
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
				return fmt.Errorf("linha %d, coluna %q: %w", index+1, name, err)
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
		return nil, fmt.Errorf("%q não é uma data válida; use o formato %s", text, seedTimeLayout)

	case "string", "char", "text":
		// Número em coluna de texto: o pgx recusa, os outros três convertem.
		switch typed := value.(type) {
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
	}
	return value, nil
}

// bindSeederToTable amarra o Seeder() ao CreateTable do mesmo arquivo e extrai
// a chave primária declarada, que é o que decide entre inserir e editar.
func bindSeederToTable(operations []acao.Operacao, rows []acao.Linha) (acao.Operacao, error) {
	var target *acao.Operacao
	for index := range operations {
		if operations[index].Kind == string(acao.CreateTable) {
			if target != nil {
				return acao.Operacao{}, fmt.Errorf("Seeder() exige exatamente um CreateTable no arquivo, mas há mais de um")
			}
			target = &operations[index]
		}
	}
	if target == nil {
		return acao.Operacao{}, fmt.Errorf("Seeder() só pode acompanhar um CreateTable no mesmo arquivo")
	}
	if err := coerceSeedRows(rows, target.Columns); err != nil {
		return acao.Operacao{}, err
	}

	var keys []string
	identity := ""
	for _, column := range target.Columns {
		if column.PrimaryKey {
			keys = append(keys, column.Name)
		}
		if column.AutoIncrement {
			identity = column.Name
		}
	}
	return acao.Operacao{
		Kind:           string(acao.SeedRows),
		Table:          target.Table,
		AliasName:      target.AliasName,
		Rows:           rows,
		KeyColumns:     keys,
		IdentityColumn: identity,
	}, nil
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
			return nil, positionError(set, call, fmt.Errorf("migrate.Define exige ao menos uma ação"))
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
		return acao.Operacao{}, fmt.Errorf("esperado migrate.Metodo(...)")
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if ok && selector.Sel.Name == "Alias" {
		if len(call.Args) != 1 {
			return acao.Operacao{}, fmt.Errorf("Alias exige exatamente um apelido")
		}
		operation, err := evalOperation(selector.X, catalog, views, path)
		if err != nil {
			return acao.Operacao{}, err
		}
		if operation.Kind != string(acao.CreateTable) {
			return acao.Operacao{}, fmt.Errorf("Alias só pode ser usado em CreateTable")
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
			return acao.Operacao{}, fmt.Errorf("CreateTable exige nome e colunas")
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
		return acao.Operacao{}, fmt.Errorf("%s exige alias.*", method)
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
			return acao.Operacao{}, fmt.Errorf("RenameTable exige alias.* e novo nome")
		}
		name, err := astparser.StringLiteral(call.Args[1])
		return acao.Operacao{Kind: string(acao.RenameTable), Table: table, NewName: name}, err
	case "RenameColumn":
		if len(call.Args) != 3 {
			return acao.Operacao{}, fmt.Errorf("RenameColumn exige alias.*, nome atual e novo nome")
		}
		oldName, err := astparser.StringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		newName, err := astparser.StringLiteral(call.Args[2])
		return acao.Operacao{Kind: string(acao.RenameColumn), Table: table, Column: &acao.ColunaDefinicao{Name: oldName}, NewName: newName}, err
	case "AddPrimaryKey", "AddUnique":
		if len(call.Args) < 3 {
			return acao.Operacao{}, fmt.Errorf("%s exige alias.*, nome e colunas", method)
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
			return acao.Operacao{}, fmt.Errorf("AddCompositeForeignKey exige alias.*, nome, tabela referenciada e mapeamentos")
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
				return acao.Operacao{}, fmt.Errorf("mapeamento %q deve usar coluna:referência", mapping)
			}
			foreignKey.Columns = append(foreignKey.Columns, parts[0])
			foreignKey.ReferenceColumns = append(foreignKey.ReferenceColumns, parts[1])
		}
		return acao.Operacao{Kind: string(acao.AddForeignKey), Table: table, ForeignKey: foreignKey}, nil
	case "AddCheck":
		if len(call.Args) != 3 {
			return acao.Operacao{}, fmt.Errorf("AddCheck exige alias.*, nome e expressão")
		}
		name, err := astparser.StringLiteral(call.Args[1])
		if err != nil {
			return acao.Operacao{}, err
		}
		expression, err := astparser.StringLiteral(call.Args[2])
		return acao.Operacao{Kind: string(acao.AddCheck), Table: table, Name: name, SQL: expression}, err
	case "DropConstraint":
		if len(call.Args) != 2 {
			return acao.Operacao{}, fmt.Errorf("DropConstraint exige alias.* e nome")
		}
		name, err := astparser.StringLiteral(call.Args[1])
		return acao.Operacao{Kind: string(acao.DropConstraint), Table: table, Name: name}, err
	case "CreateIndex", "CreateUniqueIndex":
		if len(call.Args) < 3 {
			return acao.Operacao{}, fmt.Errorf("%s exige alias.*, nome e colunas", method)
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
			return acao.Operacao{}, fmt.Errorf("DropIndex exige alias.* e nome")
		}
		name, err := astparser.StringLiteral(call.Args[1])
		return acao.Operacao{Kind: string(acao.DropIndex), Table: table, Name: name}, err
	default:
		return acao.Operacao{}, fmt.Errorf("método migrate.%s não suportado", method)
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

func tableReference(expression ast.Expr, catalog map[string]string) (string, error) {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || (astparser.IdentName(selector.X) != "alias" && astparser.IdentName(selector.X) != "table") {
		return "", fmt.Errorf("use uma referência alias.*")
	}
	name, ok := catalog[selector.Sel.Name]
	if !ok {
		return "", fmt.Errorf("referência alias.%s não existe no catálogo", selector.Sel.Name)
	}
	return name, nil
}

func viewReference(expression ast.Expr, catalog map[string]string) (string, error) {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || astparser.IdentName(selector.X) != "view" {
		return "", fmt.Errorf("use exclusivamente uma referência view.*")
	}
	name, ok := catalog[selector.Sel.Name]
	if !ok {
		return "", fmt.Errorf("referência view.%s não existe no catálogo", selector.Sel.Name)
	}
	return name, nil
}

func loadVersionedViewSQL(migrationPath, viewName string) (map[string]string, error) {
	match := migrationIDEntry.FindStringSubmatch(filepath.Base(migrationPath))
	if len(match) == 0 {
		return nil, fmt.Errorf("migration de view precisa iniciar com um ID de timestamp")
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
	return nil, fmt.Errorf("SQL da view %s não encontrado para a migration %s", viewName, id)
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
			return nil, fmt.Errorf("arquivo de view %s inválido; use common.sql, oracle.sql, postgres.sql, mysql.sql ou sqlserver.sql", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(folder, entry.Name()))
		if err != nil {
			return nil, err
		}
		query := strings.TrimSpace(strings.TrimSuffix(string(data), ";"))
		if query == "" {
			return nil, fmt.Errorf("%s está vazio", filepath.Join(folder, entry.Name()))
		}
		result[dialect] = query
	}
	// common.sql é opcional: o executor já escolhe <dialeto>.sql quando existe e
	// só recorre a common.sql como fallback. Exigi-lo aqui impedia views
	// escritas para um único banco.
	if len(result) == 0 {
		return nil, fmt.Errorf("%s não contém nenhum .sql; crie common.sql ou um arquivo por dialeto (oracle.sql, postgres.sql, mysql.sql, sqlserver.sql)", folder)
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
		return acao.Operacao{}, fmt.Errorf("esperado migrate.Metodo(...)")
	}
	selector, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok || astparser.IdentName(selector.X) != "acao" {
		return acao.Operacao{}, fmt.Errorf("ação antiga inválida")
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
		return acao.Coluna{}, fmt.Errorf("esperado col(...) ou método de coluna")
	}

	if len(chain.RootArgs) != 1 {
		return acao.Coluna{}, fmt.Errorf("col exige um nome")
	}

	colName, ok := chain.RootArgs[0].(string)
	if !ok {
		return acao.Coluna{}, fmt.Errorf("nome de coluna inválido")
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
				return acao.Coluna{}, fmt.Errorf("Varchar exige tamanho")
			}
			size, ok := call.Args[0].(int)
			if !ok {
				return acao.Coluna{}, fmt.Errorf("Varchar exige tamanho numérico")
			}
			column = column.Varchar(size)
		case "Char":
			if len(call.Args) != 1 {
				return acao.Coluna{}, fmt.Errorf("Char exige tamanho")
			}
			size, ok := call.Args[0].(int)
			if !ok {
				return acao.Coluna{}, fmt.Errorf("Char exige tamanho numérico")
			}
			column = column.Char(size)
		case "Text":
			column = column.Text()
		case "Boolean":
			column = column.Boolean()
		case "Decimal":
			if len(call.Args) != 2 {
				return acao.Coluna{}, fmt.Errorf("Decimal exige precisão e escala")
			}
			p, ok1 := call.Args[0].(int)
			s, ok2 := call.Args[1].(int)
			if !ok1 || !ok2 {
				return acao.Coluna{}, fmt.Errorf("Decimal exige valores numéricos")
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
				return acao.Coluna{}, fmt.Errorf("Default exige um valor")
			}
			val, ok := call.Args[0].(string)
			if !ok {
				return acao.Coluna{}, fmt.Errorf("Default exige valor texto")
			}
			column = column.Default(val)
		case "DefaultExpr":
			if len(call.Args) != 1 {
				return acao.Coluna{}, fmt.Errorf("DefaultExpr exige um valor")
			}
			val, ok := call.Args[0].(string)
			if !ok {
				return acao.Coluna{}, fmt.Errorf("DefaultExpr exige valor texto")
			}
			column = column.DefaultExpr(val)
		case "References":
			if len(call.Args) != 2 {
				return acao.Coluna{}, fmt.Errorf("References exige tabela e coluna")
			}
			tbl, ok1 := call.Args[0].(string)
			cl, ok2 := call.Args[1].(string)
			if !ok1 || !ok2 {
				return acao.Coluna{}, fmt.Errorf("References exige parâmetros texto")
			}
			column = column.References(tbl, cl)
		case "Constraint":
			if len(call.Args) != 1 {
				return acao.Coluna{}, fmt.Errorf("Constraint exige nome")
			}
			name, ok := call.Args[0].(string)
			if !ok {
				return acao.Coluna{}, fmt.Errorf("Constraint exige nome texto")
			}
			column = column.Constraint(name)
		case "OnDeleteCascade":
			column = column.OnDeleteCascade()
		default:
			return acao.Coluna{}, fmt.Errorf("método de coluna %s não suportado", call.Method)
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
		return fmt.Errorf("%s exige %d argumento(s)", callName(c), n)
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
