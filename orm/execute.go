package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Runner é a ponte entre a pesquisa (agnóstica) e a conexão ativa. Quem sabe o
// dialeto/schema e como falar com o banco é a conexão — o motor só compila e
// pede a execução. O pacote público gokit/db implementa esta interface.
//
// As assinaturas de QueryContext/QueryRowContext batem com as de *sql.DB, então
// uma conexão pode simplesmente embutir *sql.DB.
type Runner interface {
	Dialect() Dialect
	Schema() string
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// defaultRunner é a "conexão padrão" (metade global do modelo híbrido). Os
// métodos sem sufixo (Get, Count...) a usam; os métodos *With recebem um Runner
// explícito para transação/multi-banco.
var defaultRunner Runner

// UseDefault instala a conexão padrão global. Chamado por db.Connect.
func UseDefault(r Runner) { defaultRunner = r }

// ErrSemConexao indica que não há conexão padrão nem Runner explícito.
var ErrSemConexao = errors.New("orm: nenhuma conexão ativa — chame db.Connect(ctx) ou use os métodos *With")

// Record é uma linha genérica (map), chaveada pelo nome da coluna em minúsculas.
// Serve de retorno "sem tipo" quando não há struct gerada — o código gerado usa
// os *Row tipados. Use orm.NewModel[orm.Record](entity, orm.ScanRecords).
type Record map[string]any

func resolveRunner(r Runner) (Runner, error) {
	if r == nil {
		r = defaultRunner
	}
	if r == nil {
		return nil, ErrSemConexao
	}
	return r, nil
}

func options(r Runner) CompileOptions {
	return CompileOptions{Dialect: r.Dialect(), Schema: r.Schema()}
}

// Get executa o SELECT na conexão padrão e devolve as linhas tipadas.
func (q Query[T]) Get(ctx context.Context) ([]T, error) { return q.GetWith(ctx, nil) }

// GetWith executa o SELECT numa conexão explícita.
func (q Query[T]) GetWith(ctx context.Context, r Runner) ([]T, error) {
	run, err := resolveRunner(r)
	if err != nil {
		return nil, err
	}
	compiled, err := q.Compile(Select, options(run))
	if err != nil {
		return nil, err
	}
	rows, err := run.QueryContext(ctx, compiled.SQL, compiled.Args...)
	if err != nil {
		return nil, err
	}
	linhas, err := q.scan(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Eager load: cada relação roda sua própria query e costura nas linhas.
	for _, rel := range q.withs {
		if err := rel.load(ctx, run, linhas, rel); err != nil {
			return nil, err
		}
	}
	return linhas, nil
}

// First devolve a primeira linha (found=false quando não há nenhuma).
func (q Query[T]) First(ctx context.Context) (T, bool, error) { return q.FirstWith(ctx, nil) }

func (q Query[T]) FirstWith(ctx context.Context, r Runner) (T, bool, error) {
	var zero T
	rows, err := q.Limit(1).GetWith(ctx, r)
	if err != nil {
		return zero, false, err
	}
	if len(rows) == 0 {
		return zero, false, nil
	}
	return rows[0], true, nil
}

// Count executa SELECT COUNT(*).
func (q Query[T]) Count(ctx context.Context) (int64, error) { return q.CountWith(ctx, nil) }

func (q Query[T]) CountWith(ctx context.Context, r Runner) (int64, error) {
	run, err := resolveRunner(r)
	if err != nil {
		return 0, err
	}
	compiled, err := q.Compile(Count, options(run))
	if err != nil {
		return 0, err
	}
	var total int64
	if err := run.QueryRowContext(ctx, compiled.SQL, compiled.Args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// Exists responde se há ao menos uma linha que satisfaz a pesquisa.
func (q Query[T]) Exists(ctx context.Context) (bool, error) { return q.ExistsWith(ctx, nil) }

func (q Query[T]) ExistsWith(ctx context.Context, r Runner) (bool, error) {
	run, err := resolveRunner(r)
	if err != nil {
		return false, err
	}
	compiled, err := q.Compile(Exists, options(run))
	if err != nil {
		return false, err
	}
	rows, err := run.QueryContext(ctx, compiled.SQL, compiled.Args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	found := rows.Next()
	return found, rows.Err()
}

// Atalhos no Model para a tabela inteira (sem filtro).
func (m Model[T]) Get(ctx context.Context) ([]T, error)       { return m.All().Get(ctx) }
func (m Model[T]) Count(ctx context.Context) (int64, error)   { return m.All().Count(ctx) }
func (m Model[T]) First(ctx context.Context) (T, bool, error) { return m.All().First(ctx) }
func (m Model[T]) Exists(ctx context.Context) (bool, error)   { return m.All().Exists(ctx) }

// ScanRecords é o Scanner embutido para Record (map). Normaliza a chave para
// minúsculas e converte []byte em string (o driver do MySQL devolve texto/número
// como []byte). Os *Row gerados têm scanner próprio, tipado por coluna.
func ScanRecords(rows *sql.Rows) ([]Record, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(cols))
	for i, c := range cols {
		keys[i] = strings.ToLower(c)
	}
	var out []Record
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		rec := make(Record, len(cols))
		for i := range keys {
			rec[keys[i]] = normalizeCell(cells[i])
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func normalizeCell(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// ------------------------------------------------------------ Agregações

func (q Query[T]) aggScalar(ctx context.Context, r Runner, fn string, col Column) (float64, error) {
	run, err := resolveRunner(r)
	if err != nil {
		return 0, err
	}
	qq := q.clone()
	qq.agg = &aggregate{fn: fn, col: col.columnField()}
	compiled, err := qq.Compile(Select, options(run))
	if err != nil {
		return 0, err
	}
	var v sql.NullFloat64
	if err := run.QueryRowContext(ctx, compiled.SQL, compiled.Args...).Scan(&v); err != nil {
		return 0, err
	}
	return v.Float64, nil // NULL (sem linhas) → 0
}

func (q Query[T]) Sum(ctx context.Context, col Column) (float64, error) {
	return q.aggScalar(ctx, nil, "SUM", col)
}
func (q Query[T]) Avg(ctx context.Context, col Column) (float64, error) {
	return q.aggScalar(ctx, nil, "AVG", col)
}
func (q Query[T]) Min(ctx context.Context, col Column) (float64, error) {
	return q.aggScalar(ctx, nil, "MIN", col)
}
func (q Query[T]) Max(ctx context.Context, col Column) (float64, error) {
	return q.aggScalar(ctx, nil, "MAX", col)
}

func (m Model[T]) Sum(ctx context.Context, col Column) (float64, error) { return m.All().Sum(ctx, col) }
func (m Model[T]) Avg(ctx context.Context, col Column) (float64, error) { return m.All().Avg(ctx, col) }
func (m Model[T]) Min(ctx context.Context, col Column) (float64, error) { return m.All().Min(ctx, col) }
func (m Model[T]) Max(ctx context.Context, col Column) (float64, error) { return m.All().Max(ctx, col) }

// ------------------------------------------------------------ Paginação

// Page é o resultado paginado: itens da página + metadados.
type Page[T any] struct {
	Items []T
	Total int64
	Page  int
	Size  int
	Pages int
}

func (q Query[T]) Paginate(ctx context.Context, page, size int) (Page[T], error) {
	return q.PaginateWith(ctx, nil, page, size)
}

func (q Query[T]) PaginateWith(ctx context.Context, r Runner, page, size int) (Page[T], error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 1
	}
	base := q.clone()
	base.limit, base.offset = 0, 0
	total, err := base.CountWith(ctx, r)
	if err != nil {
		return Page[T]{}, err
	}
	items, err := q.Limit(size).Offset((page-1)*size).GetWith(ctx, r)
	if err != nil {
		return Page[T]{}, err
	}
	pages := int((total + int64(size) - 1) / int64(size))
	return Page[T]{Items: items, Total: total, Page: page, Size: size, Pages: pages}, nil
}

func (m Model[T]) Paginate(ctx context.Context, page, size int) (Page[T], error) {
	return m.All().Paginate(ctx, page, size)
}

// ------------------------------------------------------------ Find / Pluck

func primaryKeyOf(e EntityFields) (Field, bool) {
	for _, f := range e.Fields {
		if f.PrimaryKey {
			return f, true
		}
	}
	return Field{}, false
}

// Find busca uma linha pela chave primária. found=false quando não existe.
func (m Model[T]) Find(ctx context.Context, id any) (T, bool, error) { return m.FindWith(ctx, nil, id) }

func (m Model[T]) FindWith(ctx context.Context, r Runner, id any) (T, bool, error) {
	pk, ok := primaryKeyOf(m.Entity)
	if !ok {
		var zero T
		return zero, false, fmt.Errorf("orm: entidade %q não tem chave primária para Find", m.Entity.Name)
	}
	return m.Where(Filter{Field: pk, Operator: Equal, Values: []any{id}, Active: true}).FirstWith(ctx, r)
}

// Pluck devolve os valores de UMA coluna como slice (ex.: ids como []any).
func (q Query[T]) Pluck(ctx context.Context, col Column) ([]any, error) {
	return q.PluckWith(ctx, nil, col)
}

func (q Query[T]) PluckWith(ctx context.Context, r Runner, col Column) ([]any, error) {
	run, err := resolveRunner(r)
	if err != nil {
		return nil, err
	}
	qq := q.clone()
	qq.selects = []Field{col.columnField()}
	compiled, err := qq.Compile(Select, options(run))
	if err != nil {
		return nil, err
	}
	rows, err := run.QueryContext(ctx, compiled.SQL, compiled.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []any
	for rows.Next() {
		var v any
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, normalizeCell(v))
	}
	return out, rows.Err()
}

func (m Model[T]) Pluck(ctx context.Context, col Column) ([]any, error) {
	return m.All().Pluck(ctx, col)
}
