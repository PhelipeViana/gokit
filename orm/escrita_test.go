package orm

import (
	"errors"
	"strings"
	"testing"
)

// entidadeUsers reproduz o que o fields.gen.go gera: PK auto-incremento chamada
// id, mais colunas comuns.
func entidadeUsers() EntityFields {
	return EntityFields{
		Name: "users",
		Fields: []Field{
			{Name: "Id", Entity: "users", Table: "users", Column: "id", DataType: "integer", PrimaryKey: true, AutoIncrement: true},
			{Name: "Nome", Entity: "users", Table: "users", Column: "nome", DataType: "string"},
			{Name: "Email", Entity: "users", Table: "users", Column: "email", DataType: "string", Unique: true},
			{Name: "CidadeId", Entity: "users", Table: "users", Column: "cidade_id", DataType: "integer", Nullable: true},
			{Name: "Saldo", Entity: "users", Table: "users", Column: "saldo", DataType: "decimal", Nullable: true},
			{Name: "Nascimento", Entity: "users", Table: "users", Column: "nascimento", DataType: "date", Nullable: true},
			{Name: "Ativo", Entity: "users", Table: "users", Column: "ativo", DataType: "boolean", Nullable: true},
		},
	}
}

// entidadeComposta é o caso que o schema legado impõe: chave de duas colunas, sem
// auto-incremento, e nenhuma delas chamada id.
func entidadeComposta() EntityFields {
	return EntityFields{
		Name: "matriculas",
		Fields: []Field{
			{Name: "AlunoCod", Entity: "matriculas", Table: "matriculas", Column: "aluno_cod", DataType: "integer", PrimaryKey: true},
			{Name: "CursoCod", Entity: "matriculas", Table: "matriculas", Column: "curso_cod", DataType: "string", PrimaryKey: true},
			{Name: "Situacao", Entity: "matriculas", Table: "matriculas", Column: "situacao", DataType: "string"},
		},
	}
}

func campo(e EntityFields, coluna string) Field {
	f, ok := e.fieldPorColuna(coluna)
	if !ok {
		panic("coluna inexistente no teste: " + coluna)
	}
	return f
}

func modelo(e EntityFields) Model[Record] {
	return NewModel[Record](e, nil)
}

// A mesma autoria tem de produzir o SQL de cada banco, com o quoting e o
// placeholder de cada um. É a promessa central da ORM.
func TestInsertCompilaNosQuatroDialetos(t *testing.T) {
	users := entidadeUsers()
	valores := Values{
		campo(users, "nome"):  "Ana",
		campo(users, "email"): "ana@exemplo.com",
	}

	casos := map[Dialect]string{
		MySQL:     "INSERT INTO `users` (`nome`, `email`) VALUES (?, ?)",
		Postgres:  `INSERT INTO "users" ("nome", "email") VALUES ($1, $2)`,
		Oracle:    `INSERT INTO "USERS" ("NOME", "EMAIL") VALUES (:1, :2)`,
		SQLServer: "INSERT INTO [users] ([nome], [email]) VALUES (@p1, @p2)",
	}
	for dialeto, esperado := range casos {
		compilada, err := compileInsert(users, []Values{valores}, CompileOptions{Dialect: dialeto})
		if err != nil {
			t.Fatalf("%s: %v", dialeto, err)
		}
		if compilada.SQL != esperado {
			t.Errorf("%s:\n  esperado %s\n  obtido   %s", dialeto, esperado, compilada.SQL)
		}
		if len(compilada.Args) != 2 || compilada.Args[0] != "Ana" {
			t.Errorf("%s: argumentos fora de ordem: %v", dialeto, compilada.Args)
		}
	}
}

// A ordem das colunas é a da entidade, não a do mapa. Sem isso o SQL mudaria a
// cada execução e não haveria como comparar dialetos nem testar.
func TestInsertOrdenaColunasPelaEntidade(t *testing.T) {
	users := entidadeUsers()
	valores := Values{
		campo(users, "cidade_id"): 3,
		campo(users, "email"):     "z@z.com",
		campo(users, "nome"):      "Ana",
	}
	for i := 0; i < 20; i++ {
		compilada, err := compileInsert(users, []Values{valores}, CompileOptions{Dialect: MySQL})
		if err != nil {
			t.Fatal(err)
		}
		esperado := "INSERT INTO `users` (`nome`, `email`, `cidade_id`) VALUES (?, ?, ?)"
		if compilada.SQL != esperado {
			t.Fatalf("ordem instável na iteração %d:\n  %s", i, compilada.SQL)
		}
		if compilada.Args[0] != "Ana" || compilada.Args[2] != 3 {
			t.Fatalf("argumentos não acompanharam as colunas: %v", compilada.Args)
		}
	}
}

