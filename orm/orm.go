// Package orm contains GoKit's reusable read-query engine. Applications only
// receive generated mappings that compose these types.
//
// O objetivo é um "dialeto" de autoria único, agnóstico de banco (à la
// Eloquent): a aplicação escreve a pesquisa de forma fluente a partir do model
// gerado e o GoKit compila para o SQL de cada banco. Esta fase cobre apenas os
// métodos de PESQUISA (montagem + compilação); a execução contra o banco e os
// demais dialetos entram em fases seguintes.
package orm

import (
	"database/sql"
	"fmt"
	"strings"
)

type Field struct {
	Name          string
	Entity        string
	Table         string
	Column        string
	DataType      string
	Nullable      bool
	PrimaryKey    bool
	AutoIncrement bool
	Unique        bool
	Reference     string
}
type EntityFields struct {
	Name   string
	Fields []Field
}

// Scanner mapeia as linhas cruas do banco para o tipo de linha da entidade.
// O código gerado fornece um por tabela (scan posicional por nome de coluna).
type Scanner[T any] func(*sql.Rows) ([]T, error)

// Model é o ponto de partida das pesquisas de uma entidade, parametrizado pelo
// tipo da linha retornada (T). O código gerado instancia Model[UsersRow] etc.
type Model[T any] struct {
	Entity EntityFields
	scan   Scanner[T]
}

func NewModel[T any](entity EntityFields, scan Scanner[T]) Model[T] {
	return Model[T]{Entity: entity, scan: scan}
}

type Operator string

const (
	Equal          Operator = "equal"
	NotEqual       Operator = "not_equal"
	GreaterThan    Operator = "greater_than"
	GreaterOrEqual Operator = "greater_or_equal"
	LessThan       Operator = "less_than"
	LessOrEqual    Operator = "less_or_equal"
	Contains       Operator = "contains"
	StartsWith     Operator = "starts_with"
	EndsWith       Operator = "ends_with"
	In             Operator = "in"
	Between        Operator = "between"
	IsNull         Operator = "is_null"
	IsNotNull      Operator = "is_not_null"
)

// Expression é qualquer condição componível de uma pesquisa: um Filter (folha)
// ou um Group (subárvore com parênteses).
type Expression interface {
	expression()
	active() bool
}

// Filter é a condição-folha sobre uma coluna. Active=false faz o compilador
// ignorá-la — é o que permite pesquisas dinâmicas (campo vazio não filtra).
type Filter struct {
	Field    Field
	Operator Operator
	Values   []any
	Active   bool
}

func (Filter) expression()    {}
func (f Filter) active() bool { return f.Active }

// item liga uma expressão à anterior por AND (or=false) ou OR (or=true).
type item struct {
	or   bool
	expr Expression
}

// Group agrupa expressões entre parênteses, preservando o conector interno.
type Group struct{ items []item }

func (Group) expression() {}
func (g Group) active() bool {
	for _, it := range g.items {
		if it.expr != nil && it.expr.active() {
			return true
		}
	}
	return false
}

// And é o combinador que agrupa expressões unidas por AND, entre parênteses:
// (a AND b AND ...). Use dentro de Where para compor a árvore booleana sem
// ambiguidade de precedência: Where(a, Or(b, c)) → a AND (b OR c).
func And(expressions ...Expression) Group {
	g := Group{}
	for _, e := range expressions {
		g.items = append(g.items, item{or: false, expr: e})
	}
	return g
}

// Or é o combinador que agrupa expressões unidas por OR, entre parênteses:
// (a OR b OR ...).
func Or(expressions ...Expression) Group {
	g := Group{}
	for i, e := range expressions {
		g.items = append(g.items, item{or: i > 0, expr: e})
	}
	return g
}

type StringFilterField struct{ Field Field }
type NumberFilterField struct{ Field Field }
type DateFilterField struct{ Field Field }
type BoolFilterField struct{ Field Field }

func StringFilter(f Field) StringFilterField { return StringFilterField{f} }
func NumberFilter(f Field) NumberFilterField { return NumberFilterField{f} }
func DateFilter(f Field) DateFilterField     { return DateFilterField{f} }
func BoolFilter(f Field) BoolFilterField     { return BoolFilterField{f} }

func condition(f Field, op Operator, values ...any) Filter {
	return Filter{Field: f, Operator: op, Values: values, Active: conditionActive(op, values)}
}

func conditionActive(op Operator, values []any) bool {
	if op == IsNull || op == IsNotNull {
		return true
	}
	return len(values) > 0
}

