package orm

// Bloco 5: expressões cross-dialect.
//
// É o bloco que mais elimina Raw, porque é aqui que o dialeto vaza na prática.
// "Compare ignorando a caixa", "junte nome e sobrenome", "agrupe por ano",
// "trate nulo como zero" são perguntas triviais que, sem isto, só saem em SQL
// escrito à mão — e SQL escrito à mão é por banco.
//
// O que cada banco chama de quê:
//
//	                 mysql            postgres          oracle            sqlserver
//	minúsculas       LOWER            LOWER             LOWER             LOWER
//	tamanho          LENGTH           LENGTH            LENGTH            LEN
//	concatenação     CONCAT_WS('')    CONCAT            a || b            CONCAT
//	nulo por outro   COALESCE         COALESCE          COALESCE          COALESCE
//	ano de uma data  YEAR             EXTRACT(YEAR ...) EXTRACT(YEAR ...) YEAR
//	agora            CURRENT_TIMESTAMP em todos
//
// Expr implementa Column. Isso é o que faz o bloco caber sem método novo em
// Where, Select, OrderBy e GroupBy — a mesma unificação do Aggregation do bloco 2:
// quem aceita coluna passa a aceitar expressão de graça.

import (
	"fmt"
	"strings"
)

// funcao identifica a operação. É interna: a autoria usa os construtores.
type funcao string

const (
	fnLower    funcao = "LOWER"
	fnUpper    funcao = "UPPER"
	fnTrim     funcao = "TRIM"
	fnLength   funcao = "LENGTH"
	fnConcat   funcao = "CONCAT"
	fnCoalesce funcao = "COALESCE"
	fnAno      funcao = "YEAR"
	fnMes      funcao = "MONTH"
	fnDia      funcao = "DAY"
	fnNow      funcao = "NOW"
)

// Expr é uma função de banco aplicada a colunas e valores.
//
// Compõe: o argumento de uma expressão pode ser outra expressão, então
// orm.Lower(orm.Concat(f.Nome, f.Sobrenome)) é uma expressão só.
type Expr struct {
	fn    funcao
	args  []argumento
	alias string
}

// argumento é coluna, expressão aninhada ou valor. Valor vira placeholder — nunca
// texto no SQL, igual ao resto do motor.
type argumento struct {
	coluna  Column
	valor   any
	eValor  bool
	eColuna bool
}

// argumentosDe classifica o que a autoria passou. Column entra como coluna (ou
// expressão, que também é Column); qualquer outra coisa é valor.
func argumentosDe(partes []any) []argumento {
	args := make([]argumento, 0, len(partes))
	for _, parte := range partes {
		if coluna, ok := parte.(Column); ok {
			args = append(args, argumento{coluna: coluna, eColuna: true})
			continue
		}
		args = append(args, argumento{valor: parte, eValor: true})
	}
	return args
}

func exprDeColuna(fn funcao, col Column) Expr {
	return Expr{fn: fn, args: []argumento{{coluna: col, eColuna: true}}}
}

// ── Construtores ──

// Lower e Upper resolvem a comparação insensível à caixa de forma igual nos
// quatro. É a saída para o LIKE, que é sensível no Postgres e no Oracle e
// insensível no MySQL e no SQL Server por COLLATION do banco:
//
//	orm.Lower(f.Nome).Contains("ana")   // mesma resposta nos quatro
func Lower(col Column) Expr { return exprDeColuna(fnLower, col) }
func Upper(col Column) Expr { return exprDeColuna(fnUpper, col) }

// Trim remove espaços das duas pontas. Vale mais do que parece em schema legado,
// onde CHAR de tamanho fixo devolve o valor preenchido com espaços.
func Trim(col Column) Expr { return exprDeColuna(fnTrim, col) }

// Length é o número de caracteres. Ressalva do SQL Server: o LEN dele ignora
// espaços à direita, os outros três contam. Combine com Trim quando a diferença
// importar.
func Length(col Column) Expr { return exprDeColuna(fnLength, col) }

// Concat junta colunas e textos.
//
//	orm.Concat(f.Nome, " ", f.Sobrenome)
//
// Nulo conta como texto vazio nos quatro. Três bancos já fazem isso; o MySQL
// devolveria NULL para a linha inteira, e por isso a compilação dele usa
// CONCAT_WS, que pula nulo. Promessa única em vez de comportamento por banco.
func Concat(partes ...any) Expr {
	return Expr{fn: fnConcat, args: argumentosDe(partes)}
}

// Coalesce devolve o primeiro argumento não nulo — o "trate nulo como zero" que
// aparece em todo relatório de soma.
func Coalesce(partes ...any) Expr {
	return Expr{fn: fnCoalesce, args: argumentosDe(partes)}
}

