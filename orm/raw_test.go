package orm

import (
	"strings"
	"testing"
)

// regexPorDialeto é o exemplo canônico do bloco 6: expressão regular, que os quatro
// bancos escrevem de forma diferente e nenhuma expressão do bloco 5 cobre.
func regexPorDialeto() map[Dialect]string {
	return map[Dialect]string{
		MySQL:     "`nome` REGEXP {}",
		Postgres:  `"nome" ~ {}`,
		Oracle:    `REGEXP_LIKE("NOME", {})`,
		SQLServer: "[nome] LIKE {}",
	}
}

// O fragmento de cada dialeto entra no lugar certo, com o placeholder de cada um.
func TestRawPorCompilaOFragmentoDoDialeto(t *testing.T) {
	users := entidadeUsers()
	casos := map[Dialect]string{
		MySQL:     "SELECT `id`, `nome`, `email`, `cidade_id`, `saldo`, `nascimento`, `ativo` FROM `users` WHERE (`nome` REGEXP ?)",
		Postgres:  `SELECT "id", "nome", "email", "cidade_id", "saldo", "nascimento", "ativo" FROM "users" WHERE ("nome" ~ $1)`,
		Oracle:    `SELECT "ID", "NOME", "EMAIL", "CIDADE_ID", "SALDO", "NASCIMENTO", "ATIVO" FROM "USERS" WHERE (REGEXP_LIKE("NOME", :1))`,
		SQLServer: "SELECT [id], [nome], [email], [cidade_id], [saldo], [nascimento], [ativo] FROM [users] WHERE ([nome] LIKE @p1)",
	}
	for dialeto, esperado := range casos {
		compilada, err := modelo(users).
			Where(RawPerDialect(regexPorDialeto(), "^Ana")).
			Explain(OpSelect, dialeto)
		if err != nil {
			t.Fatalf("%s: %v", dialeto, err)
		}
		if compilada.SQL != esperado {
			t.Errorf("%s:\n  esperado %s\n  obtido   %s", dialeto, esperado, compilada.SQL)
		}
		if len(compilada.Args) != 1 || compilada.Args[0] != "^Ana" {
			t.Errorf("%s: o valor deveria estar nos argumentos: %v", dialeto, compilada.Args)
		}
	}
}

// A regra central do bloco: o valor não aparece no texto, em nenhum dialeto.
func TestRawNaoPoeValorNoTexto(t *testing.T) {
	users := entidadeUsers()
	segredo := "'; DROP TABLE users; --"
	for _, dialeto := range []Dialect{MySQL, Postgres, Oracle, SQLServer} {
		compilada, err := modelo(users).
			Where(RawPerDialect(regexPorDialeto(), segredo)).
			Explain(OpSelect, dialeto)
		if err != nil {
			t.Fatalf("%s: %v", dialeto, err)
		}
		if strings.Contains(compilada.SQL, "DROP") {
			t.Errorf("%s: o valor entrou no texto do SQL: %s", dialeto, compilada.SQL)
		}
		if len(compilada.Args) != 1 || compilada.Args[0] != segredo {
			t.Errorf("%s: o valor deveria ter virado argumento: %v", dialeto, compilada.Args)
		}
	}
}

// Este é o teste que justifica o marcador neutro: o número do placeholder depende
// do que as OUTRAS cláusulas vincularam antes, então o autor não teria como
// escrevê-lo. Aqui o fragmento é o terceiro valor da pesquisa.
func TestRawNumeraOPlaceholderPelaPosicaoRealDaPesquisa(t *testing.T) {
	users := entidadeUsers()
	compilada, err := modelo(users).
		Where(
			condition(campo(users, "id"), GreaterThan, 10),
			condition(campo(users, "email"), Equal, "a@b.com"),
			RawPerDialect(regexPorDialeto(), "^Ana"),
		).
		Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, `("nome" ~ $3)`) {
		t.Errorf("o fragmento deveria receber $3: %s", compilada.SQL)
	}
	if len(compilada.Args) != 3 || compilada.Args[2] != "^Ana" {
		t.Errorf("argumentos fora de ordem: %v", compilada.Args)
	}
}

// O fragmento vem entre parênteses: um OR interno mudaria a precedência do Where
// inteiro se entrasse cru na árvore.
func TestRawFicaEntreParenteses(t *testing.T) {
	users := entidadeUsers()
	comOr := map[Dialect]string{
		MySQL: "`a` = 1 OR `b` = 2", Postgres: `"a" = 1 OR "b" = 2`,
		Oracle: `"A" = 1 OR "B" = 2`, SQLServer: "[a] = 1 OR [b] = 2",
	}
	compilada, err := modelo(users).
		Where(condition(campo(users, "id"), Equal, 1), RawPerDialect(comOr)).
		Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compilada.SQL, `WHERE "id" = $1 AND ("a" = 1 OR "b" = 2)`) {
		t.Errorf("o fragmento deveria estar entre parênteses: %s", compilada.SQL)
	}
}