// --- Texto ---
func (f StringFilterField) Igual(v string) Filter     { return condition(f.Field, Equal, v) }
func (f StringFilterField) Diferente(v string) Filter { return condition(f.Field, NotEqual, v) }
func (f StringFilterField) Contem(v string) Filter    { return condition(f.Field, Contains, v) }
func (f StringFilterField) ComecaCom(v string) Filter { return condition(f.Field, StartsWith, v) }
func (f StringFilterField) TerminaCom(v string) Filter {
	return condition(f.Field, EndsWith, v)
}
func (f StringFilterField) Em(vs ...string) Filter {
	return condition(f.Field, In, textsToAny(vs)...)
}
func (f StringFilterField) Nulo() Filter    { return condition(f.Field, IsNull) }
func (f StringFilterField) NaoNulo() Filter { return condition(f.Field, IsNotNull) }

// --- Número ---
func (f NumberFilterField) Igual(v any) Filter      { return condition(f.Field, Equal, v) }
func (f NumberFilterField) Diferente(v any) Filter  { return condition(f.Field, NotEqual, v) }
func (f NumberFilterField) Maior(v any) Filter      { return condition(f.Field, GreaterThan, v) }
func (f NumberFilterField) MaiorIgual(v any) Filter { return condition(f.Field, GreaterOrEqual, v) }
func (f NumberFilterField) Menor(v any) Filter      { return condition(f.Field, LessThan, v) }
func (f NumberFilterField) MenorIgual(v any) Filter { return condition(f.Field, LessOrEqual, v) }
func (f NumberFilterField) Entre(min, max any) Filter {
	return condition(f.Field, Between, min, max)
}
func (f NumberFilterField) Em(vs ...any) Filter { return condition(f.Field, In, vs...) }
func (f NumberFilterField) Nulo() Filter        { return condition(f.Field, IsNull) }
func (f NumberFilterField) NaoNulo() Filter     { return condition(f.Field, IsNotNull) }

// --- Data ---
// Todos os métodos passam o valor por ValorData: time.Time, *time.Time, Valor e
// texto em formato conhecido ("2006-01-02", RFC3339...) chegam ao driver já como
// time.Time. É o tratamento genérico — o autor da pesquisa informa a data no
// formato que tem em mão e o motor resolve.
func (f DateFilterField) Igual(v any) Filter     { return condition(f.Field, Equal, ValorData(v)) }
func (f DateFilterField) Diferente(v any) Filter { return condition(f.Field, NotEqual, ValorData(v)) }
func (f DateFilterField) Antes(v any) Filter     { return condition(f.Field, LessThan, ValorData(v)) }
func (f DateFilterField) Depois(v any) Filter    { return condition(f.Field, GreaterThan, ValorData(v)) }

// AntesOuEm / DeOuDepois são os comparadores inclusivos (<= e >=), úteis em
// intervalo aberto de um lado: "até 31/12" ou "a partir de 01/01".
func (f DateFilterField) AntesOuEm(v any) Filter {
	return condition(f.Field, LessOrEqual, ValorData(v))
}
func (f DateFilterField) DeOuDepois(v any) Filter {
	return condition(f.Field, GreaterOrEqual, ValorData(v))
}

// Entre é o intervalo FECHADO (inclui as duas pontas): BETWEEN inicio AND fim.
func (f DateFilterField) Entre(inicio, fim any) Filter {
	return condition(f.Field, Between, ValorData(inicio), ValorData(fim))
}
func (f DateFilterField) Em(vs ...any) Filter { return condition(f.Field, In, valoresData(vs)...) }
func (f DateFilterField) Nulo() Filter        { return condition(f.Field, IsNull) }
func (f DateFilterField) NaoNulo() Filter     { return condition(f.Field, IsNotNull) }

// --- Booleano ---
func (f BoolFilterField) Igual(v bool) Filter { return condition(f.Field, Equal, v) }
func (f BoolFilterField) Verdadeiro() Filter  { return condition(f.Field, Equal, true) }
func (f BoolFilterField) Falso() Filter       { return condition(f.Field, Equal, false) }
func (f BoolFilterField) Nulo() Filter        { return condition(f.Field, IsNull) }
func (f BoolFilterField) NaoNulo() Filter     { return condition(f.Field, IsNotNull) }

func textsToAny(vs []string) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}
	return out
}

