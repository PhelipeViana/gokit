package orm

// Bloco 4 (parte 2): Upsert — gravar sem saber se a linha já existe.
//
// É a operação em que os quatro bancos mais divergem de forma: não é uma
// cláusula a mais no INSERT, são dois comandos diferentes.
//
//	MySQL       INSERT ... ON DUPLICATE KEY UPDATE
//	Postgres    INSERT ... ON CONFLICT (chave) DO UPDATE SET
//	Oracle      MERGE INTO ... USING (SELECT :1 ... FROM dual) ...
//	SQL Server  MERGE INTO ... USING (VALUES (@p1, ...)) ... ;
//
// Sem isto, todo "grava ou atualiza" — importação, sincronização, idempotência de
// integração — viraria Raw por dialeto, que é exatamente o que a ORM existe para
// evitar.
//
// Duas operações, porque são duas intenções diferentes e confundi-las é perda de
// dado silenciosa: Upsert atualiza a linha existente; InsertIgnore a preserva.
//
// Nenhuma das duas devolve LastID. Numa escrita que pode não inserir nada, "a
// chave gerada" não existe: o MySQL informa valor de sequência mesmo quando não
// inseriu, e o RETURNING do Postgres não dispara no DO NOTHING. Quem precisa da
// chave lê pela chave de conflito, que ele acabou de informar.

import (
	"context"
	"strings"
)

// Upsert grava a linha e, se ela já existir pela chave informada, atualiza as
// outras colunas de valores.
//
// A chave é o critério de "já existe": as colunas cuja violação de unicidade
// significa a mesma linha. Sem chave informada vale a chave primária. As colunas
// da chave precisam estar em valores — é delas que sai a comparação — e ficam
// fora do que se atualiza: mudar a coluna que identifica a linha durante a
// própria identificação não faz sentido, e o Oracle recusa.
//
// A chave precisa ter índice único no banco. Sem ele, Postgres e SQL Server
// recusam o comando e o MySQL insere duplicado sem reclamar — a garantia é do
// schema, não da consulta.
//
// Ressalva do MySQL: ele não sabe restringir o conflito a uma chave específica.
// Colidir em QUALQUER chave única da tabela dispara a atualização. Nos outros
// três, só a chave informada. Tabela com uma unicidade só — o caso normal — se
// comporta igual nos quatro.
func (m Model[T]) Upsert(ctx context.Context, valores Values, chave ...Column) (Result, error) {
	return m.UpsertWith(ctx, nil, valores, chave...)
}

func (m Model[T]) UpsertWith(ctx context.Context, r Runner, valores Values, chave ...Column) (Result, error) {
	return m.gravarIdempotente(ctx, r, valores, chave, false)
}

// InsertIgnore grava a linha e, se ela já existir pela chave informada, não faz
// nada — a linha do banco fica como está.
//
// Affected distingue os dois desfechos nos quatro dialetos: 1 quando inseriu, 0
// quando a linha já existia.
//
// Ignora só o conflito de chave, não erro de escrita: valor nulo em coluna
// obrigatória, texto longo demais e FK inexistente continuam falhando. Por isso
// não usa INSERT IGNORE do MySQL, que rebaixa todos esses a aviso e grava a linha
// truncada.
func (m Model[T]) InsertIgnore(ctx context.Context, valores Values, chave ...Column) (Result, error) {
	return m.InsertIgnoreWith(ctx, nil, valores, chave...)
}

func (m Model[T]) InsertIgnoreWith(ctx context.Context, r Runner, valores Values, chave ...Column) (Result, error) {
	return m.gravarIdempotente(ctx, r, valores, chave, true)
}

// ExplainUpsert e ExplainInsertIgnore são atalhos de DEPURAÇÃO, iguais ao
// Explain da leitura: compilam para um dialeto sem tocar no banco. É o jeito de
// ver, lado a lado, as quatro formas que a mesma autoria produz.
func (m Model[T]) ExplainUpsert(valores Values, dialeto Dialect, chave ...Column) (CompiledQuery, error) {
	return compileUpsert(m.Entity, valores, chave, false, CompileOptions{Dialect: dialeto})
}

