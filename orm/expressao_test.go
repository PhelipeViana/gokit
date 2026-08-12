package orm

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// O bloco 5 existe porque cada banco chama a mesma operação de outro jeito. O
// teste é o inventário dessa diferença.
func TestExpressoesCompilamNosQuatroDialetos(t *testing.T) {
	users := entidadeUsers()
	nome := campo(users, "nome")
	nascimento := campo(users, "nascimento")

	casos := []struct {
		titulo   string
		expr     Expr
		esperado map[Dialect]string
	}{
		{"Lower", Lower(nome), map[Dialect]string{
			MySQL:     "LOWER(`nome`)",
			Postgres:  `LOWER("nome")`,
			Oracle:    `LOWER("NOME")`,
			SQLServer: "LOWER([nome])",
		}},
		{"Upper", Upper(nome), map[Dialect]string{
			MySQL:     "UPPER(`nome`)",
			Postgres:  `UPPER("nome")`,
			Oracle:    `UPPER("NOME")`,
			SQLServer: "UPPER([nome])",
		}},
		{"Trim", Trim(nome), map[Dialect]string{
			MySQL:     "TRIM(`nome`)",
			Postgres:  `TRIM("nome")`,
			Oracle:    `TRIM("NOME")`,
			SQLServer: "TRIM([nome])",
		}},
		// O SQL Server não tem LENGTH.
		{"Length", Length(nome), map[Dialect]string{
			MySQL:     "LENGTH(`nome`)",
			Postgres:  `LENGTH("nome")`,
			Oracle:    `LENGTH("NOME")`,
			SQLServer: "LEN([nome])",
		}},
		// Quatro formas: operador no Oracle, CONCAT_WS no MySQL (para pular nulo) e
		// CONCAT nos outros dois.
		{"Concat de duas colunas", Concat(nome, campo(users, "email")), map[Dialect]string{
			MySQL:     "CONCAT_WS('', `nome`, `email`)",
			Postgres:  `CONCAT("nome", "email")`,
			Oracle:    `("NOME" || "EMAIL")`,
			SQLServer: "CONCAT([nome], [email])",
		}},
		{"YearOf", YearOf(nascimento), map[Dialect]string{
			MySQL:     "YEAR(`nascimento`)",
			Postgres:  `EXTRACT(YEAR FROM "nascimento")`,
			Oracle:    `EXTRACT(YEAR FROM "NASCIMENTO")`,
			SQLServer: "YEAR([nascimento])",
		}},
		{"MonthOf", MonthOf(nascimento), map[Dialect]string{
			MySQL:     "MONTH(`nascimento`)",
			Postgres:  `EXTRACT(MONTH FROM "nascimento")`,
			Oracle:    `EXTRACT(MONTH FROM "NASCIMENTO")`,
			SQLServer: "MONTH([nascimento])",
		}},
		// COALESCE é o único que os quatro escrevem igual.
		{"Coalesce", Coalesce(campo(users, "saldo"), 0), map[Dialect]string{
			MySQL:     "COALESCE(`saldo`, ?)",
			Postgres:  `COALESCE("saldo", $1)`,
			Oracle:    `COALESCE("SALDO", :1)`,
			SQLServer: "COALESCE([saldo], @p1)",
		}},
		{"Now", Now(), map[Dialect]string{
			MySQL:     "CURRENT_TIMESTAMP",
			Postgres:  "CURRENT_TIMESTAMP",
			Oracle:    "CURRENT_TIMESTAMP",
			SQLServer: "CURRENT_TIMESTAMP",
		}},
	}

	for _, caso := range casos {
		for dialeto, esperado := range caso.esperado {
			ctx := &compileCtx{dialect: dialeto}
			obtido, err := caso.expr.expressao(ctx)
			if err != nil {
				t.Fatalf("%s/%s: %v", caso.titulo, dialeto, err)
			}
			if obtido != esperado {
				t.Errorf("%s/%s:\n  esperado %s\n  obtido   %s", caso.titulo, dialeto, esperado, obtido)
			}
		}
	}
}

// Expressão dentro de expressão é uma expressão só.
func TestExpressaoAninhada(t *testing.T) {
	users := entidadeUsers()
	e := Lower(Concat(campo(users, "nome"), " ", campo(users, "email")))
	ctx := &compileCtx{dialect: Postgres}
	obtido, err := e.expressao(ctx)
	if err != nil {
		t.Fatal(err)
	}
	esperado := `LOWER(CONCAT("nome", CAST($1 AS TEXT), "email"))`
	if obtido != esperado {
		t.Errorf("esperado %s, obtido %s", esperado, obtido)
	}
	if len(ctx.args) != 1 || ctx.args[0] != " " {
		t.Errorf("o texto literal deveria ter virado argumento: %v", ctx.args)
	}
}