// Column identifica uma coluna tanto por um Field cru quanto pelo campo de
// filtro gerado (UsersFilter.Id). Serve para OrderBy e Select — a autoria fica
// uniforme (o dev sempre passa orm.Users.Field.<Coluna>).
type Column interface{ columnField() Field }

func (f Field) columnField() Field             { return f }
func (f StringFilterField) columnField() Field { return f.Field }
func (f NumberFilterField) columnField() Field { return f.Field }
func (f DateFilterField) columnField() Field   { return f.Field }
func (f BoolFilterField) columnField() Field   { return f.Field }

type ordering struct {
	field Field
	desc  bool
}

// Query é a pesquisa em construção, parametrizada pelo tipo da linha (T). É
// imutável: todo método encadeável devolve uma cópia, então uma Query base pode
// ser reaproveitada sem efeito colateral.
type Query[T any] struct {
	Entity   EntityFields
	items    []item
	orders   []ordering
	limit    int
	offset   int
	scan     Scanner[T]
	withs    []Relation
	selects  []Field // vazio = todas as colunas da entidade
	distinct bool
	agg      *aggregate // quando != nil, SELECT vira fn(col) (Sum/Avg/Min/Max)
}

// aggregate descreve uma função de agregação sobre uma coluna.
type aggregate struct {
	fn  string // SUM | AVG | MIN | MAX
	col Field
}

func (m Model[T]) Where(expressions ...Expression) Query[T] {
	return Query[T]{Entity: m.Entity, scan: m.scan}.and(expressions...)
}

// All inicia uma pesquisa sem filtro (SELECT de toda a tabela).
func (m Model[T]) All() Query[T] { return Query[T]{Entity: m.Entity, scan: m.scan} }

// Conveniências para começar a pesquisa já por ordenação/paginação.
func (m Model[T]) OrderBy(o Column) Query[T]     { return m.All().OrderBy(o) }
func (m Model[T]) OrderByDesc(o Column) Query[T] { return m.All().OrderByDesc(o) }
func (m Model[T]) Limit(n int) Query[T]          { return m.All().Limit(n) }
func (m Model[T]) Offset(n int) Query[T]         { return m.All().Offset(n) }

func (q Query[T]) clone() Query[T] {
	return Query[T]{
		Entity:   q.Entity,
		items:    append([]item(nil), q.items...),
		orders:   append([]ordering(nil), q.orders...),
		limit:    q.limit,
		offset:   q.offset,
		scan:     q.scan,
		withs:    append([]Relation(nil), q.withs...),
		selects:  append([]Field(nil), q.selects...),
		distinct: q.distinct,
		agg:      q.agg,
	}
}

// Distinct aplica SELECT DISTINCT.
func (m Model[T]) Distinct() Query[T] { return m.All().Distinct() }

func (q Query[T]) Distinct() Query[T] {
	next := q.clone()
	next.distinct = true
	return next
}

// Select projeta um subconjunto de colunas (evita trazer a tabela inteira). Os
// campos não selecionados ficam com o valor zero no Row tipado. Atenção: se for
// combinar com .With, inclua a coluna da FK/chave usada pela relação.
func (m Model[T]) Select(columns ...Column) Query[T] { return m.All().Select(columns...) }

func (q Query[T]) Select(columns ...Column) Query[T] {
	next := q.clone()
	for _, c := range columns {
		next.selects = append(next.selects, c.columnField())
	}
	return next
}

// With agenda o eager load das relações informadas (e das aninhadas nelas).
// Aceita tanto uma Relation quanto os caminhos tipados gerados
// (orm.Users.Relation.Cidade.Estado), que embutem Relation.
func (m Model[T]) With(relations ...RelationSource) Query[T] { return m.All().With(relations...) }

func (q Query[T]) With(relations ...RelationSource) Query[T] {
	return q.withAll(relationsDe(relations))
}

// withAll é a versão interna, já com as relações resolvidas.
func (q Query[T]) withAll(relations []Relation) Query[T] {
	next := q.clone()
	next.withs = append(next.withs, relations...)
	return next
}

// Where é a entrada única do filtro. Os argumentos são unidos por AND; o OR e o
// aninhamento vêm dos combinadores orm.Or(...) / orm.And(...) passados como
// argumento, mantendo a precedência explícita:
//
//	user.DB.Where(f.Ativo.Verdadeiro(), orm.Or(f.Uf.Igual("MT"), f.Uf.Igual("SP")))
//	// → WHERE ativo = ? AND (uf = ? OR uf = ?)
//
// Chamar Where mais de uma vez também acumula por AND (seguro: AND é associativo).
func (q Query[T]) Where(expressions ...Expression) Query[T] { return q.and(expressions...) }

