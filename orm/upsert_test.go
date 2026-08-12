package orm

import (
	"strings"
	"testing"
)

// valoresDeUpsert é a linha usada nos testes de forma: a chave (email) mais duas
// colunas comuns.
func valoresDeUpsert(users EntityFields) Values {
	return Values{
		campo(users, "nome"):  "Ana",
		campo(users, "email"): "ana@exemplo.com",
		campo(users, "saldo"): 10.5,
	}
}

// O Upsert é a operação de forma mais diferente entre os quatro: dois dialetos
// estendem o INSERT, dois trocam o comando por MERGE. A mesma autoria tem de
// produzir os quatro, byte a byte.
func TestUpsertCompilaNosQuatroDialetos(t *testing.T) {
	users := entidadeUsers()
	chave := []Column{campo(users, "email")}

	casos := map[Dialect]string{
		MySQL: "INSERT INTO `users` (`nome`, `email`, `saldo`) VALUES (?, ?, ?) " +
			"ON DUPLICATE KEY UPDATE `nome` = VALUES(`nome`), `saldo` = VALUES(`saldo`)",

		Postgres: `INSERT INTO "users" ("nome", "email", "saldo") VALUES ($1, $2, $3) ` +
			`ON CONFLICT ("email") DO UPDATE SET "nome" = EXCLUDED."nome", "saldo" = EXCLUDED."saldo"`,

		Oracle: `MERGE INTO "USERS" t ` +
			`USING (SELECT :1 AS "NOME", :2 AS "EMAIL", :3 AS "SALDO" FROM dual) s ` +
			`ON (t."EMAIL" = s."EMAIL") ` +
			`WHEN MATCHED THEN UPDATE SET t."NOME" = s."NOME", t."SALDO" = s."SALDO" ` +
			`WHEN NOT MATCHED THEN INSERT ("NOME", "EMAIL", "SALDO") ` +
			`VALUES (s."NOME", s."EMAIL", s."SALDO")`,

		SQLServer: "MERGE INTO [users] AS t " +
			"USING (VALUES (@p1, @p2, @p3)) AS s ([nome], [email], [saldo]) " +
			"ON (t.[email] = s.[email]) " +
			"WHEN MATCHED THEN UPDATE SET t.[nome] = s.[nome], t.[saldo] = s.[saldo] " +
			"WHEN NOT MATCHED THEN INSERT ([nome], [email], [saldo]) " +
			"VALUES (s.[nome], s.[email], s.[saldo]);",
	}

	for dialeto, esperado := range casos {
		compilada, err := compileUpsert(users, valoresDeUpsert(users), chave, false,
			CompileOptions{Dialect: dialeto})
		if err != nil {
			t.Fatalf("%s: %v", dialeto, err)
		}
		if compilada.SQL != esperado {
			t.Errorf("%s:\n  esperado %s\n  obtido   %s", dialeto, esperado, compilada.SQL)
		}
		// Os args são a linha uma única vez, nos quatro: nenhum dialeto repete o
		// valor no lado da atualização. Se algum passar a repetir, os placeholders
		// deixam de casar com os args e o bug é silencioso.
		if len(compilada.Args) != 3 || compilada.Args[0] != "Ana" || compilada.Args[1] != "ana@exemplo.com" {
			t.Errorf("%s: argumentos fora de ordem: %v", dialeto, compilada.Args)
		}
	}
}

func TestInsertIgnoreCompilaNosQuatroDialetos(t *testing.T) {
	users := entidadeUsers()
	chave := []Column{campo(users, "email")}

	casos := map[Dialect]string{
		// A atribuição-identidade é o no-op do MySQL. INSERT IGNORE também
		// silenciaria o conflito, mas rebaixaria a aviso todo o resto.
		MySQL: "INSERT INTO `users` (`nome`, `email`, `saldo`) VALUES (?, ?, ?) " +
			"ON DUPLICATE KEY UPDATE `email` = `email`",

		Postgres: `INSERT INTO "users" ("nome", "email", "saldo") VALUES ($1, $2, $3) ` +
			`ON CONFLICT ("email") DO NOTHING`,

		// MERGE sem WHEN MATCHED: a linha existente não é tocada.
		Oracle: `MERGE INTO "USERS" t ` +
			`USING (SELECT :1 AS "NOME", :2 AS "EMAIL", :3 AS "SALDO" FROM dual) s ` +
			`ON (t."EMAIL" = s."EMAIL") ` +
			`WHEN NOT MATCHED THEN INSERT ("NOME", "EMAIL", "SALDO") ` +
			`VALUES (s."NOME", s."EMAIL", s."SALDO")`,

		SQLServer: "MERGE INTO [users] AS t " +
			"USING (VALUES (@p1, @p2, @p3)) AS s ([nome], [email], [saldo]) " +
			"ON (t.[email] = s.[email]) " +
			"WHEN NOT MATCHED THEN INSERT ([nome], [email], [saldo]) " +
			"VALUES (s.[nome], s.[email], s.[saldo]);",
	}

	for dialeto, esperado := range casos {
		compilada, err := compileUpsert(users, valoresDeUpsert(users), chave, true,
			CompileOptions{Dialect: dialeto})
		if err != nil {
			t.Fatalf("%s: %v", dialeto, err)
		}
		if compilada.SQL != esperado {
			t.Errorf("%s:\n  esperado %s\n  obtido   %s", dialeto, esperado, compilada.SQL)
		}
	}
}