func (m Model[T]) ExplainInsertIgnore(valores Values, dialeto Dialect, chave ...Column) (CompiledQuery, error) {
	return compileUpsert(m.Entity, valores, chave, true, CompileOptions{Dialect: dialeto})
}

func (m Model[T]) gravarIdempotente(ctx context.Context, r Runner, valores Values, chave []Column, ignorar bool) (Result, error) {
	if err := conferirAutoria(valores); err != nil {
		return Result{}, err
	}
	alvo, exec, err := runnerParaEscrita(r)
	if err != nil {
		return Result{}, err
	}
	compilada, err := compileUpsert(m.Entity, valores, chave, ignorar,
		CompileOptions{Dialect: alvo.Dialect(), Schema: alvo.Schema()})
	if err != nil {
		return Result{}, err
	}
	saida, err := exec.ExecContext(ctx, compilada.SQL, compilada.Args...)
	if err != nil {
		return Result{}, ClassifyError(err)
	}
	afetadas, _ := saida.RowsAffected()
	return Result{Affected: normalizarAfetadas(afetadas)}, nil
}

// normalizarAfetadas achata o retorno do MySQL, que conta a atualização por
// ON DUPLICATE KEY como duas linhas (uma removida, uma inserida). A escrita é de
// uma linha só, então 2 não é informação — é detalhe de implementação vazando. Os
// outros três já devolvem 1.
func normalizarAfetadas(n int64) int64 {
	if n > 1 {
		return 1
	}
	return n
}

// compileUpsert monta o comando do dialeto ativo.
func compileUpsert(entity EntityFields, valores Values, chave []Column, ignorar bool, options CompileOptions) (CompiledQuery, error) {
	d := options.Dialect
	if !supportedDialect(d) {
		return CompiledQuery{}, errDialetoNaoSuportado(d)
	}
	operacao := "Upsert"
	if ignorar {
		operacao = "InsertIgnore"
	}

	campos, dados, err := valores.ordenar(entity)
	if err != nil {
		return CompiledQuery{}, err
	}
	if len(campos) == 0 {
		return CompiledQuery{}, errSemValores(operacao)
	}

	chaves, err := camposDeConflito(entity, chave)
	if err != nil {
		return CompiledQuery{}, err
	}
	for _, k := range chaves {
		if !contemCampo(campos, k) {
			return CompiledQuery{}, errChaveDeConflitoSemValor(k.Column, operacao)
		}
	}

	// O SET é tudo o que foi informado menos a chave. Vazio significa que só a
	// chave foi informada: não há o que atualizar, e responder DO NOTHING em
	// silêncio esconderia do autor que o Upsert dele não atualiza nada.
	atualizaveis := semOsCampos(campos, chaves)
	if !ignorar && len(atualizaveis) == 0 {
		return CompiledQuery{}, errUpsertSemAtualizacao(nomesDeColunas(chaves))
	}

	// Os args são os valores da linha, na ordem das colunas, e valem para os
	// quatro: nenhum dialeto precisa repetir o valor no lado da atualização
	// (VALUES(col), EXCLUDED.col e o alias do MERGE apontam para o que já foi
	// vinculado).
	marcas := make([]string, 0, len(campos))
	args := make([]any, 0, len(campos))
	for i := range campos {
		args = append(args, dados[i])
		marcas = append(marcas, placeholderFor(d, len(args)))
	}

	var sql string
	switch d {
	case MySQL:
		sql = upsertMySQL(d, options.Schema, entity, campos, marcas, chaves, atualizaveis, ignorar)
	case Postgres:
		sql = upsertPostgres(d, options.Schema, entity, campos, marcas, chaves, atualizaveis, ignorar)
	case Oracle:
		sql = upsertOracle(d, options.Schema, entity, campos, marcas, chaves, atualizaveis, ignorar)
	case SQLServer:
		sql = upsertSQLServer(d, options.Schema, entity, campos, marcas, chaves, atualizaveis, ignorar)
	}
	return CompiledQuery{SQL: sql, Args: args}, nil
}