func (q Query[T]) and(expressions ...Expression) Query[T] {
	next := q.clone()
	for _, e := range expressions {
		next.items = append(next.items, item{or: false, expr: e})
	}
	return next
}

func (q Query[T]) OrderBy(o Column) Query[T] {
	next := q.clone()
	next.orders = append(next.orders, ordering{field: o.columnField()})
	return next
}

func (q Query[T]) OrderByDesc(o Column) Query[T] {
	next := q.clone()
	next.orders = append(next.orders, ordering{field: o.columnField(), desc: true})
	return next
}

// appendOrders anexa ordenações já resolvidas (uso interno: constraints de relação).
func (q Query[T]) appendOrders(os []ordering) Query[T] {
	next := q.clone()
	next.orders = append(next.orders, os...)
	return next
}

func (q Query[T]) Limit(n int) Query[T]  { next := q.clone(); next.limit = n; return next }
func (q Query[T]) Offset(n int) Query[T] { next := q.clone(); next.offset = n; return next }

type Dialect string

const (
	Oracle    Dialect = "oracle"
	Postgres  Dialect = "postgres"
	MySQL     Dialect = "mysql"
	SQLServer Dialect = "sqlserver"
)

func supportedDialect(d Dialect) bool {
	switch d {
	case Oracle, Postgres, MySQL, SQLServer:
		return true
	default:
		return false
	}
}

type Operation string

const (
	Select Operation = "select"
	Count  Operation = "count"
	Exists Operation = "exists"
)

type CompileOptions struct {
	Dialect Dialect
	Schema  string
	Limit   int // usado só quando a Query não define Limit próprio
}
type CompiledQuery struct {
	SQL  string
	Args []any
}

// Explain é um atalho de DEPURAÇÃO: compila a pesquisa para um dialeto sem
// tocar no banco nem exigir schema. A API de uso final não passa dialeto — ele
// vem da conexão ativa do cliente na fase de execução. Serve para inspeção e
// testes multi-dialeto.
func (q Query[T]) Explain(operation Operation, dialect Dialect) (CompiledQuery, error) {
	return q.Compile(operation, CompileOptions{Dialect: dialect})
}

// compileCtx acumula os args e sabe emitir o placeholder do dialeto ativo.
type compileCtx struct {
	dialect     Dialect
	args        []any
	parentTable string // tabela da query externa (para EXISTS correlacionado)
	schema      string
}

func (c *compileCtx) bind(v any) string {
	c.args = append(c.args, v)
	return placeholderFor(c.dialect, len(c.args))
}

// Compile produz SQL parametrizado para o dialeto ativo. Valores nunca entram
// no texto do SQL — sempre viram placeholder + arg.
func (q Query[T]) Compile(operation Operation, options CompileOptions) (CompiledQuery, error) {
	d := options.Dialect
	if !supportedDialect(d) {
		return CompiledQuery{}, fmt.Errorf("dialeto %q ainda não suportado", d)
	}
	if q.Entity.Name == "" {
		return CompiledQuery{}, fmt.Errorf("model sem tabela")
	}

	limit := q.limit
	if limit == 0 && options.Limit > 0 {
		limit = options.Limit
	}
	offset := q.offset

	ctx := &compileCtx{dialect: d, parentTable: q.Entity.Name, schema: options.Schema}
	where, err := compileItems(q.items, ctx)
	if err != nil {
		return CompiledQuery{}, err
	}

	// Agregação: SELECT fn(col) FROM ... WHERE ...  (uma linha; sem order/limit/distinct).
	if operation == Select && q.agg != nil {
		sql := "SELECT " + q.agg.fn + "(" + quoteIdentFor(d, q.agg.col.Column) + ")" +
			" FROM " + qualifyFor(d, options.Schema, q.Entity.Name)
		if where != "" {
			sql += " WHERE " + where
		}
		return CompiledQuery{SQL: sql, Args: ctx.args}, nil
	}

	// ORDER BY (só faz sentido no Select).
	orderClause := ""
	if operation == Select && len(q.orders) > 0 {
		parts := make([]string, 0, len(q.orders))
		for _, o := range q.orders {
			direction := "ASC"
			if o.desc {
				direction = "DESC"
			}
			parts = append(parts, quoteIdentFor(d, o.field.Column)+" "+direction)
		}
		orderClause = "ORDER BY " + strings.Join(parts, ", ")
	}

	// Colunas do SELECT: projeção explícita (.Select) ou todas as da entidade.
	cols := q.Entity.Fields
	if len(q.selects) > 0 {
		cols = q.selects
	}

	// Prefixo do SELECT. No SQL Server o TOP entra aqui (não existe LIMIT).
	top := sqlServerTop(operation, d, offset, limit)
	prefix := selectPrefix(operation, d, cols, top, q.distinct)

	sql := prefix + " FROM " + qualifyFor(d, options.Schema, q.Entity.Name)
	if where != "" {
		sql += " WHERE " + where
	}

	// SQL Server exige ORDER BY para usar OFFSET/FETCH; sintetiza um estável.
	if d == SQLServer && offset > 0 && orderClause == "" {
		orderClause = "ORDER BY (SELECT NULL)"
	}
	if orderClause != "" {
		sql += " " + orderClause
	}

	sql += paginationTail(operation, d, offset, limit)

	return CompiledQuery{SQL: sql, Args: ctx.args}, nil
}

