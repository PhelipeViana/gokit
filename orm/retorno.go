package orm

// Devolução da chave gerada num INSERT.
//
// É o ponto em que os quatro bancos mais divergem, e por isso mora num arquivo
// só. Quem escreve não vê nada disso: chama Insert e recebe Result.LastID.
//
//	MySQL      LastInsertId() do próprio driver
//	Postgres   INSERT ... RETURNING "id"        → lê como consulta
//	SQL Server INSERT ... OUTPUT INSERTED.[id]  → lê como consulta
//	Oracle     INSERT ... RETURNING "ID" INTO :n → parâmetro de saída (sql.Out)
//
// Tabela sem coluna auto-incremento não tem chave para devolver: o INSERT roda
// como Exec puro e LastID fica 0. Isso é resposta, não falha.

import (
	"context"
	"database/sql"
)

func executarInsert(ctx context.Context, r Runner, exec Executor, entity EntityFields, compilada CompiledQuery) (Result, error) {
	identidade, temIdentidade := entity.autoIncrement()
	if !temIdentidade {
		saida, err := exec.ExecContext(ctx, compilada.SQL, compilada.Args...)
		if err != nil {
			return Result{}, ClassifyError(err)
		}
		afetadas := linhasAfetadas(saida)
		return Result{Affected: afetadas}, nil
	}

	d := r.Dialect()
	switch d {
	case MySQL:
		saida, err := exec.ExecContext(ctx, compilada.SQL, compilada.Args...)
		if err != nil {
			return Result{}, ClassifyError(err)
		}
		afetadas := linhasAfetadas(saida)
		id, err := saida.LastInsertId()
		if err != nil {
			// Driver que não sabe informar não invalida o INSERT, que já ocorreu.
			return Result{Affected: afetadas}, nil
		}
		return Result{Affected: afetadas, LastID: id}, nil

	case Postgres:
		sqlComRetorno := compilada.SQL + " RETURNING " + quoteIdentFor(d, identidade.Column)
		var id int64
		if err := r.QueryRowContext(ctx, sqlComRetorno, compilada.Args...).Scan(&id); err != nil {
			return Result{}, ClassifyError(err)
		}
		return Result{Affected: 1, LastID: id}, nil

	case SQLServer:
		// O OUTPUT vai entre a lista de colunas e o VALUES, não no fim: por isso a
		// inserção é feita na compilação, não por concatenação.
		sqlComRetorno := inserirOutputSQLServer(compilada.SQL, quoteIdentFor(d, identidade.Column))
		var id int64
		if err := r.QueryRowContext(ctx, sqlComRetorno, compilada.Args...).Scan(&id); err != nil {
			return Result{}, ClassifyError(err)
		}
		return Result{Affected: 1, LastID: id}, nil

	case Oracle:
		// O Oracle devolve por parâmetro de saída, então o placeholder do RETURNING
		// vem depois dos valores — a numeração continua de onde o INSERT parou.
		var id int64
		posicao := len(compilada.Args) + 1
		sqlComRetorno := compilada.SQL + " RETURNING " + quoteIdentFor(d, identidade.Column) +
			" INTO " + placeholderFor(d, posicao)
		args := append(append([]any{}, compilada.Args...), sql.Out{Dest: &id})
		saida, err := exec.ExecContext(ctx, sqlComRetorno, args...)
		if err != nil {
			return Result{}, ClassifyError(err)
		}
		afetadas := linhasAfetadas(saida)
		return Result{Affected: afetadas, LastID: id}, nil
	}

	saida, err := exec.ExecContext(ctx, compilada.SQL, compilada.Args...)
	if err != nil {
		return Result{}, ClassifyError(err)
	}
	afetadas := linhasAfetadas(saida)
	return Result{Affected: afetadas}, nil
}

// inserirOutputSQLServer põe a cláusula OUTPUT antes do VALUES.
//
// A sintaxe do SQL Server exige essa posição:
//
//	INSERT INTO t (a, b) OUTPUT INSERTED.[id] VALUES (@p1, @p2)
func inserirOutputSQLServer(sqlInsert, colunaCitada string) string {
	const marca = ") VALUES "
	posicao := indiceDe(sqlInsert, marca)
	if posicao < 0 {
		return sqlInsert
	}
	corte := posicao + 1 // depois do ')'
	return sqlInsert[:corte] + " OUTPUT INSERTED." + colunaCitada + sqlInsert[corte:]
}

// indiceDe existe para não importar strings só por isto e para deixar explícito
// que a busca é da PRIMEIRA ocorrência: o ") VALUES " que fecha a lista de
// colunas vem antes de qualquer outro no comando.
func indiceDe(texto, alvo string) int {
	for i := 0; i+len(alvo) <= len(texto); i++ {
		if texto[i:i+len(alvo)] == alvo {
			return i
		}
	}
	return -1
}

// linhasAfetadas lê a contagem de linhas de um comando que JÁ foi executado com
// sucesso.
//
// O erro é engolido aqui, num lugar só, e de propósito: RowsAffected é opcional no
// contrato do database/sql, e driver que não sabe informar não desfaz a escrita — o
// comando aconteceu. Devolver erro faria o chamador tratar como falha um UPDATE que
// gravou. Ficam os dois casos claros para quem usa: a escrita falhou → err != nil; a
// escrita ocorreu e o driver não contou → Affected 0.
func linhasAfetadas(saida sql.Result) int64 {
	afetadas, err := saida.RowsAffected()
	if err != nil {
		return 0
	}
	return afetadas
}