// O ponto-e-vírgula do MERGE não é estilo: sem ele o SQL Server recusa o comando.
func TestMergeDoSQLServerTerminaComPontoEVirgula(t *testing.T) {
	users := entidadeUsers()
	for _, ignorar := range []bool{false, true} {
		compilada, err := compileUpsert(users, valoresDeUpsert(users),
			[]Column{campo(users, "email")}, ignorar, CompileOptions{Dialect: SQLServer})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(compilada.SQL, ";") {
			t.Errorf("ignorar=%v: MERGE sem ponto-e-vírgula final: %s", ignorar, compilada.SQL)
		}
	}
}

// O Oracle não aceita AS no apelido de tabela; o SQL Server aceita. Trocar os dois
// dá erro de sintaxe em um dos bancos e passa no outro.
func TestApelidoDeTabelaSegueOBancoNoMerge(t *testing.T) {
	users := entidadeUsers()
	chave := []Column{campo(users, "email")}

	oracle, err := compileUpsert(users, valoresDeUpsert(users), chave, false, CompileOptions{Dialect: Oracle})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(oracle.SQL, " AS t") {
		t.Errorf("o Oracle não aceita AS no apelido de tabela: %s", oracle.SQL)
	}

	mss, err := compileUpsert(users, valoresDeUpsert(users), chave, false, CompileOptions{Dialect: SQLServer})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mss.SQL, " AS t ") {
		t.Errorf("esperado apelido com AS no SQL Server: %s", mss.SQL)
	}
}

// A chave fica fora do SET: atualizar a coluna que identifica a linha durante a
// própria identificação não faz sentido, e o Oracle recusa com ORA-38104.
func TestUpsertNaoAtualizaAColunaDaChave(t *testing.T) {
	users := entidadeUsers()
	compilada, err := compileUpsert(users, valoresDeUpsert(users),
		[]Column{campo(users, "email")}, false, CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	set := compilada.SQL[strings.Index(compilada.SQL, "DO UPDATE SET"):]
	if strings.Contains(set, `"email"`) {
		t.Errorf("a coluna da chave não deveria estar no SET: %s", set)
	}
}

// Sem chave informada vale a chave primária, que é a identidade que a entidade já
// declara.
func TestUpsertSemChaveUsaAPrimaria(t *testing.T) {
	users := entidadeUsers()
	valores := Values{
		campo(users, "id"):   7,
		campo(users, "nome"): "Ana",
	}
	compilada, err := compileUpsert(users, valores, nil, false, CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, `ON CONFLICT ("id")`) {
		t.Errorf("esperado conflito pela chave primária: %s", compilada.SQL)
	}
}

// Chave composta compara coluna a coluna, unida por AND. Comparar só a primeira
// casaria com a linha errada e o UPDATE atingiria outro registro.
func TestUpsertComChaveCompostaComparaTodasAsColunas(t *testing.T) {
	matriculas := entidadeComposta()
	valores := Values{
		campo(matriculas, "aluno_cod"): 10,
		campo(matriculas, "curso_cod"): "MAT1",
		campo(matriculas, "situacao"):  "ativa",
	}

	pg, err := compileUpsert(matriculas, valores, nil, false, CompileOptions{Dialect: Postgres})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pg.SQL, `ON CONFLICT ("aluno_cod", "curso_cod")`) {
		t.Errorf("esperado conflito pelas duas colunas: %s", pg.SQL)
	}

	ora, err := compileUpsert(matriculas, valores, nil, false, CompileOptions{Dialect: Oracle})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ora.SQL, `ON (t."ALUNO_COD" = s."ALUNO_COD" AND t."CURSO_COD" = s."CURSO_COD")`) {
		t.Errorf("ON do MERGE deveria unir as duas colunas por AND: %s", ora.SQL)
	}
}