// O valor de dentro da expressão vira placeholder, nunca texto no SQL — a mesma
// regra do resto do motor.
func TestValorDaExpressaoNaoEntraNoSQL(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(Coalesce(campo(users, "saldo"), 0).GreaterThan(100))
	compilada, err := q.Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(compilada.SQL, "100") {
		t.Errorf("o valor entrou no texto do SQL: %s", compilada.SQL)
	}
	if len(compilada.Args) != 2 || compilada.Args[0] != 0 || compilada.Args[1] != 100 {
		t.Errorf("argumentos inesperados: %v", compilada.Args)
	}
}

// Este é o risco real do bloco: com expressões que vinculam valor em mais de uma
// cláusula, a ordem de COMPILAÇÃO precisa ser a ordem do TEXTO. Errar isso liga o
// valor de uma cláusula ao placeholder de outra e o SQL continua válido.
func TestPlaceholdersSeguemAOrdemDoTexto(t *testing.T) {
	users := entidadeUsers()
	saldo := campo(users, "saldo")
	q := modelo(users).
		Select(Coalesce(saldo, 1).As("na_projecao")).
		Where(condition(campo(users, "id"), GreaterThan, 2)).
		GroupBy(Lower(campo(users, "nome"))). // sem valor: ver ErrValueInGroupBy
		Having(CountAll().GreaterThan(3)).
		OrderBy(Coalesce(saldo, 4))

	compilada, err := q.Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	// Os valores 1..4 estão em cláusulas distintas, na ordem em que aparecem no
	// texto: projeção, WHERE, HAVING, ORDER BY.
	esperados := []any{1, 2, 3, 4}
	if len(compilada.Args) != len(esperados) {
		t.Fatalf("esperado %d argumentos, obtido %v — SQL: %s", len(esperados), compilada.Args, compilada.SQL)
	}
	for i, esperado := range esperados {
		if compilada.Args[i] != esperado {
			t.Errorf("argumento %d = %v, esperado %v (SQL: %s)", i+1, compilada.Args[i], esperado, compilada.SQL)
		}
	}
	// E a checagem que fecha: cada placeholder aparece no texto na mesma ordem.
	posicoes := []int{
		strings.Index(compilada.SQL, "$1"),
		strings.Index(compilada.SQL, "$2"),
		strings.Index(compilada.SQL, "$3"),
		strings.Index(compilada.SQL, "$4"),
	}
	for i := 1; i < len(posicoes); i++ {
		if posicoes[i] <= posicoes[i-1] {
			t.Fatalf("placeholder $%d aparece antes de $%d no texto: %s", i+1, i, compilada.SQL)
		}
	}
}

// Sem expressão em nenhuma cláusula, o SQL tem de sair igual ao que sempre foi: a
// reordenação da compilação não pode mudar o texto de quem já funciona.
func TestReordenacaoNaoMudaOSQLSemExpressao(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).
		Where(condition(campo(users, "id"), GreaterThan, 2)).
		GroupBy(campo(users, "cidade_id")).
		Having(CountAll().GreaterThan(1)).
		OrderBy(campo(users, "id")).
		Limit(5)

	casos := map[Dialect]string{
		Postgres: `SELECT "cidade_id", COUNT(*) AS "count" FROM "users" WHERE "id" > $1 ` +
			`GROUP BY "cidade_id" HAVING COUNT(*) > $2 ORDER BY "id" ASC LIMIT 5`,
		MySQL: "SELECT `cidade_id`, COUNT(*) AS `count` FROM `users` WHERE `id` > ? " +
			"GROUP BY `cidade_id` HAVING COUNT(*) > ? ORDER BY `id` ASC LIMIT 5",
	}
	for dialeto, esperado := range casos {
		compilada, err := q.Explain(OpSelect, dialeto)
		if err != nil {
			t.Fatal(err)
		}
		if compilada.SQL != esperado {
			t.Errorf("%s:\n  esperado %s\n  obtido   %s", dialeto, esperado, compilada.SQL)
		}
	}
}