// upsertMySQL: a atualização lê o valor já vinculado com VALUES(coluna).
//
// Para o InsertIgnore a atribuição é uma identidade (chave = chave): é um no-op
// que o MySQL aceita e que, diferente do INSERT IGNORE, não rebaixa nenhum outro
// erro a aviso.
func upsertMySQL(d Dialect, schema string, entity EntityFields, campos []Field, marcas []string, chaves, atualizaveis []Field, ignorar bool) string {
	atribuicoes := make([]string, 0, len(atualizaveis))
	if ignorar {
		coluna := quoteIdentFor(d, chaves[0].Column)
		atribuicoes = append(atribuicoes, coluna+" = "+coluna)
	} else {
		for _, campo := range atualizaveis {
			coluna := quoteIdentFor(d, campo.Column)
			atribuicoes = append(atribuicoes, coluna+" = VALUES("+coluna+")")
		}
	}
	return "INSERT INTO " + qualifyFor(d, schema, entity.Name) +
		" (" + listaDeColunas(d, campos) + ") VALUES (" + strings.Join(marcas, ", ") + ")" +
		" ON DUPLICATE KEY UPDATE " + strings.Join(atribuicoes, ", ")
}

// upsertPostgres: a atualização lê a linha rejeitada pelo pseudo-registro
// EXCLUDED, e o alvo do conflito é declarado — só a chave informada dispara.
func upsertPostgres(d Dialect, schema string, entity EntityFields, campos []Field, marcas []string, chaves, atualizaveis []Field, ignorar bool) string {
	sql := "INSERT INTO " + qualifyFor(d, schema, entity.Name) +
		" (" + listaDeColunas(d, campos) + ") VALUES (" + strings.Join(marcas, ", ") + ")" +
		" ON CONFLICT (" + listaDeColunas(d, chaves) + ")"
	if ignorar {
		return sql + " DO NOTHING"
	}
	atribuicoes := make([]string, 0, len(atualizaveis))
	for _, campo := range atualizaveis {
		coluna := quoteIdentFor(d, campo.Column)
		atribuicoes = append(atribuicoes, coluna+" = EXCLUDED."+coluna)
	}
	return sql + " DO UPDATE SET " + strings.Join(atribuicoes, ", ")
}

// upsertOracle: MERGE com a linha nova montada como consulta a dual.
//
// O Oracle não aceita AS no apelido de tabela, e as colunas da cláusula INSERT
// vão sem apelido — qualificá-las ali é erro de sintaxe.
func upsertOracle(d Dialect, schema string, entity EntityFields, campos []Field, marcas []string, chaves, atualizaveis []Field, ignorar bool) string {
	projecao := make([]string, 0, len(campos))
	for i, campo := range campos {
		projecao = append(projecao, marcas[i]+" AS "+quoteIdentFor(d, campo.Column))
	}
	sql := "MERGE INTO " + qualifyFor(d, schema, entity.Name) + " " + apelidoAlvo +
		" USING (SELECT " + strings.Join(projecao, ", ") + " FROM dual) " + apelidoOrigem +
		" ON (" + condicaoDeConflito(d, chaves) + ")"
	if !ignorar {
		sql += " WHEN MATCHED THEN UPDATE SET " + atribuicoesDoMerge(d, atualizaveis)
	}
	return sql + " WHEN NOT MATCHED THEN INSERT (" + listaDeColunas(d, campos) + ")" +
		" VALUES (" + listaDeColunasDaOrigem(d, campos) + ")"
}

