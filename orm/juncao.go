package orm

// Bloco 3 (parte 2): JOIN.
//
// É a mudança mais invasiva da ORM até aqui, e o motivo é um detalhe de SQL: sem
// junção, `WHERE "nome" = ?` é inequívoco; com junção, "nome" pode existir nas
// duas tabelas e o banco recusa por ambiguidade. Então suportar JOIN obriga a
// qualificar toda coluna com a tabela.
//
// A qualificação é CONDICIONAL: só acontece quando há junção na pesquisa. Quem
// não usa JOIN continua gerando exatamente o mesmo SQL de antes — é o que permite
// acrescentar isto sem reescrever as 55 leituras que já passam nos quatro bancos.
//
// A junção é declarada pela RELAÇÃO, não por tabela crua. O gerador já conhece a
// FK, então a condição ON sai dele: ninguém escreve `ON a.id = b.a_id` à mão, que
// é onde nasce o erro de junção invertida.

import "fmt"

type tipoJuncao string

const (
	juncaoInterna  tipoJuncao = "JOIN"
	juncaoEsquerda tipoJuncao = "LEFT JOIN"
)

type juncao struct {
	tipo tipoJuncao
	rel  Relation
}

// Join acrescenta uma junção interna pela relação declarada: só as linhas com
// correspondência no destino sobrevivem.
//
// Diferente do With: o With CARREGA o destino em consulta separada e devolve a
// árvore montada; o Join não carrega nada — ele existe para filtrar e ordenar
// pela outra tabela dentro da mesma consulta.
func (q Query[T]) Join(relacoes ...RelationSource) Query[T] {
	return q.juntar(juncaoInterna, relacoes)
}

// LeftJoin preserva as linhas sem correspondência no destino.
func (q Query[T]) LeftJoin(relacoes ...RelationSource) Query[T] {
	return q.juntar(juncaoEsquerda, relacoes)
}

func (q Query[T]) juntar(tipo tipoJuncao, relacoes []RelationSource) Query[T] {
	next := q.clone()
	for _, fonte := range relacoes {
		next.joins = append(next.joins, juncao{tipo: tipo, rel: fonte.relation()})
	}
	return next
}

func (m Model[T]) Join(relacoes ...RelationSource) Query[T] { return m.All().Join(relacoes...) }
func (m Model[T]) LeftJoin(relacoes ...RelationSource) Query[T] {
	return m.All().LeftJoin(relacoes...)
}

// compilarJuncoes monta as cláusulas JOIN ... ON. A condição sai dos metadados da
// relação — a mesma informação que o EXISTS correlacionado já usa.
func (q Query[T]) compilarJuncoes(d Dialect, schema string) (string, error) {
	if len(q.joins) == 0 {
		return "", nil
	}
	saida := ""
	for _, j := range q.joins {
		alvo, chaveAlvo, chavePropria, err := ladosDaJuncao(q.Entity.Name, j.rel)
		if err != nil {
			return "", err
		}
		saida += " " + string(j.tipo) + " " + qualifyFor(d, schema, alvo) +
			" ON " + colunaQualificada(d, q.Entity.Name, chavePropria) +
			" = " + colunaQualificada(d, alvo, chaveAlvo)
	}
	return saida, nil
}

// ladosDaJuncao decide qual coluna de cada lado entra no ON, a partir do tipo da
// relação. Em belongsTo a FK está na própria tabela; em hasMany, no destino.
func ladosDaJuncao(propria string, rel Relation) (alvo, chaveAlvo, chavePropria string, err error) {
	alvo, chaveAlvo, chavePropria = rel.ladosParaJuncao()
	if alvo == "" || chaveAlvo == "" || chavePropria == "" {
		return "", "", "", fmt.Errorf("orm: relação sem metadados de junção; o Join aceita apenas relações geradas em %s.Relation", propria)
	}
	return alvo, chaveAlvo, chavePropria, nil
}

// colunaQualificada devolve tabela.coluna, que é a forma que o JOIN exige.
func colunaQualificada(d Dialect, tabela, coluna string) string {
	return quoteIdentFor(d, tabela) + "." + quoteIdentFor(d, coluna)
}