// A expressão entra nas quatro cláusulas sem método novo em nenhuma delas — é o
// que o Column dá de graça.
func TestExpressaoEntraNasQuatroClausulas(t *testing.T) {
	users := entidadeUsers()
	nome := campo(users, "nome")

	where, err := modelo(users).Where(Lower(nome).Contains("ana")).Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(where.SQL, `WHERE LOWER("nome") LIKE $1`) {
		t.Errorf("Where: %s", where.SQL)
	}
	if len(where.Args) != 1 || where.Args[0] != "%ana%" {
		t.Errorf("o LIKE deveria receber o padrão com os curingas: %v", where.Args)
	}

	ordem, err := modelo(users).OrderByDesc(Lower(nome)).Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordem.SQL, `ORDER BY LOWER("nome") DESC`) {
		t.Errorf("OrderBy: %s", ordem.SQL)
	}

	grupo, err := modelo(users).GroupBy(YearOf(campo(users, "nascimento"))).Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(grupo.SQL, `GROUP BY EXTRACT(YEAR FROM "nascimento")`) {
		t.Errorf("GroupBy: %s", grupo.SQL)
	}

	projecao, err := modelo(users).Select(Lower(nome)).Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(projecao.SQL, `LOWER("nome") AS "lower_nome"`) {
		t.Errorf("Select: %s", projecao.SQL)
	}
}

// O alias é a chave de leitura no Record, então precisa ser previsível sem As e
// respeitar o As quando houver.
func TestNomeDeSaidaDaExpressao(t *testing.T) {
	users := entidadeUsers()
	casos := map[string]Expr{
		"lower_nome":      Lower(campo(users, "nome")),
		"year_nascimento": YearOf(campo(users, "nascimento")),
		"now":             Now(),
		"apelidada":       Lower(campo(users, "nome")).As("apelidada"),
	}
	for esperado, expressao := range casos {
		if obtido := expressao.OutputName(); obtido != esperado {
			t.Errorf("esperado %q, obtido %q", esperado, obtido)
		}
	}
}

// Projeção com expressão não cabe no tipo da linha da entidade — mesma regra da
// agregação, e o Get precisa dizer isso em vez de descartar a expressão.
func TestGetRecusaProjecaoComExpressao(t *testing.T) {
	users := entidadeUsers()
	_, err := modelo(users).Select(Lower(campo(users, "nome"))).Get(nil)
	if err == nil {
		t.Fatal("Get com projeção de expressão deveria falhar")
	}
	if !strings.Contains(err.Error(), "Rows") {
		t.Errorf("a mensagem deveria apontar o Rows: %v", err)
	}
}

// WhereColumn passa a aceitar expressão nos dois lados: é o que permite comparar
// coluna com o relógio do BANCO em vez do da aplicação.
func TestWhereColumnAceitaExpressao(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(WhereColumn(campo(users, "nascimento"), LessThan, Now()))
	compilada, err := q.Explain(OpSelect, Oracle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, `WHERE "NASCIMENTO" < CURRENT_TIMESTAMP`) {
		t.Errorf("SQL inesperado: %s", compilada.SQL)
	}
}

// ── Avaliação em memória ──
//
// Sem isto o bloco 5 furaria o Matches: uma regra guardada numa variável deixaria
// de valer no Go, e a camada de regras voltaria a escrever a condição duas vezes.

type linhaDeTeste struct {
	Id         int64
	Nome       string
	Email      string
	Saldo      *float64
	Nascimento *time.Time
}

func TestMatchesDeExpressao(t *testing.T) {
	users := entidadeUsers()
	nascimento := time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC)
	linha := linhaDeTeste{Id: 1, Nome: "  ANA Maria  ", Email: "ana@x.com", Nascimento: &nascimento}

	casos := []struct {
		titulo   string
		expr     Expression
		esperado bool
	}{
		{"Lower casa insensível à caixa", Lower(campo(users, "nome")).Contains("ana maria"), true},
		{"Lower não casa o que não existe", Lower(campo(users, "nome")).Contains("joão"), false},
		{"Upper", Upper(campo(users, "email")).Contains("ANA@"), true},
		{"Trim tira as pontas", Trim(campo(users, "nome")).StartsWith("ANA"), true},
		{"Length conta caracteres", Length(campo(users, "nome")).Equal(13), true},
		{"YearOf", YearOf(campo(users, "nascimento")).Equal(1990), true},
		{"MonthOf", MonthOf(campo(users, "nascimento")).Equal(5), true},
		{"DayOf", DayOf(campo(users, "nascimento")).Equal(17), true},
		{"Coalesce troca o nulo", Coalesce(campo(users, "saldo"), 0).Equal(0), true},
		{"Concat junta", Concat(campo(users, "email"), "!").Equal("ana@x.com!"), true},
		{"Concat pula o nulo", Concat(campo(users, "email"), campo(users, "saldo")).Equal("ana@x.com"), true},
	}
	for _, caso := range casos {
		if obtido := Matches(caso.expr, linha); obtido != caso.esperado {
			t.Errorf("%s: obtido %v, esperado %v", caso.titulo, obtido, caso.esperado)
		}
	}
}

