package orm

// Bloco 3 (parte 1): comparação entre colunas e subconsulta.
//
// São os dois casos que mais empurravam para Raw sem exigir JOIN:
//
//	WHERE "saldo" > "limite"                          → WhereColumn
//	WHERE "cidade_id" IN (SELECT "id" FROM "cidades" WHERE ...)  → In com subconsulta
//
// O JOIN fica para a parte 2 porque ele exige qualificar toda coluna com a
// tabela, e isso mexe na compilação de quem já funciona. Estes dois não mexem em
// nada: são expressões novas no mesmo switch.

import "fmt"

// ── Comparação entre duas colunas da mesma tabela ──

// columnCondition guarda as Column, não os Field resolvidos: é o que permite
// comparar expressão com coluna, ou com outra expressão — WhereColumn(f.Vencimento,
// orm.LessThan, orm.Now()) compara a coluna com o relógio do banco.
type columnCondition struct {
	esquerda Column
	op       Operator
	direita  Column
}

func (columnCondition) expression()  {}
func (columnCondition) active() bool { return true }

// WhereColumn compara dois lados entre si, em vez de coluna com valor. Cada lado
// é uma coluna ou uma expressão do bloco 5.
//
// É o caso do "saldo acima do limite" ou "atualizado depois de criado", que sem
// isso só saía em SQL escrito à mão.
func WhereColumn(esquerda Column, op Operator, direita Column) Expression {
	return columnCondition{esquerda: esquerda, op: op, direita: direita}
}

func compileColumnCondition(c columnCondition, ctx *compileCtx) (string, error) {
	// Lista explícita em vez de confiar no comparator: ele devolve "=" para
	// operador desconhecido, então um Contains entre colunas passaria como
	// igualdade — SQL válido com semântica errada.
	switch c.op {
	case Equal, NotEqual, GreaterThan, GreaterOrEqual, LessThan, LessOrEqual:
	default:
		return "", fmt.Errorf("orm: operador %q não vale entre duas colunas; use igualdade ou comparação de ordem", c.op)
	}
	esquerda, err := ctx.expr(c.esquerda)
	if err != nil {
		return "", err
	}
	direita, err := ctx.expr(c.direita)
	if err != nil {
		return "", err
	}
	return esquerda + " " + comparator(c.op) + " " + direita, nil
}

// ── Subconsulta ──

// Subquery é uma pesquisa usada dentro de outra. Query[T] a satisfaz, então
// qualquer entidade serve de subconsulta sem o tipo da linha aparecer aqui — é o
// que permite guardar a subconsulta numa Expression, que não é genérica.
type Subquery interface {
	compilarSub(ctx *compileCtx, schema string) (string, error)
}

// compilarSub compila a pesquisa como SELECT aninhado, usando o contexto de
// argumentos do PAI. A numeração dos placeholders continua de onde o pai parou —
// no Postgres e no Oracle, começar do $1 de novo ligaria o valor errado.
// Só projeção e WHERE entram: é o que uma subconsulta de IN ou EXISTS precisa.
// Ordem e limite não fazem sentido dentro de IN, e limite dentro de subconsulta
// tem sintaxe diferente em cada banco — deixar de fora agora é melhor que
// entregar quatro comportamentos diferentes.
func (q Query[T]) compilarSub(ctx *compileCtx, schema string) (string, error) {
	where, err := compileItems(q.items, ctx)
	if err != nil {
		return "", err
	}

	// A projeção da subconsulta: o Select explícito, ou a chave primária. Num IN,
	// trazer todas as colunas seria erro de SQL ("operand should contain 1 column").
	colunas := q.selects
	if len(colunas) == 0 {
		colunas = q.Entity.PrimaryKeys()
	}
	if len(colunas) == 0 {
		return "", fmt.Errorf("orm: subconsulta de %s precisa de .Select(coluna) — a entidade não declara chave primária", q.Entity.Name)
	}
	if len(colunas) > 1 {
		return "", fmt.Errorf("orm: subconsulta de %s projeta %d colunas; use .Select() com uma só", q.Entity.Name, len(colunas))
	}

	sql := "SELECT " + quoteIdentFor(ctx.dialect, colunas[0].Column) +
		" FROM " + qualifyFor(ctx.dialect, schema, q.Entity.Name)
	if where != "" {
		sql += " WHERE " + where
	}
	return sql, nil
}

type subqueryCondition struct {
	coluna        Field
	negado        bool
	sub           Subquery
	somenteExiste bool // true = [NOT] EXISTS; false = coluna [NOT] IN
}

func (subqueryCondition) expression()  {}
func (subqueryCondition) active() bool { return true }

// InQuery filtra pela coluna estar no resultado de outra pesquisa.
//
//	orm.Users.Where(orm.InQuery(u.Column.CidadeId,
//	    orm.Cidades.Where(c.Column.Uf.Equal("PR")).Select(c.Column.Id)))
//
// Diferente do eager loading: aqui não se carrega o filho, só se filtra o pai.
func InQuery(coluna Column, sub Subquery) Expression {
	return subqueryCondition{coluna: coluna.columnField(), sub: sub}
}

// NotInQuery é o inverso.
func NotInQuery(coluna Column, sub Subquery) Expression {
	return subqueryCondition{coluna: coluna.columnField(), sub: sub, negado: true}
}

// ExistsQuery filtra pela subconsulta devolver alguma linha.
//
// Diferente do Relation.Exists(): aquele é correlacionado pela FK que o gerador
// conhece; este aceita qualquer pesquisa, inclusive de tabela sem relação
// declarada.
func ExistsQuery(sub Subquery) Expression {
	return subqueryCondition{sub: sub, somenteExiste: true}
}

// NotExistsQuery é o inverso.
func NotExistsQuery(sub Subquery) Expression {
	return subqueryCondition{sub: sub, somenteExiste: true, negado: true}
}

func compileSubquery(c subqueryCondition, ctx *compileCtx) (string, error) {
	if c.sub == nil {
		return "", fmt.Errorf("orm: subconsulta ausente")
	}
	interno, err := c.sub.compilarSub(ctx, ctx.schema)
	if err != nil {
		return "", err
	}
	if c.somenteExiste {
		if c.negado {
			return "NOT EXISTS (" + interno + ")", nil
		}
		return "EXISTS (" + interno + ")", nil
	}
	operador := " IN ("
	if c.negado {
		operador = " NOT IN ("
	}
	return ctx.col(c.coluna) + operador + interno + ")", nil
}

// Model também serve de subconsulta, para que orm.Cidades entre direto sem
// .All() — é o mesmo princípio dos outros terminais promovidos na entidade.
func (m Model[T]) compilarSub(ctx *compileCtx, schema string) (string, error) {
	return m.All().compilarSub(ctx, schema)
}