// selectPrefix monta "SELECT [DISTINCT] [TOP n] <colunas|COUNT(*)|1>".
func selectPrefix(operation Operation, d Dialect, cols []Field, top string, distinct bool) string {
	body := ""
	switch operation {
	case Count:
		body = "COUNT(*)"
	case Exists:
		body = "1"
	default:
		columns := make([]string, 0, len(cols))
		for _, f := range cols {
			columns = append(columns, quoteIdentFor(d, f.Column))
		}
		body = strings.Join(columns, ", ")
	}
	prefix := "SELECT "
	if distinct && operation == Select {
		prefix += "DISTINCT "
	}
	if top != "" {
		prefix += top + " "
	}
	return prefix + body
}

// sqlServerTop devolve o "TOP (n)" quando o SQL Server precisa limitar sem
// OFFSET (que exigiria ORDER BY): Exists sempre TOP 1; Select com limit e sem
// offset vira TOP limit. Nos demais dialetos é vazio.
func sqlServerTop(operation Operation, d Dialect, offset, limit int) string {
	if d != SQLServer {
		return ""
	}
	if operation == Exists {
		return "TOP 1"
	}
	if operation == Select && offset == 0 && limit > 0 {
		return fmt.Sprintf("TOP %d", limit)
	}
	return ""
}

// paginationTail rende a cláusula de limite/deslocamento que vai no fim do SQL.
func paginationTail(operation Operation, d Dialect, offset, limit int) string {
	if operation == Exists {
		switch d {
		case Oracle:
			return " FETCH FIRST 1 ROWS ONLY"
		case Postgres, MySQL:
			return " LIMIT 1"
		default: // sqlserver já resolveu com TOP 1
			return ""
		}
	}
	if operation != Select {
		return ""
	}
	switch d {
	case Oracle:
		return oracleOffsetFetch(offset, limit)
	case SQLServer:
		// Sem offset o TOP já cobriu; com offset usa OFFSET/FETCH (ORDER BY garantido).
		if offset > 0 {
			return sqlServerOffsetFetch(offset, limit)
		}
		return ""
	default: // postgres, mysql
		return postgresMySQLLimit(offset, limit)
	}
}

func oracleOffsetFetch(offset, limit int) string {
	sql := ""
	if offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d ROWS", offset)
	}
	if limit > 0 {
		if offset > 0 {
			sql += fmt.Sprintf(" FETCH NEXT %d ROWS ONLY", limit)
		} else {
			sql += fmt.Sprintf(" FETCH FIRST %d ROWS ONLY", limit)
		}
	}
	return sql
}

func sqlServerOffsetFetch(offset, limit int) string {
	sql := fmt.Sprintf(" OFFSET %d ROWS", offset)
	if limit > 0 {
		sql += fmt.Sprintf(" FETCH NEXT %d ROWS ONLY", limit)
	}
	return sql
}

func postgresMySQLLimit(offset, limit int) string {
	sql := ""
	if limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", limit)
	}
	if offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d", offset)
	}
	return sql
}

// compileItems renderiza uma lista de itens, ignorando expressões inativas e
// aplicando o conector (AND/OR) só entre fragmentos realmente escritos.
func compileItems(items []item, ctx *compileCtx) (string, error) {
	var b strings.Builder
	wrote := false
	for _, it := range items {
		if it.expr == nil || !it.expr.active() {
			continue
		}
		frag, err := compileExpression(it.expr, ctx)
		if err != nil {
			return "", err
		}
		if frag == "" {
			continue
		}
		if wrote {
			if it.or {
				b.WriteString(" OR ")
			} else {
				b.WriteString(" AND ")
			}
		}
		b.WriteString(frag)
		wrote = true
	}
	return b.String(), nil
}

