package orm

import (
	"strings"
	"testing"
)

// relacaoBelongsTo monta a relação com os metadados que o gerador preenche.
func relacaoBelongsTo() Relation {
	return NewRelation("Cidade", "belongsTo", "cidades", "cidade_id", "id", nil)
}

// A garantia mais importante do JOIN: quem NÃO usa junção gera o mesmo SQL de
// sempre. Sem isso, acrescentar JOIN teria reescrito as 55 leituras que passam.
func TestSemJoinOSQLNaoMuda(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(condition(campo(users, "nome"), Equal, "Ana")).OrderBy(campo(users, "id"))

	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(compilada.SQL, "`users`.`") {
		t.Errorf("coluna foi qualificada sem haver junção: %s", compilada.SQL)
	}
	if !strings.Contains(compilada.SQL, "WHERE `nome` = ?") {
		t.Errorf("WHERE mudou de forma: %s", compilada.SQL)
	}
	if !strings.Contains(compilada.SQL, "ORDER BY `id` ASC") {
		t.Errorf("ORDER BY mudou de forma: %s", compilada.SQL)
	}
}

// Com junção, TODA coluna passa a ser qualificada — senão "nome" existindo nas
// duas tabelas faz o banco recusar por ambiguidade.
func TestComJoinTodaColunaEQualificada(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).
		Join(relacaoBelongsTo()).
		Where(condition(campo(users, "nome"), Equal, "Ana")).
		OrderBy(campo(users, "id"))

	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	for _, esperado := range []string{
		"JOIN `cidades` ON `users`.`cidade_id` = `cidades`.`id`",
		"WHERE `users`.`nome` = ?",
		"ORDER BY `users`.`id` ASC",
		"SELECT `users`.`id`, `users`.`nome`",
	} {
		if !strings.Contains(compilada.SQL, esperado) {
			t.Errorf("esperado conter %s\n  obtido %s", esperado, compilada.SQL)
		}
	}
}

func TestJoinNosQuatroDialetos(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Join(relacaoBelongsTo())

	casos := map[Dialect]string{
		MySQL:     "JOIN `cidades` ON `users`.`cidade_id` = `cidades`.`id`",
		Postgres:  `JOIN "cidades" ON "users"."cidade_id" = "cidades"."id"`,
		Oracle:    `JOIN "CIDADES" ON "USERS"."CIDADE_ID" = "CIDADES"."ID"`,
		SQLServer: "JOIN [cidades] ON [users].[cidade_id] = [cidades].[id]",
	}
	for dialeto, esperado := range casos {
		compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: dialeto})
		if err != nil {
			t.Fatalf("%s: %v", dialeto, err)
		}
		if !strings.Contains(compilada.SQL, esperado) {
			t.Errorf("%s:\n  esperado conter %s\n  obtido          %s", dialeto, esperado, compilada.SQL)
		}
	}
}

func TestLeftJoinPreservaOSemCorrespondencia(t *testing.T) {
	users := entidadeUsers()
	compilada, err := modelo(users).LeftJoin(relacaoBelongsTo()).
		Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, "LEFT JOIN `cidades` ON") {
		t.Errorf("LEFT JOIN inesperado: %s", compilada.SQL)
	}
}

// Relação sem metadados não pode virar ON adivinhado: junção invertida devolve
// linhas erradas em silêncio.
func TestJoinComRelacaoSemMetadadosFalha(t *testing.T) {
	users := entidadeUsers()
	_, err := modelo(users).Join(Relation{}).Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err == nil {
		t.Fatal("relação sem metadados deveria falhar")
	}
	if !strings.Contains(err.Error(), "Relation") {
		t.Errorf("a mensagem deveria dizer de onde vem uma relação válida: %v", err)
	}
}

// Junção com agrupamento: a chave do grupo e a agregação também precisam ser
// qualificadas.
func TestJoinComAgrupamentoQualificaOGroupBy(t *testing.T) {
	users := entidadeUsers()
	cidade := campo(users, "cidade_id")
	compilada, err := modelo(users).
		Join(relacaoBelongsTo()).
		Select(cidade, Sum(campo(users, "saldo")).As("total")).
		GroupBy(cidade).
		Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	for _, esperado := range []string{
		"SUM(`users`.`saldo`) AS `total`",
		"GROUP BY `users`.`cidade_id`",
	} {
		if !strings.Contains(compilada.SQL, esperado) {
			t.Errorf("esperado conter %s\n  obtido %s", esperado, compilada.SQL)
		}
	}
}

// A junção atravessa o clone.
func TestJoinSobreviveAoClone(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Join(relacaoBelongsTo()).Where(condition(campo(users, "id"), Equal, 1)).Limit(5)
	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, "JOIN `cidades`") {
		t.Errorf("a junção se perdeu no clone: %s", compilada.SQL)
	}
}
