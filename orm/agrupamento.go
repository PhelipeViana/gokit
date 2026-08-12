package orm

// Agregação agrupada: GROUP BY, HAVING e agregação na projeção.
//
// Até aqui a agregação era só escalar — Sum/Avg/Min/Max devolviam um número da
// tabela inteira. O que faltava é a pergunta que todo relatório faz: "quanto por
// cidade", "quantos por status". Sem isso, quem precisasse disso escreveria Raw,
// e Raw é por dialeto.
//
// A projeção agrupada não cabe no tipo da linha da entidade (um SELECT de
// cidade_id + SUM(saldo) não é um UsersRow), então o retorno é []Record — linha
// genérica chaveada pelo nome da coluna em minúsculas.

import (
	"context"
	"strings"
)

// Aggregation é uma função de agregação aplicada a uma coluna. Implementa Column,
// então serve na projeção (Select), no HAVING e no OrderBy — os três lugares onde
// o SQL aceita agregação.
type Aggregation struct {
	fn      string
	col     Field
	alias   string
	estrela bool // COUNT(*) não tem coluna
}

// columnField faz a Aggregation valer como Column. O Field devolvido carrega o
// nome de saída como Column para que o scanner ache o valor pelo alias.
func (a Aggregation) columnField() Field {
	campo := a.col
	campo.Column = a.NomeDeSaida()
	return campo
}

// NomeDeSaida é o alias da coluna no resultado. Sem As explícito, o nome é
// derivado da função e da coluna — previsível, e é a chave do Record.
func (a Aggregation) NomeDeSaida() string {
	if a.alias != "" {
		return a.alias
	}
	if a.estrela {
		return "count"
	}
	return strings.ToLower(a.fn) + "_" + a.col.Column
}

// As nomeia a coluna de saída.
func (a Aggregation) As(alias string) Aggregation {
	a.alias = alias
	return a
}

// expressao compila a agregação em si, sem o alias.
func (a Aggregation) expressao(ctx *compileCtx) string {
	if a.estrela {
		return a.fn + "(*)"
	}
	return a.fn + "(" + ctx.col(a.col) + ")"
}

// Construtores. CountAll é COUNT(*); Count(col) conta valores não nulos, que é
// pergunta diferente e vale ter separada.
func CountAll() Aggregation        { return Aggregation{fn: "COUNT", estrela: true} }
func Count(col Column) Aggregation { return Aggregation{fn: "COUNT", col: col.columnField()} }
func Sum(col Column) Aggregation   { return Aggregation{fn: "SUM", col: col.columnField()} }
func Avg(col Column) Aggregation   { return Aggregation{fn: "AVG", col: col.columnField()} }
func Min(col Column) Aggregation   { return Aggregation{fn: "MIN", col: col.columnField()} }
func Max(col Column) Aggregation   { return Aggregation{fn: "MAX", col: col.columnField()} }

// Operadores de comparação para o HAVING. Ficam aqui, e não nos FilterField, por
// serem sobre o resultado da agregação e não sobre a coluna.
func (a Aggregation) Equal(v any) Expression       { return aggCondition(a, Equal, v) }
func (a Aggregation) NotEqual(v any) Expression    { return aggCondition(a, NotEqual, v) }
func (a Aggregation) GreaterThan(v any) Expression { return aggCondition(a, GreaterThan, v) }
func (a Aggregation) GreaterOrEqual(v any) Expression {
	return aggCondition(a, GreaterOrEqual, v)
}
func (a Aggregation) LessThan(v any) Expression    { return aggCondition(a, LessThan, v) }
func (a Aggregation) LessOrEqual(v any) Expression { return aggCondition(a, LessOrEqual, v) }

// havingCondition é uma condição cuja esquerda é agregação, não coluna.
type havingCondition struct {
	agg   Aggregation
	op    Operator
	valor any
}

func (havingCondition) expression() {}

// active é sempre verdadeiro: uma condição de HAVING é escrita à mão com um
// valor concreto, diferente do Filter dinâmico, que nasce inativo quando o
// valor chega vazio.
func (havingCondition) active() bool { return true }

func aggCondition(a Aggregation, op Operator, v any) Expression {
	return havingCondition{agg: a, op: op, valor: v}
}

// GroupBy agrupa o resultado. As colunas do GROUP BY entram automaticamente na
// projeção quando ela não foi declarada — agrupar sem trazer a chave do grupo
// produz números sem rótulo, o que nunca é o que se quer.
func (q Query[T]) GroupBy(cols ...Column) Query[T] {
	next := q.clone()
	for _, col := range cols {
		next.groups = append(next.groups, col.columnField())
	}
	return next
}

func (m Model[T]) GroupBy(cols ...Column) Query[T] { return m.All().GroupBy(cols...) }

// Having filtra os grupos. Recebe as mesmas combinações do Where — And, Or e
// condições de agregação — porque a gramática é a mesma; só o momento em que o
// banco aplica é outro.
func (q Query[T]) Having(expressions ...Expression) Query[T] {
	next := q.clone()
	for _, expression := range expressions {
		next.havings = append(next.havings, item{expr: expression})
	}
	return next
}

// Rows executa a pesquisa agrupada e devolve linhas genéricas.
//
// Não usa o Scanner da entidade de propósito: a projeção agrupada tem forma
// própria (chave do grupo + agregações), e forçá-la no tipo da linha da entidade
// mentiria sobre o que voltou.
func (q Query[T]) Rows(ctx context.Context) ([]Record, error) {
	return q.RowsWith(ctx, nil)
}