// upsertSQLServer: MERGE com a linha nova como construtor VALUES.
//
// O ponto-e-vírgula final é obrigatório — sem ele o SQL Server recusa o comando
// (é a única sentença da linguagem com essa exigência).
func upsertSQLServer(d Dialect, schema string, entity EntityFields, campos []Field, marcas []string, chaves, atualizaveis []Field, ignorar bool) string {
	sql := "MERGE INTO " + qualifyFor(d, schema, entity.Name) + " AS " + apelidoAlvo +
		" USING (VALUES (" + strings.Join(marcas, ", ") + ")) AS " + apelidoOrigem +
		" (" + listaDeColunas(d, campos) + ")" +
		" ON (" + condicaoDeConflito(d, chaves) + ")"
	if !ignorar {
		sql += " WHEN MATCHED THEN UPDATE SET " + atribuicoesDoMerge(d, atualizaveis)
	}
	return sql + " WHEN NOT MATCHED THEN INSERT (" + listaDeColunas(d, campos) + ")" +
		" VALUES (" + listaDeColunasDaOrigem(d, campos) + ");"
}

// Apelidos do MERGE. Curtos e fixos: não há tabela do usuário no comando além do
// alvo, então não há com o que colidir.
const (
	apelidoAlvo   = "t"
	apelidoOrigem = "s"
)

func listaDeColunas(d Dialect, campos []Field) string {
	partes := make([]string, 0, len(campos))
	for _, campo := range campos {
		partes = append(partes, quoteIdentFor(d, campo.Column))
	}
	return strings.Join(partes, ", ")
}

func listaDeColunasDaOrigem(d Dialect, campos []Field) string {
	partes := make([]string, 0, len(campos))
	for _, campo := range campos {
		partes = append(partes, apelidoOrigem+"."+quoteIdentFor(d, campo.Column))
	}
	return strings.Join(partes, ", ")
}

// condicaoDeConflito é o ON do MERGE: alvo comparado com origem, coluna a coluna.
func condicaoDeConflito(d Dialect, chaves []Field) string {
	partes := make([]string, 0, len(chaves))
	for _, chave := range chaves {
		coluna := quoteIdentFor(d, chave.Column)
		partes = append(partes, apelidoAlvo+"."+coluna+" = "+apelidoOrigem+"."+coluna)
	}
	return strings.Join(partes, " AND ")
}

func atribuicoesDoMerge(d Dialect, campos []Field) string {
	partes := make([]string, 0, len(campos))
	for _, campo := range campos {
		coluna := quoteIdentFor(d, campo.Column)
		partes = append(partes, apelidoAlvo+"."+coluna+" = "+apelidoOrigem+"."+coluna)
	}
	return strings.Join(partes, ", ")
}

// camposDeConflito resolve as colunas da chave, validando que pertencem à
// entidade. Lista vazia cai na chave primária, que é o critério de identidade que
// a entidade já declara.
func camposDeConflito(entity EntityFields, chave []Column) ([]Field, error) {
	if len(chave) == 0 {
		chaves := entity.PrimaryKeys()
		if len(chaves) == 0 {
			return nil, errSemChavePrimaria(entity.Name)
		}
		return chaves, nil
	}
	campos := make([]Field, 0, len(chave))
	for _, coluna := range chave {
		campo := coluna.columnField()
		declarado, ok := campoDaEntidade(entity, campo)
		if !ok {
			return nil, errColunaDeOutraEntidade(campo.Column, entity.Name)
		}
		campos = append(campos, declarado)
	}
	return campos, nil
}

// campoDaEntidade acha o Field declarado que corresponde ao informado. A
// comparação inclui a tabela, não só o nome: "id" e "nome" repetem em quase toda
// entidade, e sem a tabela o Field de outra entidade passaria como se fosse desta.
func campoDaEntidade(entity EntityFields, campo Field) (Field, bool) {
	for _, declarado := range entity.Fields {
		if strings.EqualFold(declarado.Column, campo.Column) &&
			strings.EqualFold(declarado.Table, campo.Table) {
			return declarado, true
		}
	}
	return Field{}, false
}

func contemCampo(campos []Field, alvo Field) bool {
	for _, campo := range campos {
		if strings.EqualFold(campo.Column, alvo.Column) {
			return true
		}
	}
	return false
}

// semOsCampos devolve campos menos remover, preservando a ordem.
func semOsCampos(campos, remover []Field) []Field {
	saida := make([]Field, 0, len(campos))
	for _, campo := range campos {
		if !contemCampo(remover, campo) {
			saida = append(saida, campo)
		}
	}
	return saida
}
