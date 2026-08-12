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

// ErrNoConnection indica que não há conexão padrão nem Runner explícito.
var ErrNoConnection = errors.New("orm: nenhuma conexão ativa — chame db.Connect(ctx) ou use os métodos *With")

// ErrNotFound é devolvido por First/Find quando a pesquisa não traz linha.
// É sentinela: use errors.Is(err, orm.ErrNotFound).
var ErrNotFound = errors.New("orm: registro não encontrado")

// Record é uma linha genérica (map), chaveada pelo nome da coluna em minúsculas.
// Serve de retorno "sem tipo" quando não há struct gerada — o código gerado usa
// os *Row tipados. Use orm.NewModel[orm.Record](entity, orm.ScanRecords).
type Record map[string]any

func resolveRunner(r Runner) (Runner, error) {
	if r == nil {
		r = defaultRunner
	}
	if r == nil {
		return nil, ErrNoConnection
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
	// Projeção com agregação não cabe na linha da entidade: um SELECT de cidade_id
	// + SUM(saldo) não é um UsersRow. Recusar aqui é melhor que devolver a linha
	// com os campos zerados e a agregação descartada em silêncio.
	if len(q.aggSelects) > 0 || len(q.groups) > 0 {
		return nil, ErrAggregateProjection
	}
	run, err := resolveRunner(r)
	if err != nil {
		return nil, err
	}
	compiled, err := q.Compile(OpSelect, options(run))
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
	// Eager load: cada relação roda sua própria query e costura nas linhas. O
	// merge une caminhos com prefixo comum (senão um sobrescreveria o outro).
	for _, rel := range mergeRelations(q.withs) {
		if err := rel.load(ctx, run, linhas, rel); err != nil {
			return nil, err
		}
	}
	if linhas == nil {
		// Slice vazia em vez de nil: no JSON isso é [] e não null.
		linhas = []T{}
	}
	return linhas, nil
}

// First devolve a primeira linha. Quando não há nenhuma, devolve
// ErrNotFound — trate com errors.Is(err, orm.ErrNotFound). Assim a
// assinatura fica idiomática (valor, erro) e o responder deriva o 404 do erro.
func (q Query[T]) First(ctx context.Context) (T, error) { return q.FirstWith(ctx, nil) }

func (q Query[T]) FirstWith(ctx context.Context, r Runner) (T, error) {
	var zero T
	rows, err := q.Limit(1).GetWith(ctx, r)
	if err != nil {
		return zero, err
	}
	if len(rows) == 0 {
		return zero, ErrNotFound
	}
	return rows[0], nil
}

// Count executa SELECT COUNT(*).
func (q Query[T]) Count(ctx context.Context) (int64, error) { return q.CountWith(ctx, nil) }

func (q Query[T]) CountWith(ctx context.Context, r Runner) (int64, error) {
	run, err := resolveRunner(r)
	if err != nil {
		return 0, err
	}
	compiled, err := q.Compile(OpCount, options(run))
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
	compiled, err := q.Compile(OpExists, options(run))
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
func (m Model[T]) Get(ctx context.Context) ([]T, error)     { return m.All().Get(ctx) }
func (m Model[T]) Count(ctx context.Context) (int64, error) { return m.All().Count(ctx) }
func (m Model[T]) First(ctx context.Context) (T, error)     { return m.All().First(ctx) }
func (m Model[T]) Exists(ctx context.Context) (bool, error) { return m.All().Exists(ctx) }

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

// aggScalar roda a agregação e lê o resultado CRU (any). Ler em any — e não em
// float64 — é o que permite MIN/MAX em coluna de data ou texto.
func (q Query[T]) aggScalar(ctx context.Context, r Runner, fn string, col Column) (Value, error) {
	run, err := resolveRunner(r)
	if err != nil {
		return Value{}, err
	}
	qq := q.clone()
	qq.agg = &aggregate{fn: fn, col: col.columnField()}
	compiled, err := qq.Compile(OpSelect, options(run))
	if err != nil {
		return Value{}, err
	}
	var bruto any
	if err := run.QueryRowContext(ctx, compiled.SQL, compiled.Args...).Scan(&bruto); err != nil {
		return Value{}, err
	}
	return NewValue(bruto), nil // sem linhas → Value.IsNull() == true
}

// Sum/Avg/Min/Max devolvem Value: extraia com .Float(), .Int(), .Tempo() ou
// .Texto() conforme a coluna. Value.IsNull() indica agregação sem linhas.
func (q Query[T]) Sum(ctx context.Context, col Column) (Value, error) {
	return q.aggScalar(ctx, nil, "SUM", col)
}
func (q Query[T]) Avg(ctx context.Context, col Column) (Value, error) {
	return q.aggScalar(ctx, nil, "AVG", col)
}
func (q Query[T]) Min(ctx context.Context, col Column) (Value, error) {
	return q.aggScalar(ctx, nil, "MIN", col)
}
func (q Query[T]) Max(ctx context.Context, col Column) (Value, error) {
	return q.aggScalar(ctx, nil, "MAX", col)
}

func (m Model[T]) Sum(ctx context.Context, col Column) (Value, error) { return m.All().Sum(ctx, col) }
func (m Model[T]) Avg(ctx context.Context, col Column) (Value, error) { return m.All().Avg(ctx, col) }
func (m Model[T]) Min(ctx context.Context, col Column) (Value, error) { return m.All().Min(ctx, col) }
func (m Model[T]) Max(ctx context.Context, col Column) (Value, error) { return m.All().Max(ctx, col) }

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
// Find busca pela chave primária. Sem linha, devolve ErrNotFound.
func (m Model[T]) Find(ctx context.Context, id any) (T, error) { return m.FindWith(ctx, nil, id) }

func (m Model[T]) FindWith(ctx context.Context, r Runner, id any) (T, error) {
	pk, ok := primaryKeyOf(m.Entity)
	if !ok {
		var zero T
		return zero, fmt.Errorf("orm: entidade %q não tem chave primária para Find", m.Entity.Name)
	}
	return m.Where(Condition{Field: pk, Operator: Equal, Values: []any{id}, Active: true}).FirstWith(ctx, r)
}

// Pluck devolve os valores de UMA coluna como []Value — extraia com .Int(),
// .Texto(), .Tempo()... Para os casos comuns há PluckInt/PluckTexto, que já
// devolvem o slice tipado.
func (q Query[T]) Pluck(ctx context.Context, col Column) ([]Value, error) {
	return q.PluckWith(ctx, nil, col)
}

func (q Query[T]) PluckWith(ctx context.Context, r Runner, col Column) ([]Value, error) {
	run, err := resolveRunner(r)
	if err != nil {
		return nil, err
	}
	qq := q.clone()
	qq.selects = []Field{col.columnField()}
	compiled, err := qq.Compile(OpSelect, options(run))
	if err != nil {
		return nil, err
	}
	rows, err := run.QueryContext(ctx, compiled.SQL, compiled.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Value{}
	for rows.Next() {
		var v any
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, NewValue(v))
	}
	return out, rows.Err()
}

// PluckInt / PluckTexto devolvem a coluna já no tipo Go correspondente.
func (q Query[T]) PluckInt(ctx context.Context, col Column) ([]int64, error) {
	vs, err := q.Pluck(ctx, col)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Int())
	}
	return out, nil
}

