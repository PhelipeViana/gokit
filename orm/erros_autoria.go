package orm

// Erros de autoria: o desenvolvedor escreveu uma pesquisa ou uma escrita que não
// faz sentido. São diferentes dos erros de banco — não há o que classificar, há
// o que corrigir no código. Por isso a mensagem diz o que fazer.

import (
	"errors"
	"fmt"
)

// ErrSemWhere protege contra o acidente clássico: UPDATE ou DELETE sem WHERE,
// que atinge a tabela inteira. Quem quer mesmo a tabela inteira diz isso em voz
// alta com .Todas().
var ErrSemWhere = errors.New("orm: escrita sem Where atingiria a tabela inteira")

func errSemChavePrimaria(entidade string) error {
	return fmt.Errorf("orm: a entidade %s não declara chave primária; marque a coluna com .PrimaryKey() na migration", entidade)
}

func errChaveIncompleta(entidade string, esperados, recebidos int, colunas string) error {
	return fmt.Errorf("orm: a chave primária de %s tem %d coluna(s) (%s) e foram informados %d valor(es)",
		entidade, esperados, colunas, recebidos)
}

func errSemValores(operacao string) error {
	return fmt.Errorf("orm: %s sem valores; informe ao menos uma coluna", operacao)
}

func errColunaDeOutraEntidade(coluna, entidade string) error {
	return fmt.Errorf("orm: a coluna %s não pertence à entidade %s", coluna, entidade)
}

func errSemWhere(operacao string) error {
	return fmt.Errorf("%w — use .Where(...) para restringir, ou .Todas() se a intenção é o %s completo",
		ErrSemWhere, operacao)
}

func errRunnerSemEscrita() error {
	return errors.New("orm: a conexão ativa não implementa ExecContext; escrita exige um Runner que saiba escrever")
}

// ErrSavepointInvalido protege a montagem do comando SAVEPOINT, que não aceita
// parâmetro e por isso concatena o nome no SQL.
var ErrSavepointInvalido = errors.New("orm: nome de savepoint inválido; use letras, dígitos e sublinhado, começando por letra")

// ErrProjecaoAgregada é devolvido pelo Get quando a projeção tem agregação ou
// agrupamento: o resultado não tem a forma da linha da entidade. Use Rows.
var ErrProjecaoAgregada = errors.New("orm: projeção com agregação ou GROUP BY não cabe na linha da entidade; use .Rows(ctx) em vez de .Get(ctx)")

// ErrTravaComLimiteNoOracle sinaliza a única combinação que a trava não suporta.
//
// No Oracle a limitação de linhas é montada como subconsulta, e FOR UPDATE não se
// aplica a subconsulta — o banco recusa com ORA-02014. Nos outros três funciona.
// A saída é travar a linha pela chave, que é o caso real de quem quer travar:
// Lock() com Where na chave, sem Limit.
var ErrTravaComLimiteNoOracle = errors.New("orm: no Oracle, Lock() não pode ser combinado com Limit/Offset (ORA-02014); restrinja pela chave no Where")