// A condição de expressão precisa compor com And/Or como qualquer outra.
func TestMatchesDeExpressaoDentroDeGrupo(t *testing.T) {
	users := entidadeUsers()
	linha := linhaDeTeste{Id: 3, Nome: "ANA"}
	criterio := And(
		condition(campo(users, "id"), GreaterThan, 2),
		Or(Lower(campo(users, "nome")).Equal("ana"), Lower(campo(users, "nome")).Equal("bia")),
	)
	if !Matches(criterio, linha) {
		t.Error("o grupo com expressão deveria casar")
	}
	if !Evaluable(criterio) {
		t.Error("o grupo com expressão deveria ser avaliável em memória")
	}
}

// Coluna fora da projeção não tem valor para calcular: falso, e não panic.
func TestMatchesDeExpressaoComColunaAusente(t *testing.T) {
	users := entidadeUsers()
	type parcial struct{ Id int64 }
	if Matches(Lower(campo(users, "nome")).Contains("x"), parcial{Id: 1}) {
		t.Error("coluna ausente da linha deveria devolver falso")
	}
}

// Filtro dinâmico: expressão sem valor não filtra, igual ao Condition.
func TestExpressaoComInVazioNaoFiltra(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(Lower(campo(users, "nome")).In())
	compilada, err := q.Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(compilada.SQL, "WHERE") {
		t.Errorf("In vazio não deveria gerar WHERE: %s", compilada.SQL)
	}
}

// Agrupar por expressão com valor sai com placeholder diferente do da projeção, e
// nenhum dos quatro bancos casa as duas. Falhar na compilação, com a frase que diz
// o que fazer, é melhor que devolver o código de erro de cada banco.
func TestGroupByPorExpressaoComValorFalha(t *testing.T) {
	users := entidadeUsers()
	saldo := campo(users, "saldo")
	for _, dialeto := range []Dialect{MySQL, Postgres, Oracle, SQLServer} {
		_, err := modelo(users).GroupBy(Coalesce(saldo, 0)).Explain(OpSelect, dialeto)
		if !errors.Is(err, ErrValueInGroupBy) {
			t.Errorf("%s: esperado ErrValueInGroupBy, veio: %v", dialeto, err)
		}
	}
	// Expressão sem valor agrupa normalmente.
	if _, err := modelo(users).GroupBy(Lower(campo(users, "nome"))).Explain(OpSelect, Oracle); err != nil {
		t.Errorf("GroupBy por expressão sem valor deveria compilar: %v", err)
	}
	// E a detecção atravessa o aninhamento.
	if _, err := modelo(users).GroupBy(Lower(Concat(campo(users, "nome"), "x"))).Explain(OpSelect, MySQL); !errors.Is(err, ErrValueInGroupBy) {
		t.Errorf("valor em expressão aninhada deveria ser detectado: %v", err)
	}
}

// O concat do Postgres não deduz o tipo do parâmetro; sem a conversão explícita o
// banco recusa a consulta inteira.
func TestConcatConverteOParametroNoPostgres(t *testing.T) {
	users := entidadeUsers()
	compilada, err := Concat(campo(users, "nome"), " ").expressaoParaTeste(Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada, "CAST($1 AS TEXT)") {
		t.Errorf("esperado CAST no parâmetro do concat: %s", compilada)
	}
	// Os outros três deduzem pelo contexto e não levam conversão.
	for _, dialeto := range []Dialect{MySQL, Oracle, SQLServer} {
		texto, err := Concat(campo(users, "nome"), " ").expressaoParaTeste(dialeto)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(texto, "CAST") {
			t.Errorf("%s não deveria levar conversão: %s", dialeto, texto)
		}
	}
}

// expressaoParaTeste compila a expressão isolada, sem uma pesquisa em volta.
func (e Expr) expressaoParaTeste(d Dialect) (string, error) {
	return e.expressao(&compileCtx{dialect: d})
}

// Guardas de autoria.
func TestExpressoesRecusamArgumentacaoErrada(t *testing.T) {
	users := entidadeUsers()
	nome := campo(users, "nome")
	ctx := func() *compileCtx { return &compileCtx{dialect: MySQL} }

	if _, err := Concat(nome).expressao(ctx()); err == nil {
		t.Error("Concat de uma parte só deveria falhar")
	}
	if _, err := Coalesce(nome).expressao(ctx()); err == nil {
		t.Error("Coalesce de um argumento só deveria falhar")
	}
	if _, err := (Expr{fn: fnLower}).expressao(ctx()); err == nil {
		t.Error("expressão sem argumento deveria falhar")
	}
}