// RawPerDialect exige os quatro, e a falta aparece compilando QUALQUER um deles — é o que
// faz o problema surgir na máquina de quem escreveu, não no banco do cliente.
func TestRawPorExigeOsQuatroDialetos(t *testing.T) {
	users := entidadeUsers()
	incompleto := map[Dialect]string{
		MySQL:    "`nome` REGEXP {}",
		Postgres: `"nome" ~ {}`,
	}
	// Compilando para MySQL, que ESTÁ declarado: mesmo assim tem de falhar.
	_, err := modelo(users).Where(RawPerDialect(incompleto, "^Ana")).Explain(OpSelect, MySQL)
	if err == nil {
		t.Fatal("RawPerDialect incompleto deveria falhar mesmo no dialeto declarado")
	}
	if !strings.Contains(err.Error(), "oracle") || !strings.Contains(err.Error(), "sqlserver") {
		t.Errorf("a mensagem deveria dizer quais faltam: %v", err)
	}
	if !strings.Contains(err.Error(), "RawOnly") {
		t.Errorf("a mensagem deveria apontar o RawOnly como alternativa: %v", err)
	}
}

// RawOnly roda no dialeto declarado e FALHA nos outros três. Falhar é o recurso.
func TestRawEmSoValeNoDialetoDeclarado(t *testing.T) {
	users := entidadeUsers()
	fragmento := RawOnly(Oracle, `REGEXP_LIKE("NOME", {})`, "^Ana")

	compilada, err := modelo(users).Where(fragmento).Explain(OpSelect, Oracle)
	if err != nil {
		t.Fatalf("no Oracle deveria compilar: %v", err)
	}
	if !strings.Contains(compilada.SQL, `REGEXP_LIKE("NOME", :1)`) {
		t.Errorf("SQL inesperado: %s", compilada.SQL)
	}

	for _, outro := range []Dialect{MySQL, Postgres, SQLServer} {
		_, err := modelo(users).Where(fragmento).Explain(OpSelect, outro)
		if err == nil {
			t.Errorf("%s: RawOnly(oracle) deveria falhar", outro)
			continue
		}
		if !strings.Contains(err.Error(), "oracle") || !strings.Contains(err.Error(), string(outro)) {
			t.Errorf("%s: a mensagem deveria citar os dois dialetos: %v", outro, err)
		}
	}
}

// Placeholder escrito à mão é erro de autoria: a numeração depende da pesquisa
// inteira, e o autor não tem essa informação.
func TestRawRecusaPlaceholderEscritoAMao(t *testing.T) {
	users := entidadeUsers()
	casos := map[string]map[Dialect]string{
		"$1":  {MySQL: "`a` = {}", Postgres: `"a" = $1`, Oracle: `"A" = {}`, SQLServer: "[a] = {}"},
		":1":  {MySQL: "`a` = {}", Postgres: `"a" = {}`, Oracle: `"A" = :1`, SQLServer: "[a] = {}"},
		"@p1": {MySQL: "`a` = {}", Postgres: `"a" = {}`, Oracle: `"A" = {}`, SQLServer: "[a] = @p1"},
		// O ? é o placeholder real do MySQL, e é o que o autor escreve por reflexo.
		"?": {MySQL: "`a` = ?", Postgres: `"a" = {}`, Oracle: `"A" = {}`, SQLServer: "[a] = {}"},
	}
	for marca, porDialeto := range casos {
		_, err := modelo(users).Where(RawPerDialect(porDialeto, 1)).Explain(OpSelect, MySQL)
		if err == nil {
			t.Errorf("%s: deveria falhar", marca)
			continue
		}
		if !strings.Contains(err.Error(), marca) || !strings.Contains(err.Error(), "{}") {
			t.Errorf("%s: a mensagem deveria citar a marca e o {}: %v", marca, err)
		}
	}
}

// Contagem de marcadores diferente entre dialetos poria o valor em posição
// diferente em cada banco — com SQL válido nos quatro.
func TestRawExigeAMesmaContagemDeValoresEmTodosOsDialetos(t *testing.T) {
	users := entidadeUsers()
	desalinhado := map[Dialect]string{
		MySQL:     "`nome` REGEXP {}",
		Postgres:  `"nome" ~ {}`,
		Oracle:    `REGEXP_LIKE("NOME", {}, {})`, // um marcador a mais
		SQLServer: "[nome] LIKE {}",
	}
	_, err := modelo(users).Where(RawPerDialect(desalinhado, "^Ana")).Explain(OpSelect, MySQL)
	if err == nil {
		t.Fatal("contagem diferente entre dialetos deveria falhar")
	}
	if !strings.Contains(err.Error(), "oracle") {
		t.Errorf("a mensagem deveria apontar o dialeto desalinhado: %v", err)
	}
}

