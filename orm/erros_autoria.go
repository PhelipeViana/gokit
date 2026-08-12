package orm

// Erros de autoria: o desenvolvedor escreveu uma pesquisa ou uma escrita que não
// faz sentido. São diferentes dos erros de banco — não há o que classificar, há
// o que corrigir no código. Por isso a mensagem diz o que fazer.

import (
	"errors"
	"fmt"
)

// ErrMissingWhere protege contra o acidente clássico: UPDATE ou DELETE sem WHERE,
// que atinge a tabela inteira. Quem quer mesmo a tabela inteira diz isso em voz
// alta com .Unrestricted().
var ErrMissingWhere = errors.New("orm: escrita sem Where atingiria a tabela inteira")

func errSemChavePrimaria(entidade string) error {
	return fmt.Errorf("orm: a entidade %s não declara chave primária; marque a coluna com .PrimaryKey() na migration", entidade)
}

func errChaveIncompleta(entidade string, esperados, recebidos int, colunas string) error {
	return fmt.Errorf("orm: a chave primária de %s tem %d coluna(s) (%s) e foram informados %d valor(es)",
		entidade, esperados, colunas, recebidos)
}

func errExpressaoSemArgumento(fn string) error {
	return fmt.Errorf("orm: a expressão %s precisa de ao menos uma coluna", fn)
}

func errExpressaoUmArgumento(fn string, recebidos int) error {
	return fmt.Errorf("orm: a expressão %s recebe uma coluna só e recebeu %d", fn, recebidos)
}

func errDialetoNaoSuportado(d Dialect) error {
	return fmt.Errorf("dialeto %q ainda não suportado", d)
}

// errChaveDeConflitoSemValor cobre o Upsert em que a chave não está entre os
// valores. É de onde sai a comparação com a linha existente: sem o valor, o
// comando não tem com o que comparar e o banco recusaria — ou, no MySQL, gravaria
// sem checar nada.
func errChaveDeConflitoSemValor(coluna, operacao string) error {
	return fmt.Errorf("orm: %s exige a coluna %s nos valores, porque é ela que identifica a linha existente",
		operacao, coluna)
}

// errUpsertSemAtualizacao cobre o Upsert em que só a chave foi informada: não há
// coluna para atualizar. Aceitar isso em silêncio seria um Upsert que nunca
// atualiza nada — quem quer esse comportamento pede InsertIgnore pelo nome.
func errUpsertSemAtualizacao(chave string) error {
	return fmt.Errorf("orm: Upsert só recebeu a chave de conflito (%s) e não sobrou coluna para atualizar; informe as colunas a gravar, ou use InsertIgnore se a intenção é preservar a linha existente",
		chave)
}

func errSemValores(operacao string) error {
	return fmt.Errorf("orm: %s sem valores; informe ao menos uma coluna", operacao)
}

func errColunaDeOutraEntidade(coluna, entidade string) error {
	return fmt.Errorf("orm: a coluna %s não pertence à entidade %s", coluna, entidade)
}

func errSemWhere(operacao string) error {
	return fmt.Errorf("%w — use .Where(...) para restringir, ou .Unrestricted() se a intenção é o %s completo",
		ErrMissingWhere, operacao)
}

func errRunnerSemEscrita() error {
	return errors.New("orm: a conexão ativa não implementa ExecContext; escrita exige um Runner que saiba escrever")
}

// ErrInvalidSavepoint protege a montagem do comando SAVEPOINT, que não aceita
// parâmetro e por isso concatena o nome no SQL.
var ErrInvalidSavepoint = errors.New("orm: nome de savepoint inválido; use letras, dígitos e sublinhado, começando por letra")

// ErrAggregateProjection é devolvido pelo Get quando a projeção tem agregação,
// expressão ou agrupamento: o resultado não tem a forma da linha da entidade.
// Use Rows.
var ErrAggregateProjection = errors.New("orm: projeção com agregação, expressão ou GROUP BY não cabe na linha da entidade; use .Rows(ctx) em vez de .Get(ctx)")

// ErrValueInGroupBy sinaliza a única combinação que as expressões não
// suportam: agrupar por uma expressão que carrega valor.
//
// Os quatro bancos exigem que a expressão do GROUP BY seja a mesma da projeção e
// comparam pelo TEXTO. Como cada valor vira o seu próprio placeholder, a mesma
// expressão escrita nos dois lugares sai com marcadores diferentes e o banco a
// trata como duas — recusando com ORA-00979, 1055 ou "not contained in an
// aggregate function or the GROUP BY clause".
//
// Inlinear o valor no texto resolveria no banco e abriria injeção; falhar aqui é
// a escolha. Na prática o agrupamento que se quer não tem valor (YearOf, Lower,
// Trim), e o NULL já forma grupo próprio sem precisar de Coalesce.
var ErrValueInGroupBy = errors.New("orm: GROUP BY por expressão com valor não é aceito pelos bancos (a projeção e o agrupamento sairiam com placeholders diferentes); agrupe por expressão sem valor — YearOf, Lower, Trim — lembrando que o NULL já forma grupo próprio")

// ErrLockWithLimitOnOracle sinaliza a única combinação que a trava não suporta.
//
// No Oracle a limitação de linhas é montada como subconsulta, e FOR UPDATE não se
// aplica a subconsulta — o banco recusa com ORA-02014. Nos outros três funciona.
// A saída é travar a linha pela chave, que é o caso real de quem quer travar:
// Lock() com Where na chave, sem Limit.
var ErrLockWithLimitOnOracle = errors.New("orm: no Oracle, Lock() não pode ser combinado com Limit/Offset (ORA-02014); restrinja pela chave no Where")