// YearOf, MonthOf e DayOf extraem a parte da data. É o que permite agrupar por ano
// sem Raw:
//
//	orm.Pedidos.Select(orm.YearOf(f.Data), orm.CountAll()).GroupBy(orm.YearOf(f.Data)).Rows(ctx)
func YearOf(col Column) Expr  { return exprDeColuna(fnAno, col) }
func MonthOf(col Column) Expr { return exprDeColuna(fnMes, col) }
func DayOf(col Column) Expr   { return exprDeColuna(fnDia, col) }

// Now é o instante do BANCO, não o da aplicação.
//
// A diferença importa quando o relógio da máquina que roda o Go e o do banco não
// são o mesmo — o caso normal em produção. Comparar vencimento com Now() deixa
// a decisão inteira dentro da consulta.
//
// Compila para CURRENT_TIMESTAMP nos quatro. No Oracle isso é o fuso da SESSÃO,
// diferente do SYSDATE, que é o do servidor; a ORM escolheu o padrão em vez do
// atalho de cada banco.
func Now() Expr { return Expr{fn: fnNow} }

// temValor diz se a expressão vincula algum valor, em qualquer nível.
//
// Serve para a guarda do GROUP BY: os quatro bancos exigem que a expressão do
// agrupamento seja a MESMA da projeção, e comparam pelo texto. Como cada valor
// gera o seu próprio placeholder, COALESCE(col, $1) na projeção e COALESCE(col, $3)
// no GROUP BY são expressões diferentes para o banco — que recusa (ORA-00979,
// 1055 no MySQL, "not contained in ... GROUP BY" no SQL Server).
func (e Expr) temValor() bool {
	for _, arg := range e.args {
		if arg.eValor {
			return true
		}
		if aninhada, ok := arg.coluna.(Expr); ok && aninhada.temValor() {
			return true
		}
	}
	return false
}

// As nomeia a coluna de saída na projeção.
func (e Expr) As(alias string) Expr {
	e.alias = alias
	return e
}

// OutputName é o alias no resultado, e a chave de leitura no Record. Sem As, sai
// da função e da primeira coluna — previsível, como no Aggregation.
func (e Expr) OutputName() string {
	if e.alias != "" {
		return e.alias
	}
	base := strings.ToLower(string(e.fn))
	for _, arg := range e.args {
		if !arg.eColuna {
			continue
		}
		return base + "_" + arg.coluna.columnField().Column
	}
	return base
}

// columnField faz Expr valer como Column. O Field devolvido carrega o nome de
// saída, para que a leitura do Record ache o valor pelo alias.
func (e Expr) columnField() Field {
	campo := Field{}
	for _, arg := range e.args {
		if arg.eColuna {
			campo = arg.coluna.columnField()
			break
		}
	}
	campo.Column = e.OutputName()
	return campo
}

// ── Compilação ──

// expressao rende a função no dialeto ativo. Os valores viram placeholder na
// ordem em que aparecem no texto, então o compilador precisa chamar isto na mesma
// ordem em que monta as cláusulas — ver o comentário de Compile.
func (e Expr) expressao(ctx *compileCtx) (string, error) {
	if e.fn == fnNow {
		return "CURRENT_TIMESTAMP", nil
	}
	if len(e.args) == 0 {
		return "", errExpressaoSemArgumento(string(e.fn))
	}

	partes := make([]string, 0, len(e.args))
	for _, arg := range e.args {
		if arg.eValor {
			marca := ctx.bind(arg.valor)
			// O concat do Postgres aceita qualquer tipo, e por isso o banco não
			// consegue deduzir o do parâmetro — recusa com "could not determine data
			// type of parameter". A conversão explícita resolve, e só ele precisa: os
			// outros três deduzem pelo contexto.
			if e.fn == fnConcat && ctx.dialect == Postgres {
				marca = "CAST(" + marca + " AS TEXT)"
			}
			partes = append(partes, marca)
			continue
		}
		texto, err := ctx.expr(arg.coluna)
		if err != nil {
			return "", err
		}
		partes = append(partes, texto)
	}

	switch e.fn {
	case fnLower, fnUpper, fnTrim:
		if len(partes) != 1 {
			return "", errExpressaoUmArgumento(string(e.fn), len(partes))
		}
		return string(e.fn) + "(" + partes[0] + ")", nil

	case fnLength:
		if len(partes) != 1 {
			return "", errExpressaoUmArgumento(string(e.fn), len(partes))
		}
		// O SQL Server não tem LENGTH.
		if ctx.dialect == SQLServer {
			return "LEN(" + partes[0] + ")", nil
		}
		return "LENGTH(" + partes[0] + ")", nil

	case fnConcat:
		return concatenacao(ctx.dialect, partes)

	case fnCoalesce:
		if len(partes) < 2 {
			return "", fmt.Errorf("orm: Coalesce precisa de ao menos dois argumentos — a coluna e o que usar quando ela for nula")
		}
		return "COALESCE(" + strings.Join(partes, ", ") + ")", nil

	case fnAno, fnMes, fnDia:
		if len(partes) != 1 {
			return "", errExpressaoUmArgumento(string(e.fn), len(partes))
		}
		return parteDeData(ctx.dialect, e.fn, partes[0]), nil
	}
	return "", fmt.Errorf("orm: expressão %q ainda não compilada", e.fn)
}

