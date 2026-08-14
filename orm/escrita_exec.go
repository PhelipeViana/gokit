package orm

// Terminais de escrita: os métodos que o desenvolvedor chama.
//
// Seguem a mesma regra híbrida da leitura: o método sem sufixo usa a conexão
// padrão, o *With recebe um Runner explícito (transação, outro banco).
//
// Toda falha passa por ClassifyError, então quem chama recebe ErrDuplicate,
// ErrForeignKey ou ErrNotNull em vez de uma mensagem de driver — é o que permite
// a camada HTTP responder 409 ou 422 sem inspecionar texto.

import (
	"context"
	"database/sql"
)

// Result é o retorno de uma escrita.
//
// LastID só vem preenchido quando a tabela tem coluna auto-incremento e o
// dialeto sabe devolvê-la num INSERT de uma linha. Nos outros casos vale 0, e
// isso é informação legítima — não erro.
type Result struct {
	Affected int64
	LastID   int64
}

// Executor é a parte do Runner que escreve. Fica separada da leitura porque um
// Runner somente-leitura é legítimo (réplica), e nesse caso a escrita deve
// falhar na chamada, não no banco.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// runnerParaEscrita resolve o Runner e confirma que ele sabe escrever.
func runnerParaEscrita(r Runner) (Runner, Executor, error) {
	alvo := r
	if alvo == nil {
		alvo = defaultRunner
	}
	if alvo == nil {
		return nil, nil, ErrNoConnection
	}
	exec, ok := alvo.(Executor)
	if !ok {
		return nil, nil, errRunnerSemEscrita()
	}
	return alvo, exec, nil
}

// Insert grava uma linha e devolve a chave gerada quando o banco a fornece.
func (m Model[T]) Insert(ctx context.Context, valores Values) (Result, error) {
	return m.InsertWith(ctx, nil, valores)
}

func (m Model[T]) InsertWith(ctx context.Context, r Runner, valores Values) (Result, error) {
	if err := conferirAutoria(valores); err != nil {
		return Result{}, err
	}
	alvo, exec, err := runnerParaEscrita(r)
	if err != nil {
		return Result{}, err
	}
	compilada, err := compileInsert(m.Entity, []Values{valores},
		CompileOptions{Dialect: alvo.Dialect(), Schema: alvo.Schema()})
	if err != nil {
		return Result{}, err
	}
	return executarInsert(ctx, alvo, exec, m.Entity, compilada)
}

// InsertMany grava várias linhas num único INSERT.
//
// Não devolve LastID: com mais de uma linha, "a" chave gerada não existe — são
// várias, e cada banco devolve de um jeito. Quem precisa das chaves insere uma
// por vez ou lê depois.
func (m Model[T]) InsertMany(ctx context.Context, linhas []Values) (Result, error) {
	return m.InsertManyWith(ctx, nil, linhas)
}

func (m Model[T]) InsertManyWith(ctx context.Context, r Runner, linhas []Values) (Result, error) {
	if err := conferirAutoria(linhas...); err != nil {
		return Result{}, err
	}
	alvo, exec, err := runnerParaEscrita(r)
	if err != nil {
		return Result{}, err
	}
	compilada, err := compileInsert(m.Entity, linhas,
		CompileOptions{Dialect: alvo.Dialect(), Schema: alvo.Schema()})
	if err != nil {
		return Result{}, err
	}
	saida, err := exec.ExecContext(ctx, compilada.SQL, compilada.Args...)
	if err != nil {
		return Result{}, ClassifyError(err)
	}
	afetadas := linhasAfetadas(saida)
	return Result{Affected: afetadas}, nil
}

// Update grava os valores nas linhas que a pesquisa selecionou.
//
// Recusa pesquisa sem Where: UPDATE na tabela inteira é quase sempre acidente, e
// quando não é, .Unrestricted() diz isso em voz alta.
func (q Query[T]) Update(ctx context.Context, valores Values) (Result, error) {
	return q.UpdateWith(ctx, nil, valores)
}

func (q Query[T]) UpdateWith(ctx context.Context, r Runner, valores Values) (Result, error) {
	if err := conferirAutoria(valores); err != nil {
		return Result{}, err
	}
	if len(q.items) == 0 && !q.irrestrita {
		return Result{}, errSemWhere("UPDATE")
	}
	alvo, exec, err := runnerParaEscrita(r)
	if err != nil {
		return Result{}, err
	}
	compilada, err := compileUpdate(q, valores,
		CompileOptions{Dialect: alvo.Dialect(), Schema: alvo.Schema()})
	if err != nil {
		return Result{}, err
	}
	saida, err := exec.ExecContext(ctx, compilada.SQL, compilada.Args...)
	if err != nil {
		return Result{}, ClassifyError(err)
	}
	afetadas := linhasAfetadas(saida)
	return Result{Affected: afetadas}, nil
}

// Delete remove as linhas que a pesquisa selecionou. Mesma proteção do Update.
func (q Query[T]) Delete(ctx context.Context) (Result, error) {
	return q.DeleteWith(ctx, nil)
}

func (q Query[T]) DeleteWith(ctx context.Context, r Runner) (Result, error) {
	if len(q.items) == 0 && !q.irrestrita {
		return Result{}, errSemWhere("DELETE")
	}
	alvo, exec, err := runnerParaEscrita(r)
	if err != nil {
		return Result{}, err
	}
	compilada, err := compileDelete(q, CompileOptions{Dialect: alvo.Dialect(), Schema: alvo.Schema()})
	if err != nil {
		return Result{}, err
	}
	saida, err := exec.ExecContext(ctx, compilada.SQL, compilada.Args...)
	if err != nil {
		return Result{}, ClassifyError(err)
	}
	afetadas := linhasAfetadas(saida)
	return Result{Affected: afetadas}, nil
}

// UpdateByKey grava valores na linha identificada pela chave primária. Os valores
// da chave vêm na ordem em que as colunas foram declaradas na migration.
func (m Model[T]) UpdateByKey(ctx context.Context, valores Values, chave ...any) (Result, error) {
	return m.UpdateByKeyWith(ctx, nil, valores, chave...)
}

func (m Model[T]) UpdateByKeyWith(ctx context.Context, r Runner, valores Values, chave ...any) (Result, error) {
	condicao, err := m.Entity.condicaoPorChave(chave)
	if err != nil {
		return Result{}, err
	}
	return m.Where(condicao).UpdateWith(ctx, r, valores)
}

// DeleteByKey remove a linha identificada pela chave primária.
func (m Model[T]) DeleteByKey(ctx context.Context, chave ...any) (Result, error) {
	return m.DeleteByKeyWith(ctx, nil, chave...)
}

func (m Model[T]) DeleteByKeyWith(ctx context.Context, r Runner, chave ...any) (Result, error) {
	condicao, err := m.Entity.condicaoPorChave(chave)
	if err != nil {
		return Result{}, err
	}
	return m.Where(condicao).DeleteWith(ctx, r)
}

// Unrestricted libera a escrita sem Where. É o "sim, eu quero a tabela inteira" —
// existe para que o caso legítimo seja possível e o acidental seja impossível.
func (q Query[T]) Unrestricted() Query[T] {
	next := q.clone()
	next.irrestrita = true
	return next
}

// Unrestricted no Model, para quem parte da entidade.
func (m Model[T]) Unrestricted() Query[T] { return m.All().Unrestricted() }
