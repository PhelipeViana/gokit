// Package orm contains GoKit's reusable read-query engine. Applications only
// receive generated mappings that compose these types.
package orm

import (
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
type Model struct{ Entity EntityFields }

func NewModel(entity EntityFields) Model { return Model{Entity: entity} }

type Operator string

const (
	Equal     Operator = "equal"
	Contains  Operator = "contains"
	IsNull    Operator = "is_null"
	IsNotNull Operator = "is_not_null"
)

type Expression interface {
	expression()
	active() bool
}
type Filter struct {
	Field    Field
	Operator Operator
	Values   []any
	Active   bool
}

func (Filter) expression()    {}
func (f Filter) active() bool { return f.Active }

type StringFilterField struct{ Field Field }
type NumberFilterField struct{ Field Field }
type DateFilterField struct{ Field Field }
type BoolFilterField struct{ Field Field }

func StringFilter(f Field) StringFilterField { return StringFilterField{f} }
func NumberFilter(f Field) NumberFilterField { return NumberFilterField{f} }
func DateFilter(f Field) DateFilterField     { return DateFilterField{f} }
func BoolFilter(f Field) BoolFilterField     { return BoolFilterField{f} }
func condition(f Field, op Operator, values ...any) Filter {
	return Filter{Field: f, Operator: op, Values: values, Active: len(values) > 0 || op == IsNull || op == IsNotNull}
}
func (f StringFilterField) Igual(v string) Filter  { return condition(f.Field, Equal, v) }
func (f StringFilterField) Contem(v string) Filter { return condition(f.Field, Contains, v) }
func (f StringFilterField) Nulo() Filter           { return condition(f.Field, IsNull) }
func (f StringFilterField) NaoNulo() Filter        { return condition(f.Field, IsNotNull) }
func (f NumberFilterField) Igual(v any) Filter     { return condition(f.Field, Equal, v) }
func (f DateFilterField) Igual(v any) Filter       { return condition(f.Field, Equal, v) }
func (f BoolFilterField) Igual(v any) Filter       { return condition(f.Field, Equal, v) }

type Query struct {
	Entity      EntityFields
	Expressions []Expression
}

func (m Model) Filter(expressions ...Expression) Query {
	return Query{Entity: m.Entity, Expressions: append([]Expression(nil), expressions...)}
}
func (q Query) Filter(expressions ...Expression) Query {
	next := append([]Expression(nil), q.Expressions...)
	return Query{Entity: q.Entity, Expressions: append(next, expressions...)}
}

type Dialect string

const Oracle Dialect = "oracle"

type Operation string

const (
	Select Operation = "select"
	Count  Operation = "count"
	Exists Operation = "exists"
)

type CompileOptions struct {
	Dialect Dialect
	Schema  string
	Limit   int
}
type CompiledQuery struct {
	SQL  string
	Args []any
}

// Compile produces parameterized SQL. Values never enter SQL text.
func (q Query) Compile(operation Operation, options CompileOptions) (CompiledQuery, error) {
	if options.Dialect != Oracle {
		return CompiledQuery{}, fmt.Errorf("dialeto %q ainda não suportado", options.Dialect)
	}
	if q.Entity.Name == "" {
		return CompiledQuery{}, fmt.Errorf("model sem tabela")
	}
	columns := make([]string, 0, len(q.Entity.Fields))
	for _, f := range q.Entity.Fields {
		columns = append(columns, quote(f.Column))
	}
	prefix := "SELECT " + strings.Join(columns, ", ")
	if operation == Count {
		prefix = "SELECT COUNT(*)"
	}
	if operation == Exists {
		prefix = "SELECT 1"
	}
	table := quote(q.Entity.Name)
	if options.Schema != "" {
		table = quote(options.Schema) + "." + table
	}
	args := []any{}
	where := []string{}
	for _, expression := range q.Expressions {
		f, ok := expression.(Filter)
		if !ok || !f.Active {
			continue
		}
		column := quote(f.Field.Column)
		switch f.Operator {
		case Equal:
			if len(f.Values) != 1 {
				return CompiledQuery{}, fmt.Errorf("equal exige um valor")
			}
			args = append(args, f.Values[0])
			where = append(where, fmt.Sprintf("%s = :%d", column, len(args)))
		case Contains:
			if len(f.Values) != 1 {
				return CompiledQuery{}, fmt.Errorf("contains exige um valor")
			}
			text, ok := f.Values[0].(string)
			if !ok {
				return CompiledQuery{}, fmt.Errorf("contains exige texto")
			}
			args = append(args, "%"+text+"%")
			where = append(where, fmt.Sprintf("%s LIKE :%d", column, len(args)))
		case IsNull:
			where = append(where, column+" IS NULL")
		case IsNotNull:
			where = append(where, column+" IS NOT NULL")
		default:
			return CompiledQuery{}, fmt.Errorf("operador %s ainda não compilado", f.Operator)
		}
	}
	sql := prefix + " FROM " + table
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	if operation == Exists {
		sql += " FETCH FIRST 1 ROWS ONLY"
	} else if operation == Select && options.Limit > 0 {
		sql += fmt.Sprintf(" FETCH FIRST %d ROWS ONLY", options.Limit)
	}
	return CompiledQuery{SQL: sql, Args: args}, nil
}
func quote(value string) string {
	return `"` + strings.ReplaceAll(strings.ToUpper(value), `"`, `""`) + `"`
}