func compileExpression(e Expression, ctx *compileCtx) (string, error) {
	switch v := e.(type) {
	case Filter:
		return compileFilter(v, ctx)
	case Group:
		inner, err := compileItems(v.items, ctx)
		if err != nil {
			return "", err
		}
		if inner == "" {
			return "", nil
		}
		return "(" + inner + ")", nil
	case existsPredicate:
		return compileExists(v, ctx)
	default:
		return "", fmt.Errorf("expressão não suportada")
	}
}

func compileFilter(f Filter, ctx *compileCtx) (string, error) {
	column := quoteIdentFor(ctx.dialect, f.Field.Column)
	switch f.Operator {
	case Equal, NotEqual, GreaterThan, GreaterOrEqual, LessThan, LessOrEqual:
		if len(f.Values) != 1 {
			return "", fmt.Errorf("operador %s exige um valor", f.Operator)
		}
		return fmt.Sprintf("%s %s %s", column, comparator(f.Operator), ctx.bind(f.Values[0])), nil
	case Contains, StartsWith, EndsWith:
		if len(f.Values) != 1 {
			return "", fmt.Errorf("operador %s exige um valor", f.Operator)
		}
		text, ok := f.Values[0].(string)
		if !ok {
			return "", fmt.Errorf("operador %s exige texto", f.Operator)
		}
		return fmt.Sprintf("%s LIKE %s", column, ctx.bind(likePattern(f.Operator, text))), nil
	case In:
		if len(f.Values) == 0 {
			return "", nil
		}
		placeholders := make([]string, 0, len(f.Values))
		for _, v := range f.Values {
			placeholders = append(placeholders, ctx.bind(v))
		}
		return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ", ")), nil
	case Between:
		if len(f.Values) != 2 {
			return "", fmt.Errorf("between exige dois valores")
		}
		low := ctx.bind(f.Values[0])
		high := ctx.bind(f.Values[1])
		return fmt.Sprintf("%s BETWEEN %s AND %s", column, low, high), nil
	case IsNull:
		return column + " IS NULL", nil
	case IsNotNull:
		return column + " IS NOT NULL", nil
	default:
		return "", fmt.Errorf("operador %s ainda não compilado", f.Operator)
	}
}

func comparator(op Operator) string {
	switch op {
	case Equal:
		return "="
	case NotEqual:
		return "<>"
	case GreaterThan:
		return ">"
	case GreaterOrEqual:
		return ">="
	case LessThan:
		return "<"
	case LessOrEqual:
		return "<="
	default:
		return "="
	}
}

func likePattern(op Operator, text string) string {
	switch op {
	case StartsWith:
		return text + "%"
	case EndsWith:
		return "%" + text
	default: // Contains
		return "%" + text + "%"
	}
}

// --- Regras de identificador/placeholder por dialeto ---
// Espelham internal/migraterun/runner.go (quote/placeholder/qualified) para que
// o SQL do ORM caia exatamente sobre os nomes físicos que as migrations criam:
// Oracle dobra para maiúsculas, Postgres/MySQL preservam minúsculas, etc.

func placeholderFor(d Dialect, position int) string {
	switch d {
	case Postgres:
		return fmt.Sprintf("$%d", position)
	case Oracle:
		return fmt.Sprintf(":%d", position)
	case SQLServer:
		return fmt.Sprintf("@p%d", position)
	default: // mysql
		return "?"
	}
}

func quoteIdentFor(d Dialect, value string) string {
	switch d {
	case MySQL:
		return "`" + strings.ReplaceAll(value, "`", "``") + "`"
	case SQLServer:
		return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
	case Oracle:
		return `"` + strings.ReplaceAll(strings.ToUpper(value), `"`, `""`) + `"`
	default: // postgres
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
}

func qualifyFor(d Dialect, schema, table string) string {
	if strings.TrimSpace(schema) == "" || d == MySQL {
		return quoteIdentFor(d, table)
	}
	return quoteIdentFor(d, schema) + "." + quoteIdentFor(d, table)
}

// selectFields define a projeção a partir de campos já resolvidos (uso interno:
// projeção de nó de relação).
func (q Query[T]) selectFields(fs []Field) Query[T] {
	next := q.clone()
	next.selects = append([]Field(nil), fs...)
	return next
}