func TestInsertManyRepeteOsGruposDeValores(t *testing.T) {
	users := entidadeUsers()
	linhas := []Values{
		{campo(users, "nome"): "Ana"},
		{campo(users, "nome"): "Bia"},
	}
	compilada, err := compileInsert(users, linhas, CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	esperado := `INSERT INTO "users" ("nome") VALUES ($1), ($2)`
	if compilada.SQL != esperado {
		t.Errorf("esperado %s, obtido %s", esperado, compilada.SQL)
	}
	if len(compilada.Args) != 2 {
		t.Errorf("esperado 2 argumentos, obtido %v", compilada.Args)
	}
}

// Linha com conjunto de colunas diferente receberia NULL numa posição que o
// desenvolvedor não escreveu. Falhar é mais honesto.
func TestInsertManyRecusaColunasDiferentes(t *testing.T) {
	users := entidadeUsers()
	linhas := []Values{
		{campo(users, "nome"): "Ana"},
		{campo(users, "email"): "bia@exemplo.com"},
	}
	if _, err := compileInsert(users, linhas, CompileOptions{Dialect: MySQL}); err == nil {
		t.Fatal("esperado erro para colunas diferentes entre linhas")
	}
}

func TestInsertRecusaColunaDeOutraEntidade(t *testing.T) {
	users := entidadeUsers()
	outra := entidadeComposta()
	valores := Values{campo(outra, "situacao"): "ativo"}
	_, err := compileInsert(users, []Values{valores}, CompileOptions{Dialect: MySQL})
	if err == nil || !strings.Contains(err.Error(), "não pertence") {
		t.Fatalf("esperado erro de coluna alheia, veio: %v", err)
	}
}

// Os placeholders do SET precisam vir antes dos do WHERE, senão o Postgres liga
// o valor errado na coluna errada — e o SQL continua válido, o que faz o bug
// passar despercebido.
func TestUpdateNumeraSetAntesDoWhere(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(condition(campo(users, "id"), Equal, 7))
	compilada, err := compileUpdate(q, Values{campo(users, "nome"): "Ana"},
		CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	esperado := `UPDATE "users" SET "nome" = $1 WHERE "id" = $2`
	if compilada.SQL != esperado {
		t.Errorf("esperado %s, obtido %s", esperado, compilada.SQL)
	}
	if len(compilada.Args) != 2 || compilada.Args[0] != "Ana" || compilada.Args[1] != 7 {
		t.Errorf("argumentos fora de ordem: %v", compilada.Args)
	}
}

func TestDeleteUsaOMesmoWhereDaLeitura(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(Or(
		condition(campo(users, "id"), Equal, 1),
		condition(campo(users, "id"), Equal, 2),
	))
	compilada, err := compileDelete(q, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(compilada.SQL, "DELETE FROM `users` WHERE ") {
		t.Errorf("SQL inesperado: %s", compilada.SQL)
	}
	if len(compilada.Args) != 2 {
		t.Errorf("esperado 2 argumentos, obtido %v", compilada.Args)
	}
}

// Chave primária: nome diferente de id e chave composta são o caso normal em
// schema legado.
func TestCondicaoPorChaveComposta(t *testing.T) {
	matriculas := entidadeComposta()
	chaves := matriculas.PrimaryKeys()
	if len(chaves) != 2 || chaves[0].Column != "aluno_cod" || chaves[1].Column != "curso_cod" {
		t.Fatalf("chave composta na ordem errada: %+v", chaves)
	}

	condicao, err := matriculas.condicaoPorChave([]any{10, "MAT1"})
	if err != nil {
		t.Fatal(err)
	}
	q := modelo(matriculas).Where(condicao)
	compilada, err := compileDelete(q, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	esperado := "DELETE FROM `matriculas` WHERE (`aluno_cod` = ? AND `curso_cod` = ?)"
	if compilada.SQL != esperado {
		t.Errorf("esperado %s, obtido %s", esperado, compilada.SQL)
	}
}

// Informar menos valores que colunas de chave filtraria por menos colunas e
// atingiria a linha errada. Falhar é obrigatório.
func TestChaveIncompletaFalha(t *testing.T) {
	matriculas := entidadeComposta()
	_, err := matriculas.condicaoPorChave([]any{10})
	if err == nil {
		t.Fatal("esperado erro para chave composta incompleta")
	}
	if !strings.Contains(err.Error(), "aluno_cod, curso_cod") {
		t.Errorf("a mensagem deveria listar as colunas da chave: %v", err)
	}
}

func TestEntidadeSemChavePrimariaFalha(t *testing.T) {
	semChave := EntityFields{Name: "logs", Fields: []Field{
		{Name: "Texto", Column: "texto", DataType: "string"},
	}}
	_, err := semChave.condicaoPorChave([]any{1})
	if err == nil || !strings.Contains(err.Error(), "PrimaryKey") {
		t.Fatalf("a mensagem deveria dizer como declarar a chave: %v", err)
	}
}

// A proteção contra escrita sem filtro é a diferença entre corrigir uma linha e
// apagar uma tabela.
func TestUpdateEDeleteSemWhereFalham(t *testing.T) {
	users := entidadeUsers()
	m := modelo(users)

	if _, err := m.All().Update(nil, Values{campo(users, "nome"): "x"}); !errors.Is(err, ErrSemWhere) {
		t.Errorf("UPDATE sem Where deveria falhar com ErrSemWhere, veio: %v", err)
	}
	if _, err := m.All().Delete(nil); !errors.Is(err, ErrSemWhere) {
		t.Errorf("DELETE sem Where deveria falhar com ErrSemWhere, veio: %v", err)
	}
}

// .Todas() é o "sim, a tabela inteira". Sem conexão o erro passa a ser de
// conexão, o que prova que a guarda de Where já não é mais o obstáculo.
func TestTodasLiberaEscritaSemWhere(t *testing.T) {
	users := entidadeUsers()
	_, err := modelo(users).Todas().Delete(nil)
	if errors.Is(err, ErrSemWhere) {
		t.Error(".Todas() deveria liberar a escrita sem Where")
	}
	if !errors.Is(err, ErrNoConnection) {
		t.Errorf("esperado erro de conexão, veio: %v", err)
	}
}

// .Todas() precisa sobreviver ao clone, senão acrescentar um OrderBy depois
// reativaria a guarda.
func TestTodasSobreviveAoClone(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Todas().OrderBy(campo(users, "id")).Limit(10)
	if _, err := q.Delete(nil); errors.Is(err, ErrSemWhere) {
		t.Error("a liberação de .Todas() se perdeu no clone")
	}
}

// O OUTPUT do SQL Server tem posição obrigatória: entre a lista de colunas e o
// VALUES. Em qualquer outro lugar o comando não compila no banco.
func TestOutputDoSQLServerFicaAntesDoValues(t *testing.T) {
	entrada := "INSERT INTO [users] ([nome]) VALUES (@p1)"
	obtido := inserirOutputSQLServer(entrada, "[id]")
	esperado := "INSERT INTO [users] ([nome]) OUTPUT INSERTED.[id] VALUES (@p1)"
	if obtido != esperado {
		t.Errorf("esperado %s, obtido %s", esperado, obtido)
	}
}

// A validação precisa considerar a tabela, não só o nome da coluna: "nome" e
// "id" existem em quase toda entidade, e um Field de outra entidade passaria a
// gravar na tabela certa com a coluna de outra.
func TestInsertRecusaColunaHomonimaDeOutraEntidade(t *testing.T) {
	users := entidadeUsers()
	outra := EntityFields{Name: "cidades", Fields: []Field{
		{Name: "Nome", Entity: "cidades", Table: "cidades", Column: "nome", DataType: "string"},
	}}

	valores := Values{campo(outra, "nome"): "Curitiba"}
	_, err := compileInsert(users, []Values{valores}, CompileOptions{Dialect: MySQL})
	if err == nil {
		t.Fatal("um Field de cidades não deveria ser aceito num Insert de users")
	}
	if !strings.Contains(err.Error(), "não pertence") {
		t.Errorf("mensagem inesperada: %v", err)
	}
}
