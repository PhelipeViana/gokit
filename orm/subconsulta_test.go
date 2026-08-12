package orm

import (
	"strings"
	"testing"
)

func entidadeCidades() EntityFields {
	return EntityFields{
		Name: "cidades",
		Fields: []Field{
			{Name: "Id", Entity: "cidades", Table: "cidades", Column: "id", DataType: "integer", PrimaryKey: true, AutoIncrement: true},
			{Name: "Nome", Entity: "cidades", Table: "cidades", Column: "nome", DataType: "string"},
			{Name: "Uf", Entity: "cidades", Table: "cidades", Column: "uf", DataType: "string"},
		},
	}
}

// Comparação entre colunas nos quatro dialetos: só o quoting muda, e nenhum
// argumento é vinculado — os dois lados são identificador, não valor.
func TestWhereColumnCompilaNosQuatroDialetos(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(WhereColumn(
		campo(users, "saldo"), GreaterThan, campo(users, "id")))

	casos := map[Dialect]string{
		MySQL:     "WHERE `saldo` > `id`",
		Postgres:  `WHERE "saldo" > "id"`,
		Oracle:    `WHERE "SALDO" > "ID"`,
		SQLServer: "WHERE [saldo] > [id]",
	}
	for dialeto, esperado := range casos {
		compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: dialeto})
		if err != nil {
			t.Fatalf("%s: %v", dialeto, err)
		}
		if !strings.Contains(compilada.SQL, esperado) {
			t.Errorf("%s: esperado conter %s\n  obtido %s", dialeto, esperado, compilada.SQL)
		}
		if len(compilada.Args) != 0 {
			t.Errorf("%s: comparação entre colunas não deveria vincular argumento: %v", dialeto, compilada.Args)
		}
	}
}

func TestWhereColumnRecusaOperadorDeTexto(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(WhereColumn(campo(users, "nome"), Contains, campo(users, "email")))
	if _, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL}); err == nil {
		t.Fatal("Contains não deveria ser aceito entre duas colunas")
	}
}

// A numeração dos placeholders da subconsulta CONTINUA a do pai. Recomeçar do $1
// ligaria o valor errado no Postgres e no Oracle, e o SQL continuaria válido —
// bug silencioso.
func TestSubconsultaContinuaANumeracaoDoPai(t *testing.T) {
	users, cidades := entidadeUsers(), entidadeCidades()
	sub := modelo(cidades).
		Where(condition(campo(cidades, "uf"), Equal, "PR")).
		Select(campo(cidades, "id"))

	q := modelo(users).Where(
		condition(campo(users, "nome"), Equal, "Ana"),
		InQuery(campo(users, "cidade_id"), sub),
	)

	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	esperado := `WHERE "nome" = $1 AND "cidade_id" IN (SELECT "id" FROM "cidades" WHERE "uf" = $2)`
	if !strings.Contains(compilada.SQL, esperado) {
		t.Errorf("\n  esperado conter %s\n  obtido          %s", esperado, compilada.SQL)
	}
	if len(compilada.Args) != 2 || compilada.Args[0] != "Ana" || compilada.Args[1] != "PR" {
		t.Errorf("argumentos fora de ordem: %v", compilada.Args)
	}
}

// Sem Select explícito, a subconsulta projeta a chave primária — num IN, trazer
// todas as colunas seria erro de SQL.
func TestSubconsultaSemSelectUsaAChavePrimaria(t *testing.T) {
	users, cidades := entidadeUsers(), entidadeCidades()
	q := modelo(users).Where(InQuery(campo(users, "cidade_id"), modelo(cidades)))

	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, "IN (SELECT `id` FROM `cidades`)") {
		t.Errorf("projeção padrão inesperada: %s", compilada.SQL)
	}
}

func TestSubconsultaComMaisDeUmaColunaFalha(t *testing.T) {
	users, cidades := entidadeUsers(), entidadeCidades()
	sub := modelo(cidades).Select(campo(cidades, "id"), campo(cidades, "nome"))
	q := modelo(users).Where(InQuery(campo(users, "cidade_id"), sub))

	_, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err == nil {
		t.Fatal("subconsulta com duas colunas deveria falhar")
	}
	if !strings.Contains(err.Error(), "uma só") {
		t.Errorf("a mensagem deveria dizer o que fazer: %v", err)
	}
}

func TestNotInEExistsComSubconsulta(t *testing.T) {
	users, cidades := entidadeUsers(), entidadeCidades()
	sub := modelo(cidades).Where(condition(campo(cidades, "uf"), Equal, "SP")).Select(campo(cidades, "id"))

	naoEm := modelo(users).Where(NotInQuery(campo(users, "cidade_id"), sub))
	compilada, err := naoEm.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, "`cidade_id` NOT IN (SELECT") {
		t.Errorf("NOT IN inesperado: %s", compilada.SQL)
	}

	existe := modelo(users).Where(ExistsQuery(sub))
	compilada, err = existe.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, "EXISTS (SELECT `id` FROM `cidades`") {
		t.Errorf("EXISTS inesperado: %s", compilada.SQL)
	}

	naoExiste := modelo(users).Where(NotExistsQuery(sub))
	compilada, err = naoExiste.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, "NOT EXISTS (SELECT") {
		t.Errorf("NOT EXISTS inesperado: %s", compilada.SQL)
	}
}

// Subconsulta dentro de combinador precisa manter o parêntese do grupo e a ordem
// dos argumentos ao mesmo tempo.
func TestSubconsultaDentroDeOr(t *testing.T) {
	users, cidades := entidadeUsers(), entidadeCidades()
	sub := modelo(cidades).Where(condition(campo(cidades, "uf"), Equal, "PR")).Select(campo(cidades, "id"))

	q := modelo(users).Where(Or(
		condition(campo(users, "nome"), Equal, "Ana"),
		InQuery(campo(users, "cidade_id"), sub),
	))
	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, `("nome" = $1 OR "cidade_id" IN (SELECT "id" FROM "cidades" WHERE "uf" = $2))`) {
		t.Errorf("grupo com subconsulta inesperado: %s", compilada.SQL)
	}
}

// A escrita herda o WHERE da leitura, então subconsulta em UPDATE/DELETE tem de
// funcionar — e ali os placeholders começam depois dos do SET.
func TestSubconsultaNoUpdateContaOSetPrimeiro(t *testing.T) {
	users, cidades := entidadeUsers(), entidadeCidades()
	sub := modelo(cidades).Where(condition(campo(cidades, "uf"), Equal, "PR")).Select(campo(cidades, "id"))

	q := modelo(users).Where(InQuery(campo(users, "cidade_id"), sub))
	compilada, err := compileUpdate(q, Values{campo(users, "nome"): "Novo"},
		CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	esperado := `UPDATE "users" SET "nome" = $1 WHERE "cidade_id" IN (SELECT "id" FROM "cidades" WHERE "uf" = $2)`
	if compilada.SQL != esperado {
		t.Errorf("\n  esperado %s\n  obtido   %s", esperado, compilada.SQL)
	}
	if len(compilada.Args) != 2 || compilada.Args[0] != "Novo" || compilada.Args[1] != "PR" {
		t.Errorf("argumentos fora de ordem: %v", compilada.Args)
	}
}