// No Postgres, `?`, `?|` e `?&` são operadores de jsonb. Acusá-los ali proibiria
// consulta legítima, então o `?` só é acusado no fragmento do MySQL.
func TestRawAceitaInterrogacaoNoPostgres(t *testing.T) {
	users := entidadeUsers()
	comJsonb := map[Dialect]string{
		MySQL:     "JSON_CONTAINS_PATH(`extra`, 'one', '$.cpf')",
		Postgres:  `"extra" ? 'cpf'`,
		Oracle:    `JSON_EXISTS("EXTRA", '$.cpf')`,
		SQLServer: "JSON_VALUE([extra], '$.cpf') IS NOT NULL",
	}
	if _, err := modelo(users).Where(RawPerDialect(comJsonb)).Explain(OpSelect, Postgres); err != nil {
		t.Errorf("o operador jsonb ? do Postgres deveria passar: %v", err)
	}
}

func TestRawSemValorNemMarcador(t *testing.T) {
	users := entidadeUsers()
	semValor := map[Dialect]string{
		MySQL: "`ativo` = 1", Postgres: `"ativo" = true`,
		Oracle: `"ATIVO" = 1`, SQLServer: "[ativo] = 1",
	}
	compilada, err := modelo(users).Where(RawPerDialect(semValor)).Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if len(compilada.Args) != 0 {
		t.Errorf("fragmento sem marcador não deveria vincular nada: %v", compilada.Args)
	}
}

// O fragmento serve nos dois papéis: predicado no Where e expressão escalar nas
// outras cláusulas — é o que o Column dá, igual ao bloco 5.
func TestRawServeNasOutrasClausulas(t *testing.T) {
	users := entidadeUsers()
	tamanho := map[Dialect]string{
		MySQL: "LENGTH(`nome`)", Postgres: `LENGTH("nome")`,
		Oracle: `LENGTH("NOME")`, SQLServer: "LEN([nome])",
	}

	projecao, err := modelo(users).Select(RawPerDialect(tamanho).As("tam")).Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(projecao.SQL, `LENGTH("nome") AS "tam"`) {
		t.Errorf("Select: %s", projecao.SQL)
	}

	ordem, err := modelo(users).OrderByDesc(RawPerDialect(tamanho)).Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordem.SQL, `ORDER BY LENGTH("nome") DESC`) {
		t.Errorf("OrderBy: %s", ordem.SQL)
	}

	// No GroupBy o alias também é exigido, porque a chave do grupo entra na
	// projeção automática.
	grupo, err := modelo(users).GroupBy(RawPerDialect(tamanho).As("tam")).Explain(OpSelect, Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(grupo.SQL, `GROUP BY LENGTH("nome")`) {
		t.Errorf("GroupBy: %s", grupo.SQL)
	}
	if !strings.Contains(grupo.SQL, `LENGTH("nome") AS "tam"`) {
		t.Errorf("GroupBy deveria projetar a chave do grupo com o alias: %s", grupo.SQL)
	}
	if _, err := modelo(users).GroupBy(RawPerDialect(tamanho)).Explain(OpSelect, Postgres); err == nil {
		t.Error("GroupBy por fragmento sem As deveria falhar, porque a chave vai para a projeção")
	}
}

// Sem As na projeção não há como o Record achar o valor: cada banco nomeia
// expressão anônima de um jeito.
func TestRawNaProjecaoExigeAlias(t *testing.T) {
	users := entidadeUsers()
	tamanho := map[Dialect]string{
		MySQL: "LENGTH(`nome`)", Postgres: `LENGTH("nome")`,
		Oracle: `LENGTH("NOME")`, SQLServer: "LEN([nome])",
	}
	_, err := modelo(users).Select(RawPerDialect(tamanho)).Explain(OpSelect, Postgres)
	if err == nil || !strings.Contains(err.Error(), "As(") {
		t.Fatalf("esperado erro pedindo o As, veio: %v", err)
	}
}

// Fragmento cru precisa do banco: Matches devolve falso e Evaluable diz isso ANTES,
// que é o sinal de que a camada de regras precisa.
func TestRawNaoEAvaliavelEmMemoria(t *testing.T) {
	fragmento := RawPerDialect(regexPorDialeto(), "^Ana")
	if Evaluable(fragmento) {
		t.Error("fragmento cru não deveria ser avaliável em memória")
	}
	if Matches(fragmento, linhaDeTeste{Nome: "Ana"}) {
		t.Error("Matches de fragmento cru deveria ser falso")
	}
	// E contamina o grupo que o contém: quem pergunta antes recebe a resposta certa.
	users := entidadeUsers()
	grupo := And(condition(campo(users, "id"), GreaterThan, 1), fragmento)
	if Evaluable(grupo) {
		t.Error("grupo com fragmento cru não deveria ser avaliável")
	}
}

func TestRawVazioFalha(t *testing.T) {
	users := entidadeUsers()
	_, err := modelo(users).Where(Raw{}).Explain(OpSelect, MySQL)
	if err == nil {
		t.Fatal("Raw sem fragmento deveria falhar")
	}
}
