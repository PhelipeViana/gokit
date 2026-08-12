package orm

import (
	"strings"
	"testing"
)

// A trava divide os bancos em dois grupos, e a POSIÇÃO no comando é diferente:
// no SQL Server é dica colada na tabela, nos outros três é sufixo no fim.
func TestLockNosQuatroDialetos(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(condition(campo(users, "id"), Equal, 7)).Lock()

	casos := map[Dialect]string{
		MySQL:     "SELECT `id`, `nome`, `email`, `cidade_id`, `saldo`, `nascimento`, `ativo` FROM `users` WHERE `id` = ? FOR UPDATE",
		Postgres:  `WHERE "id" = $1 FOR UPDATE`,
		Oracle:    `WHERE "ID" = :1 FOR UPDATE`,
		SQLServer: "FROM [users] WITH (UPDLOCK, ROWLOCK) WHERE [id] = @p1",
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
	// No SQL Server o FOR UPDATE não existe: se ele aparecer, o comando não roda.
	compilada, _ := q.Compile(OpSelect, CompileOptions{Dialect: SQLServer})
	if strings.Contains(compilada.SQL, "FOR UPDATE") {
		t.Errorf("SQL Server não aceita FOR UPDATE: %s", compilada.SQL)
	}
}

func TestSemLockNadaMuda(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(condition(campo(users, "id"), Equal, 7))
	for _, d := range []Dialect{MySQL, Postgres, Oracle, SQLServer} {
		compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: d})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(compilada.SQL, "FOR UPDATE") || strings.Contains(compilada.SQL, "UPDLOCK") {
			t.Errorf("%s: trava apareceu sem Lock(): %s", d, compilada.SQL)
		}
	}
}

// A trava é só de leitura: num UPDATE ou DELETE o comando já trava por definição,
// e o FOR UPDATE ali seria erro de sintaxe.
func TestLockNaoVazaParaEscrita(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Where(condition(campo(users, "id"), Equal, 7)).Lock()

	compilada, err := compileUpdate(q, Values{campo(users, "nome"): "Ana"},
		CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(compilada.SQL, "FOR UPDATE") {
		t.Errorf("UPDATE não deveria levar a trava: %s", compilada.SQL)
	}
	compilada, err = compileDelete(q, CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(compilada.SQL, "FOR UPDATE") {
		t.Errorf("DELETE não deveria levar a trava: %s", compilada.SQL)
	}
}

func TestLockSobreviveAoClone(t *testing.T) {
	users := entidadeUsers()
	q := modelo(users).Lock().Where(condition(campo(users, "id"), Equal, 1)).Limit(1)
	compilada, err := q.Compile(OpSelect, CompileOptions{Dialect: MySQL})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, "FOR UPDATE") {
		t.Errorf("a trava se perdeu no clone: %s", compilada.SQL)
	}
}

func TestChunkRecusaTamanhoInvalido(t *testing.T) {
	users := entidadeUsers()
	if err := modelo(users).Chunk(nil, 0, func([]Record) error { return nil }); err == nil {
		t.Error("tamanho zero deveria falhar")
	}
	if err := modelo(users).Chunk(nil, -5, func([]Record) error { return nil }); err == nil {
		t.Error("tamanho negativo deveria falhar")
	}
}
