package orm

import (
	"errors"
	"strings"
	"testing"
)

// GroupBy sozinho traz a chave do grupo e a contagem: agrupar e devolver número
// sem rótulo nunca é o que se quer.
func TestGroupBySemProjecaoTrazChaveEContagem(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).GroupBy(campo(users, "cidade_id"))

	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	esperado := "SELECT `cidade_id`, COUNT(*) AS `count` FROM `users` GROUP BY `cidade_id`"
	if compilada.SQL != esperado {
		t.Errorf("\n  esperado %s\n  obtido   %s", esperado, compilada.SQL)
	}
}

func TestProjecaoAgrupadaNosQuatroDialetos(t *testing.T) {
	users := entidadeUsers()
	cidade := campo(users, "cidade_id")
	q := modelo(users).
		Select(cidade, Sum(campo(users, "saldo")).As("total")).
		GroupBy(cidade)

	casos := map[Dialect]string{
		MySQL:     "SELECT `cidade_id`, SUM(`saldo`) AS `total` FROM `users` GROUP BY `cidade_id`",
		Postgres:  `SELECT "cidade_id", SUM("saldo") AS "total" FROM "users" GROUP BY "cidade_id"`,
		Oracle:    `SELECT "CIDADE_ID", SUM("SALDO") AS "TOTAL" FROM "USERS" GROUP BY "CIDADE_ID"`,
		SQLServer: "SELECT [cidade_id], SUM([saldo]) AS [total] FROM [users] GROUP BY [cidade_id]",
	}
	for dialeto, esperado := range casos {
		compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: dialeto})
		if err != nil {
			t.Fatalf("%s: %v", dialeto, err)
		}
		if compilada.SQL != esperado {
			t.Errorf("%s:\n  esperado %s\n  obtido   %s", dialeto, esperado, compilada.SQL)
		}
	}
}

// O HAVING compara o resultado da agregação, então a esquerda é a expressão e não
// a coluna. E o placeholder dele entra depois dos do WHERE.
func TestHavingCompilaDepoisDoWhereEMantemAOrdemDosArgumentos(t *testing.T) {
	users := entidadeUsers()
	cidade := campo(users, "cidade_id")
	q := modelo(users).
		Where(condition(campo(users, "nome"), Equal, "Ana")).
		Select(cidade, CountAll().As("quantos")).
		GroupBy(cidade).
		Having(CountAll().GreaterThan(2))

	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	esperado := `SELECT "cidade_id", COUNT(*) AS "quantos" FROM "users" WHERE "nome" = $1 GROUP BY "cidade_id" HAVING COUNT(*) > $2`
	if compilada.SQL != esperado {
		t.Errorf("\n  esperado %s\n  obtido   %s", esperado, compilada.SQL)
	}
	if len(compilada.Args) != 2 || compilada.Args[0] != "Ana" || compilada.Args[1] != 2 {
		t.Errorf("argumentos fora de ordem: %v", compilada.Args)
	}
}

func TestHavingAceitaCombinadores(t *testing.T) {
	users := entidadeUsers()
	cidade := campo(users, "cidade_id")
	saldo := campo(users, "saldo")
	q := modelo(users).GroupBy(cidade).Having(Or(
		CountAll().GreaterThan(10),
		Sum(saldo).LessThan(100),
	))

	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, "HAVING (COUNT(*) > ? OR SUM(`saldo`) < ?)") {
		t.Errorf("HAVING combinado inesperado: %s", compilada.SQL)
	}
}

// O nome de saída é a chave do Record, então precisa ser previsível sem As.
func TestNomeDeSaidaPadraoDaAgregacao(t *testing.T) {
	users := entidadeUsers()
	casos := map[string]Aggregation{
		"count":     CountAll(),
		"sum_saldo": Sum(campo(users, "saldo")),
		"avg_saldo": Avg(campo(users, "saldo")),
		"min_id":    Min(campo(users, "id")),
		"max_id":    Max(campo(users, "id")),
		"total":     Sum(campo(users, "saldo")).As("total"),
	}
	for esperado, agregacao := range casos {
		if obtido := agregacao.OutputName(); obtido != esperado {
			t.Errorf("esperado %q, obtido %q", esperado, obtido)
		}
	}
}

// Sem GroupBy e sem SelectAgg nada muda: a compilação normal de leitura segue
// intacta. É a garantia de que o bloco 2 não mexeu no bloco anterior.
func TestSemAgrupamentoACompilacaoNaoMuda(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(condition(campo(users, "id"), Equal, 1))
	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(compilada.SQL, "GROUP BY") || strings.Contains(compilada.SQL, "COUNT(*)") {
		t.Errorf("projeção normal foi contaminada: %s", compilada.SQL)
	}
	if !strings.HasPrefix(compilada.SQL, "SELECT `id`, `nome`") {
		t.Errorf("projeção normal inesperada: %s", compilada.SQL)
	}
}