func (q Query[T]) PluckString(ctx context.Context, col Column) ([]string, error) {
	vs, err := q.Pluck(ctx, col)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Text())
	}
	return out, nil
}

// Variantes *With do Pluck tipado. Sem elas o contrato híbrido tinha um buraco:
// dava para plucar dentro de uma transação só perdendo a tipagem, voltando para
// []Value e convertendo à mão.
func (q Query[T]) PluckIntWith(ctx context.Context, r Runner, col Column) ([]int64, error) {
	vs, err := q.PluckWith(ctx, r, col)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Int())
	}
	return out, nil
}

func (q Query[T]) PluckStringWith(ctx context.Context, r Runner, col Column) ([]string, error) {
	vs, err := q.PluckWith(ctx, r, col)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Text())
	}
	return out, nil
}

func (m Model[T]) Pluck(ctx context.Context, col Column) ([]Value, error) {
	return m.All().Pluck(ctx, col)
}

func (m Model[T]) PluckInt(ctx context.Context, col Column) ([]int64, error) {
	return m.All().PluckInt(ctx, col)
}

func (m Model[T]) PluckString(ctx context.Context, col Column) ([]string, error) {
	return m.All().PluckString(ctx, col)
}

// Variantes *With das agregações (conexão explícita), para paridade com os
// demais terminais.
func (q Query[T]) SumWith(ctx context.Context, r Runner, col Column) (Value, error) {
	return q.aggScalar(ctx, r, "SUM", col)
}
func (q Query[T]) AvgWith(ctx context.Context, r Runner, col Column) (Value, error) {
	return q.aggScalar(ctx, r, "AVG", col)
}
func (q Query[T]) MinWith(ctx context.Context, r Runner, col Column) (Value, error) {
	return q.aggScalar(ctx, r, "MIN", col)
}
func (q Query[T]) MaxWith(ctx context.Context, r Runner, col Column) (Value, error) {
	return q.aggScalar(ctx, r, "MAX", col)
}