// concatenacao é a maior divergência do bloco: quatro formas para a mesma
// operação, e duas delas com tratamento de nulo diferente.
func concatenacao(d Dialect, partes []string) (string, error) {
	if len(partes) < 2 {
		return "", fmt.Errorf("orm: Concat precisa de ao menos duas partes")
	}
	switch d {
	case Oracle:
		// O CONCAT do Oracle aceita só dois argumentos; o operador não tem limite.
		return "(" + strings.Join(partes, " || ") + ")", nil
	case MySQL:
		// CONCAT_WS com separador vazio pula os nulos, alinhando o MySQL aos outros
		// três. O CONCAT dele devolveria NULL se qualquer parte fosse nula.
		return "CONCAT_WS('', " + strings.Join(partes, ", ") + ")", nil
	default: // postgres, sqlserver — CONCAT já trata nulo como vazio
		return "CONCAT(" + strings.Join(partes, ", ") + ")", nil
	}
}

// parteDeData: dois bancos têm função dedicada, dois usam o EXTRACT do padrão.
func parteDeData(d Dialect, fn funcao, coluna string) string {
	switch d {
	case MySQL, SQLServer:
		return string(fn) + "(" + coluna + ")"
	default: // postgres, oracle
		return "EXTRACT(" + string(fn) + " FROM " + coluna + ")"
	}
}

// ── Comparadores: é o que põe a expressão dentro do Where ──
//
// Ficam aqui, e não nos FilterField gerados, por serem sobre o resultado da
// expressão e não sobre a coluna.

func (e Expr) Equal(v any) Expression          { return exprCondicao(e, Equal, v) }
func (e Expr) NotEqual(v any) Expression       { return exprCondicao(e, NotEqual, v) }
func (e Expr) GreaterThan(v any) Expression    { return exprCondicao(e, GreaterThan, v) }
func (e Expr) GreaterOrEqual(v any) Expression { return exprCondicao(e, GreaterOrEqual, v) }
func (e Expr) LessThan(v any) Expression       { return exprCondicao(e, LessThan, v) }
func (e Expr) LessOrEqual(v any) Expression    { return exprCondicao(e, LessOrEqual, v) }

// Contains, StartsWith e EndsWith sobre expressão são o par natural do Lower: é
// aqui que a busca insensível à caixa fica igual nos quatro bancos.
func (e Expr) Contains(v string) Expression   { return exprCondicao(e, Contains, v) }
func (e Expr) StartsWith(v string) Expression { return exprCondicao(e, StartsWith, v) }
func (e Expr) EndsWith(v string) Expression   { return exprCondicao(e, EndsWith, v) }

func (e Expr) In(vs ...any) Expression { return exprCondicao(e, In, vs...) }
func (e Expr) Between(inicio, fim any) Expression {
	return exprCondicao(e, Between, inicio, fim)
}
func (e Expr) IsNull() Expression    { return exprCondicao(e, IsNull) }
func (e Expr) IsNotNull() Expression { return exprCondicao(e, IsNotNull) }

// exprCondition é uma condição cuja esquerda é expressão, não coluna.
type exprCondition struct {
	expr   Expr
	op     Operator
	values []any
}

func (exprCondition) expression() {}

// active segue a regra do Condition: sem valor não filtra, o que faz o filtro
// dinâmico funcionar igual com expressão.
func (c exprCondition) active() bool { return conditionActive(c.op, c.values) }

func exprCondicao(e Expr, op Operator, values ...any) Expression {
	return exprCondition{expr: e, op: op, values: values}
}

// compileExprCondition reaproveita a renderização do Condition: monta o lado
// esquerdo como expressão e delega o resto, para que operador novo não precise ser
// implementado duas vezes.
func compileExprCondition(c exprCondition, ctx *compileCtx) (string, error) {
	esquerda, err := c.expr.expressao(ctx)
	if err != nil {
		return "", err
	}
	return compileComparacao(esquerda, c.op, c.values, ctx)
}