// Agrupamento atravessa o clone: acrescentar OrderBy ou Limit depois não pode
// perder o GROUP BY.
func TestAgrupamentoSobreviveAoClone(t *testing.T) {
	users := entidadeUsers()
	cidade := campo(users, "cidade_id")
	q := modelo(users).GroupBy(cidade).Having(CountAll().GreaterThan(1)).OrderBy(cidade).Limit(5)

	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	for _, esperado := range []string{"GROUP BY", "HAVING", "ORDER BY", "LIMIT"} {
		if !strings.Contains(compilada.SQL, esperado) {
			t.Errorf("%s se perdeu no clone: %s", esperado, compilada.SQL)
		}
	}
}

// A unificação: um Select só, que decide pelo que recebeu. Sem agregação segue o
// caminho tipado; com agregação vai para o agrupado.
func TestSelectUnificadoDecidePeloConteudo(t *testing.T) {
	users := entidadeUsers()
	cidade := campo(users, "cidade_id")

	tipado := modelo(users).Select(campo(users, "nome"))
	if len(tipado.aggSelects) != 0 || len(tipado.selects) != 1 {
		t.Errorf("projeção sem agregação deveria seguir tipada: selects=%d aggSelects=%d",
			len(tipado.selects), len(tipado.aggSelects))
	}

	agrupado := modelo(users).Select(cidade, Sum(campo(users, "saldo")).As("total"))
	if len(agrupado.aggSelects) != 2 || len(agrupado.selects) != 0 {
		t.Errorf("projeção com agregação deveria ir para o caminho agrupado: selects=%d aggSelects=%d",
			len(agrupado.selects), len(agrupado.aggSelects))
	}
}

// Get com agregação recusa em vez de devolver linha zerada com a agregação
// descartada — o erro diz qual terminal usar.
func TestGetRecusaProjecaoAgregada(t *testing.T) {
	users := entidadeUsers()
	cidade := campo(users, "cidade_id")

	if _, err := modelo(users).Select(cidade, CountAll()).Get(nil); !errors.Is(err, ErrAggregateProjection) {
		t.Errorf("esperado ErrAggregateProjection, veio: %v", err)
	}
	if _, err := modelo(users).GroupBy(cidade).Get(nil); !errors.Is(err, ErrAggregateProjection) {
		t.Errorf("GroupBy sem agregação explícita também não cabe no Get: %v", err)
	}
	if !strings.Contains(ErrAggregateProjection.Error(), "Rows") {
		t.Error("a mensagem deveria apontar o terminal certo")
	}
}

// O getter recebe a coluna, não o nome em texto: é o mesmo contrato do Where e do
// Select, e o compilador pega o erro de digitação.
func TestGettersDoRecordRecebemColuna(t *testing.T) {
	users := entidadeUsers()
	cidade := campo(users, "cidade_id")
	total := Sum(campo(users, "saldo")).As("total")
	quantos := CountAll().As("quantos")

	linha := Record{
		"cidade_id": Value{bruto: int64(7)},
		"total":     Value{bruto: 12.5},
		"quantos":   Value{bruto: int64(3)},
	}

	if linha.Int(cidade) != 7 {
		t.Errorf("Int por coluna: %v", linha.Int(cidade))
	}
	// A agregação guardada em variável é a chave de leitura — sem repetir o alias.
	if linha.Float(total) != 12.5 {
		t.Errorf("Float por agregação: %v", linha.Float(total))
	}
	if linha.Int(quantos) != 3 {
		t.Errorf("Int por agregação: %v", linha.Int(quantos))
	}
	if !linha.Has(cidade) || linha.Has(campo(users, "email")) {
		t.Error("Has deveria seguir a presença da coluna")
	}

	// O Oracle devolve o nome em maiúsculas; a leitura não pode depender da caixa.
	maiusculo := Record{"TOTAL": Value{bruto: 9.5}}
	if maiusculo.Float(total) != 9.5 {
		t.Error("a leitura deveria ignorar a caixa do nome")
	}

	// Coluna ausente: zero, sem panic.
	if linha.Float(campo(users, "saldo")) != 0 {
		t.Error("coluna ausente deveria devolver zero")
	}

	// A saída por texto continua existindo, explícita.
	if linha.ByName("total").Float() != 12.5 {
		t.Error("ByName deveria ler pelo nome cru")
	}
}