// A chave é de onde sai a comparação: sem valor, o MySQL gravaria sem checar nada
// e os outros recusariam. Falhar na chamada diz o que corrigir.
func TestUpsertExigeAChaveNosValores(t *testing.T) {
	users := entidadeUsers()
	valores := Values{campo(users, "nome"): "Ana"}
	_, err := compileUpsert(users, valores, []Column{campo(users, "email")}, false,
		CompileOptions{Dialect: MySQL})
	if err == nil || !strings.Contains(err.Error(), "email") {
		t.Fatalf("esperado erro apontando a coluna da chave, veio: %v", err)
	}
}

// Upsert com só a chave não tem o que atualizar. Aceitar em silêncio entregaria um
// Upsert que nunca atualiza — a mensagem aponta o InsertIgnore.
func TestUpsertSoComAChaveFalhaEIndicaOInsertIgnore(t *testing.T) {
	users := entidadeUsers()
	valores := Values{campo(users, "email"): "ana@exemplo.com"}
	_, err := compileUpsert(users, valores, []Column{campo(users, "email")}, false,
		CompileOptions{Dialect: MySQL})
	if err == nil || !strings.Contains(err.Error(), "InsertIgnore") {
		t.Fatalf("esperado erro indicando o InsertIgnore, veio: %v", err)
	}
	// O InsertIgnore, ao contrário, é legítimo com só a chave.
	if _, err := compileUpsert(users, valores, []Column{campo(users, "email")}, true,
		CompileOptions{Dialect: MySQL}); err != nil {
		t.Errorf("InsertIgnore com só a chave deveria compilar: %v", err)
	}
}

func TestUpsertRecusaChaveDeOutraEntidade(t *testing.T) {
	users := entidadeUsers()
	outra := EntityFields{Name: "cidades", Fields: []Field{
		{Name: "Nome", Entity: "cidades", Table: "cidades", Column: "nome", DataType: "string"},
	}}
	_, err := compileUpsert(users, valoresDeUpsert(users), []Column{campo(outra, "nome")}, false,
		CompileOptions{Dialect: MySQL})
	if err == nil || !strings.Contains(err.Error(), "não pertence") {
		t.Fatalf("esperado erro de coluna alheia, veio: %v", err)
	}
}

func TestUpsertSemValoresFalha(t *testing.T) {
	users := entidadeUsers()
	if _, err := compileUpsert(users, Values{}, nil, false, CompileOptions{Dialect: MySQL}); err == nil {
		t.Fatal("esperado erro para Upsert sem valores")
	}
}

// Entidade sem chave primária e sem chave informada não tem critério de
// identidade. Falhar aponta a migration.
func TestUpsertSemChaveNenhumaFalha(t *testing.T) {
	logs := EntityFields{Name: "logs", Fields: []Field{
		{Name: "Texto", Entity: "logs", Table: "logs", Column: "texto", DataType: "string"},
	}}
	_, err := compileUpsert(logs, Values{campo(logs, "texto"): "x"}, nil, false,
		CompileOptions{Dialect: MySQL})
	if err == nil || !strings.Contains(err.Error(), "PrimaryKey") {
		t.Fatalf("a mensagem deveria dizer como declarar a chave: %v", err)
	}
}

// O 2 do MySQL é detalhe de implementação (uma linha removida, uma inserida). A
// escrita é de uma linha só, então a resposta é 1 nos quatro.
func TestAfetadasNormalizaODoisDoMySQL(t *testing.T) {
	casos := map[int64]int64{0: 0, 1: 1, 2: 1}
	for entrada, esperado := range casos {
		if obtido := normalizarAfetadas(entrada); obtido != esperado {
			t.Errorf("normalizarAfetadas(%d) = %d, esperado %d", entrada, obtido, esperado)
		}
	}
}

// A ordem das colunas é a da entidade, não a do mapa: sem isso o SQL muda a cada
// execução e não há como comparar dialetos.
func TestUpsertTemOrdemEstavel(t *testing.T) {
	users := entidadeUsers()
	chave := []Column{campo(users, "email")}
	var primeiro string
	for i := 0; i < 20; i++ {
		compilada, err := compileUpsert(users, valoresDeUpsert(users), chave, false,
			CompileOptions{Dialect: Oracle})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			primeiro = compilada.SQL
			continue
		}
		if compilada.SQL != primeiro {
			t.Fatalf("ordem instável na iteração %d:\n  %s\n  %s", i, primeiro, compilada.SQL)
		}
	}
}

// O schema entra no nome da tabela também no MERGE.
func TestUpsertQualificaComSchema(t *testing.T) {
	users := entidadeUsers()
	chave := []Column{campo(users, "email")}
	compilada, err := compileUpsert(users, valoresDeUpsert(users), chave, false,
		CompileOptions{Dialect: SQLServer, Schema: "dbo"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(compilada.SQL, "MERGE INTO [dbo].[users] AS t ") {
		t.Errorf("schema ausente no MERGE: %s", compilada.SQL)
	}
}
