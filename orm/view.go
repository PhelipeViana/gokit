package orm

// Fachada de LEITURA para view.
//
// Existe porque `Model[T]` e `Query[T]` carregam escrita — `Insert`, `UpdateByKey`,
// `DeleteByKey`, `Update`, `Delete` — e view não é lugar de escrever. Só view trivial é
// atualizável, a regra de qual muda em cada banco, e o gokit não tem como saber pelo
// catálogo. Expor um `Insert` que falha na maioria dos casos seria pior que não expor:
// quem usa descobriria a limitação em produção.
//
// A fachada é escrita UMA vez aqui, e não em cada view gerada. Com 553 views num
// projeto legado, repetir trinta métodos por view seria megabytes de código gerado
// dizendo a mesma coisa.
//
// O que ela NÃO tem é tão importante quanto o que tem: nenhum caminho daqui alcança
// escrita. Quem precisar escrever numa view atualizável usa SQL cru e assume a
// responsabilidade — explicitamente, não por acidente de API.

import (
	"context"
	"database/sql"
)

// View é o handle de leitura de uma view do banco.
//
// O tipo é genérico na linha para que `Get` devolva a struct tipada, igual às
// entidades. A construção fica com o código gerado, que conhece as colunas.
type View[T any] struct {
	modelo Model[T]
}

// NewView monta o handle a partir dos campos da view e do scanner tipado.
//
// Recebe o mesmo par que NewModel porque a leitura é idêntica: para o SELECT, view e
// tabela são a mesma coisa. A diferença mora só na superfície exposta.
func NewView[T any](campos EntityFields, scan func(*sql.Rows) ([]T, error)) View[T] {
	return View[T]{modelo: NewModel[T](campos, scan)}
}

// ── Construção da consulta ──

// All abre uma consulta sem filtro.
func (v View[T]) All() Query[T] { return v.modelo.All() }

// Where filtra. Aceita as mesmas condições das entidades, incluindo Filter e When.
func (v View[T]) Where(expressoes ...Expression) Query[T] { return v.modelo.Where(expressoes...) }

// Select projeta colunas. Sem ele, a view devolve todas.
func (v View[T]) Select(colunas ...Column) Query[T] { return v.modelo.Select(colunas...) }

// OrderBy e OrderByDesc ordenam.
func (v View[T]) OrderBy(coluna Column) Query[T]     { return v.modelo.OrderBy(coluna) }
func (v View[T]) OrderByDesc(coluna Column) Query[T] { return v.modelo.OrderByDesc(coluna) }

// Limit e Offset recortam.
func (v View[T]) Limit(quantidade int) Query[T]  { return v.modelo.All().Limit(quantidade) }
func (v View[T]) Offset(quantidade int) Query[T] { return v.modelo.All().Offset(quantidade) }

// Distinct remove duplicata.
func (v View[T]) Distinct() Query[T] { return v.modelo.Distinct() }

// GroupBy agrupa. Agregação em view é o caso mais comum de relatório.
func (v View[T]) GroupBy(colunas ...Column) Query[T] { return v.modelo.GroupBy(colunas...) }

// Join e LeftJoin cruzam a view com outra fonte, pela mesma forma das entidades.
func (v View[T]) Join(relacoes ...RelationSource) Query[T] { return v.modelo.Join(relacoes...) }
func (v View[T]) LeftJoin(relacoes ...RelationSource) Query[T] {
	return v.modelo.LeftJoin(relacoes...)
}

// ── Terminais ──

func (v View[T]) Get(ctx context.Context) ([]T, error)     { return v.modelo.Get(ctx) }
func (v View[T]) First(ctx context.Context) (T, error)     { return v.modelo.First(ctx) }
func (v View[T]) Count(ctx context.Context) (int64, error) { return v.modelo.Count(ctx) }
func (v View[T]) Exists(ctx context.Context) (bool, error) { return v.modelo.Exists(ctx) }

// As variantes *With recebem o Runner explícito — é como se lê dentro de uma
// transação aberta por quem chama.
func (v View[T]) GetWith(ctx context.Context, r Runner) ([]T, error) { return v.modelo.GetWith(ctx, r) }
func (v View[T]) FirstWith(ctx context.Context, r Runner) (T, error) {
	return v.modelo.FirstWith(ctx, r)
}
func (v View[T]) CountWith(ctx context.Context, r Runner) (int64, error) {
	return v.modelo.CountWith(ctx, r)
}
func (v View[T]) ExistsWith(ctx context.Context, r Runner) (bool, error) {
	return v.modelo.ExistsWith(ctx, r)
}

// ── Agregação ──

func (v View[T]) Sum(ctx context.Context, coluna Column) (Value, error) {
	return v.modelo.Sum(ctx, coluna)
}
func (v View[T]) Avg(ctx context.Context, coluna Column) (Value, error) {
	return v.modelo.Avg(ctx, coluna)
}
func (v View[T]) Min(ctx context.Context, coluna Column) (Value, error) {
	return v.modelo.Min(ctx, coluna)
}
func (v View[T]) Max(ctx context.Context, coluna Column) (Value, error) {
	return v.modelo.Max(ctx, coluna)
}

// ── Percurso em lote ──

// Each e Chunk percorrem sem carregar tudo em memória. É o que serve relatório grande,
// que é justamente o que view de legado costuma ser.
func (v View[T]) Each(ctx context.Context, fn func(T) error) error { return v.modelo.Each(ctx, fn) }
func (v View[T]) Chunk(ctx context.Context, tamanho int, fn func([]T) error) error {
	return v.modelo.Chunk(ctx, tamanho, fn)
}

// Model devolve o modelo interno, para quem precisa passar a view como fonte de um
// Join de outra entidade.
//
// É uma porta estreita de propósito: dá acesso à leitura em contextos que a fachada não
// cobre, sem transformar a view num handle de escrita.
func (v View[T]) Model() Model[T] { return v.modelo }
