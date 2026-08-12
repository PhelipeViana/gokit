package orm

// Bloco 4 (parte 1): percorrer em lotes e travar linha.
//
// Chunk/Each existem para o caso em que o resultado não cabe na memória —
// relatório de tabela inteira, migração de dado, reprocessamento. Sem eles, a
// única saída era Get() e rezar, ou paginar à mão.
//
// Lock existe para o caso em que ler e gravar precisam ser atômicos: ler o saldo,
// decidir, gravar. Sem trava, dois pedidos simultâneos leem o mesmo saldo e o
// segundo sobrescreve a decisão do primeiro.

import (
	"context"
	"fmt"
)

// tamanhoDeLotePadrao é o lote do Each. Mil linhas é grande o bastante para o
// custo por consulta diluir e pequeno o bastante para não pesar na memória.
const tamanhoDeLotePadrao = 1000

// Chunk percorre o resultado em lotes, chamando fn uma vez por lote.
//
// A paginação é por CHAVE (WHERE id > último), não por OFFSET. A diferença
// importa: com OFFSET, uma linha inserida ou removida no meio do percurso desloca
// as páginas seguintes, e o percurso pula ou repete linha sem avisar. Por chave,
// cada lote continua exatamente de onde o anterior parou.
//
// Exige chave primária de coluna única. Chave composta cai no OFFSET, que é o que
// dá para fazer, com o aviso acima valendo.
func (q Query[T]) Chunk(ctx context.Context, tamanho int, fn func([]T) error) error {
	return q.ChunkWith(ctx, nil, tamanho, fn)
}

func (q Query[T]) ChunkWith(ctx context.Context, r Runner, tamanho int, fn func([]T) error) error {
	if tamanho <= 0 {
		return fmt.Errorf("orm: tamanho de lote precisa ser maior que zero")
	}
	chaves := q.Entity.PrimaryKeys()
	if len(chaves) == 1 {
		return q.chunkPorChave(ctx, r, chaves[0], tamanho, fn)
	}
	return q.chunkPorOffset(ctx, r, tamanho, fn)
}

// chunkPorChave é o caminho estável: cada lote filtra pelo que veio depois do
// último visto, então inserção concorrente não desloca nada.
func (q Query[T]) chunkPorChave(ctx context.Context, r Runner, chave Field, tamanho int, fn func([]T) error) error {
	var ultimo any
	for {
		lote := q
		if ultimo != nil {
			lote = lote.Where(condition(chave, GreaterThan, ultimo))
		}
		linhas, err := lote.OrderBy(chave).Limit(tamanho).GetWith(ctx, r)
		if err != nil {
			return err
		}
		if len(linhas) == 0 {
			return nil
		}
		if err := fn(linhas); err != nil {
			return err
		}
		valor, ok := valorDaLinha(linhas[len(linhas)-1], chave)
		if !ok || valor == nil {
			return fmt.Errorf("orm: não foi possível ler %s da última linha do lote; o Chunk precisa da chave na projeção", chave.Column)
		}
		ultimo = valor
		if len(linhas) < tamanho {
			return nil
		}
	}
}

// chunkPorOffset é o caminho de chave composta. Fica documentado como menos
// seguro: escrita concorrente pode deslocar as páginas.
func (q Query[T]) chunkPorOffset(ctx context.Context, r Runner, tamanho int, fn func([]T) error) error {
	for pagina := 0; ; pagina++ {
		linhas, err := q.Limit(tamanho).Offset(pagina*tamanho).GetWith(ctx, r)
		if err != nil {
			return err
		}
		if len(linhas) == 0 {
			return nil
		}
		if err := fn(linhas); err != nil {
			return err
		}
		if len(linhas) < tamanho {
			return nil
		}
	}
}

// Each percorre linha por linha, em lotes por baixo. Devolver erro em fn
// interrompe o percurso e propaga — é como se sai no meio.
func (q Query[T]) Each(ctx context.Context, fn func(T) error) error {
	return q.EachWith(ctx, nil, fn)
}

func (q Query[T]) EachWith(ctx context.Context, r Runner, fn func(T) error) error {
	return q.ChunkWith(ctx, r, tamanhoDeLotePadrao, func(lote []T) error {
		for _, linha := range lote {
			if err := fn(linha); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m Model[T]) Each(ctx context.Context, fn func(T) error) error { return m.All().Each(ctx, fn) }
func (m Model[T]) Chunk(ctx context.Context, tamanho int, fn func([]T) error) error {
	return m.All().Chunk(ctx, tamanho, fn)
}

// ── Trava de linha ──

// Lock marca a pesquisa para travar as linhas lidas até o fim da transação.
//
// Só faz sentido dentro de transação: fora dela, a trava é liberada no fim da
// própria consulta e não protege nada. Por isso o compilador não deixa isso
// implícito — quem chama Lock está dizendo que já está numa transação.
//
// A sintaxe divide os bancos em dois grupos, e a posição no comando é diferente:
//
//	MySQL, Postgres, Oracle  → FOR UPDATE, no fim
//	SQL Server               → WITH (UPDLOCK, ROWLOCK), colado na tabela
func (q Query[T]) Lock() Query[T] {
	next := q.clone()
	next.travar = true
	return next
}

func (m Model[T]) Lock() Query[T] { return m.All().Lock() }

// dicaDeTabela é o sufixo colado no nome da tabela. Só o SQL Server usa.
func dicaDeTabela(d Dialect, travar bool) string {
	if travar && d == SQLServer {
		return " WITH (UPDLOCK, ROWLOCK)"
	}
	return ""
}

// sufixoDeTrava é o que vai no fim do comando, nos outros três.
func sufixoDeTrava(d Dialect, travar bool) string {
	if !travar || d == SQLServer {
		return ""
	}
	return " FOR UPDATE"
}