func (q Query[T]) RowsWith(ctx context.Context, r Runner) ([]Record, error) {
	alvo, err := resolveRunner(r)
	if err != nil {
		return nil, err
	}
	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: alvo.Dialect(), Schema: alvo.Schema()})
	if err != nil {
		return nil, err
	}
	linhas, err := alvo.QueryContext(ctx, compilada.SQL, compilada.Args...)
	if err != nil {
		return nil, ClassifyError(err)
	}
	defer linhas.Close()

	colunas, err := linhas.Columns()
	if err != nil {
		return nil, err
	}
	saida := []Record{}
	for linhas.Next() {
		destino := make([]any, len(colunas))
		for i := range destino {
			var valor any
			destino[i] = &valor
		}
		if err := linhas.Scan(destino...); err != nil {
			return nil, err
		}
		registro := Record{}
		for i, nome := range colunas {
			ponteiro := destino[i].(*any)
			// A chave sai em minúsculas porque cada banco devolve o nome da coluna
			// com a sua própria caixa — o Oracle em maiúsculas, os outros não.
			registro[strings.ToLower(nome)] = Value{bruto: *ponteiro}
		}
		saida = append(saida, registro)
	}
	return saida, linhas.Err()
}

func (m Model[T]) Rows(ctx context.Context) ([]Record, error) { return m.All().Rows(ctx) }

// projecaoAgrupada monta a lista do SELECT quando há agregação ou agrupamento.
// Devolve vazio quando não é o caso, deixando a compilação normal seguir.
func (q Query[T]) projecaoAgrupada(ctx *compileCtx) []string {
	itens := q.aggSelects
	if len(itens) == 0 {
		if len(q.groups) == 0 {
			return nil
		}
		// GroupBy sem projeção declarada: as chaves do grupo mais a contagem, que
		// é o relatório que se espera por padrão.
		for _, grupo := range q.groups {
			itens = append(itens, grupo)
		}
		itens = append(itens, CountAll())
	}

	partes := make([]string, 0, len(itens))
	for _, item := range itens {
		if agregacao, ok := item.(Aggregation); ok {
			partes = append(partes,
				agregacao.expressao(ctx)+" AS "+quoteIdentFor(ctx.dialect, agregacao.NomeDeSaida()))
			continue
		}
		partes = append(partes, ctx.col(item.columnField()))
	}
	return partes
}

// Acesso ao Record por getter, em vez de asserção de tipo.
//
// O acesso cru — linha["total"].(Value).Float() — tem dois problemas: é ruído, e
// nome de coluna errado devolve nil e a asserção entra em panic, dentro de um
// handler, em produção. Os getters trocam o panic por valor zero.

// Os getters recebem a COLUNA, não o nome dela em texto.
//
// Em nenhum outro lugar da ORM se digita nome de coluna: o Where, o Select e o
// OrderBy todos recebem u.Field.X. Abrir exceção na leitura do resultado
// devolveria à mão o que o resto da autoria evita — erro de digitação que o
// compilador não pega. A agregação funciona igual: guarde-a numa variável e
// use a mesma variável para ler.
//
//	total := orm.Sum(u.Field.Saldo).As("total")
//	linhas, _ := orm.Users.Select(u.Field.CidadeId, total).GroupBy(u.Field.CidadeId).Rows(ctx)
//	linhas[0].Float(total)
func (r Record) Valor(coluna Column) Value {
	return r.PorNome(coluna.columnField().Column)
}

// PorNome é a saída para quando só existe o nome em texto — relatório cujas
// colunas vêm de configuração, por exemplo. É explícita de propósito: quem a usa
// está abrindo mão da checagem do compilador e o nome do método diz isso.
func (r Record) PorNome(coluna string) Value {
	procurado := strings.ToLower(coluna)
	if bruto, tem := r[procurado]; tem {
		return comoValue(bruto)
	}
	// O RowsWith já grava a chave em minúsculas, então a busca direta resolve o
	// caminho normal. A varredura abaixo cobre o Record montado à mão — o tipo é
	// público, e não custa nada num caminho que só roda quando houve erro.
	for chave, bruto := range r {
		if strings.EqualFold(chave, procurado) {
			return comoValue(bruto)
		}
	}
	return Value{}
}

func comoValue(bruto any) Value {
	if valor, ok := bruto.(Value); ok {
		return valor
	}
	return Value{bruto: bruto}
}

// Has diz se a coluna veio no resultado, para quando a ausência importa —
// diferente de ter vindo nula.
func (r Record) Has(coluna Column) bool {
	procurado := strings.ToLower(coluna.columnField().Column)
	if _, tem := r[procurado]; tem {
		return true
	}
	for chave := range r {
		if strings.EqualFold(chave, procurado) {
			return true
		}
	}
	return false
}

func (r Record) Int(coluna Column) int64     { return r.Valor(coluna).Int() }
func (r Record) Float(coluna Column) float64 { return r.Valor(coluna).Float() }
func (r Record) Text(coluna Column) string   { return r.Valor(coluna).Text() }
func (r Record) Bool(coluna Column) bool     { return r.Valor(coluna).Bool() }
func (r Record) IsNull(coluna Column) bool   { return r.Valor(coluna).IsNull() }
