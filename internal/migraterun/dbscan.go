package migraterun

// Leitura do esquema REAL do banco, para o caminho inverso do normal: em vez de
// declarar e aplicar, olhar o que já existe e oferecer a declaração.
//
// É o que sustenta projeto legado — banco cheio, nenhuma migration — e o
// "2) Ler Database" da criação de projeto.
//
// O schemaCache do executor também consulta o catálogo, mas com outro objetivo:
// ele só precisa saber se tabela e coluna existem, para decidir se aplica uma
// operação. Aqui é preciso o suficiente para ESCREVER a declaração: tipo com
// tamanho e escala, nulidade, chave primária, identidade e chave estrangeira.

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

// ColunaDoBanco é uma coluna como o catálogo do banco a descreve.
type ColunaDoBanco struct {
	Nome     string
	Tipo     string // tipo cru do banco, em minúsculas
	Tamanho  int
	Precisao int
	Escala   int
	Nulo     bool
	Identity bool
	// Default é a expressão como o banco a guarda, já sem os parênteses e aspas
	// que cada catálogo acrescenta de forma própria.
	Default string
}

// IndiceDoBanco é um índice comum: nem chave primária, nem unique constraint.
type IndiceDoBanco struct {
	Nome    string
	Colunas []string
	Unico   bool
}

// FKDoBanco é uma chave estrangeira já resolvida para coluna → pai.
type FKDoBanco struct {
	Coluna    string
	TabelaPai string
	ColunaPai string
}

// TabelaDoBanco é o retrato de uma tabela existente.
type TabelaDoBanco struct {
	Nome        string
	Colunas     []ColunaDoBanco
	PrimaryKey  []string
	ForeignKeys []FKDoBanco
	Indices     []IndiceDoBanco
}

// LerEsquemaDoBanco devolve as tabelas existentes, indexadas em minúsculas.
//
// Objeto de sistema fica de fora: o Oracle guarda tabelas internas no mesmo
// catálogo, e o SQL Server tem as suas. Sem esse filtro, um `import` num banco
// legado ofereceria escrever migration para tabela do próprio SGBD.
func LerEsquemaDoBanco(ctx context.Context, db *sql.DB, dialect, schema string) (map[string]TabelaDoBanco, error) {
	colunas, err := lerColunas(ctx, db, dialect, schema)
	if err != nil {
		return nil, err
	}
	if err := lerPrimaryKeys(ctx, db, dialect, schema, colunas); err != nil {
		return nil, err
	}
	if err := lerForeignKeys(ctx, db, dialect, schema, colunas); err != nil {
		return nil, err
	}
	if err := lerIndices(ctx, db, dialect, schema, colunas); err != nil {
		return nil, err
	}
	return colunas, nil
}

func lerColunas(ctx context.Context, db *sql.DB, dialect, schema string) (map[string]TabelaDoBanco, error) {
	var comando string
	var argumentos []any

	switch dialect {
	case "postgres":
		comando = `SELECT c.table_name, c.column_name, c.data_type,
			COALESCE(c.character_maximum_length, 0), COALESCE(c.numeric_precision, 0),
			COALESCE(c.numeric_scale, 0), c.is_nullable,
			CASE WHEN c.is_identity = 'YES' OR c.column_default LIKE 'nextval%' THEN 1 ELSE 0 END,
			CASE WHEN c.column_default LIKE 'nextval%' THEN '' ELSE COALESCE(c.column_default, '') END
			FROM information_schema.columns c
			JOIN information_schema.tables t
			  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
			WHERE c.table_schema = $1 AND t.table_type = 'BASE TABLE'
			ORDER BY c.table_name, c.ordinal_position`
		argumentos = []any{schemaOr(schema, "public")}

	case "mysql":
		comando = `SELECT c.table_name, c.column_name, c.data_type,
			COALESCE(c.character_maximum_length, 0), COALESCE(c.numeric_precision, 0),
			COALESCE(c.numeric_scale, 0), c.is_nullable,
			CASE WHEN c.extra LIKE '%auto_increment%' THEN 1 ELSE 0 END,
			COALESCE(c.column_default, '')
			FROM information_schema.columns c
			JOIN information_schema.tables t
			  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
			WHERE c.table_schema = DATABASE() AND t.table_type = 'BASE TABLE'
			ORDER BY c.table_name, c.ordinal_position`

	case "sqlserver":
		comando = `SELECT c.table_name, c.column_name, c.data_type,
			COALESCE(c.character_maximum_length, 0), COALESCE(c.numeric_precision, 0),
			COALESCE(c.numeric_scale, 0), c.is_nullable,
			COALESCE(COLUMNPROPERTY(OBJECT_ID(QUOTENAME(c.table_schema) + '.' + QUOTENAME(c.table_name)), c.column_name, 'IsIdentity'), 0),
			COALESCE(c.column_default, '')
			FROM information_schema.columns c
			JOIN information_schema.tables t
			  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
			WHERE c.table_schema = @p1 AND t.table_type = 'BASE TABLE'
			ORDER BY c.table_name, c.ordinal_position`
		argumentos = []any{schemaOr(schema, "dbo")}

	default:
		// O Oracle não tem is_nullable com 'YES'/'NO': é 'Y'/'N'. E identidade
		// aparece em identity_column, que só existe a partir do 12c.
		//
		// data_default é do tipo LONG, que não se pode cortar nem converter em SQL.
		// Vem cru e o driver o entrega como texto; a normalização acontece em Go.
		comando = `SELECT c.table_name, c.column_name, c.data_type,
			NVL(c.char_length, 0), NVL(c.data_precision, 0), NVL(c.data_scale, 0),
			c.nullable, CASE WHEN c.identity_column = 'YES' THEN 1 ELSE 0 END,
			c.data_default
			FROM all_tab_columns c
			JOIN all_tables t ON t.owner = c.owner AND t.table_name = c.table_name
			WHERE c.owner = :1
			ORDER BY c.table_name, c.column_id`
		argumentos = []any{strings.ToUpper(schema)}
	}

	rows, err := db.QueryContext(ctx, comando, argumentos...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tabelas := map[string]TabelaDoBanco{}
	for rows.Next() {
		var tabela, coluna, tipo, nulo string
		var tamanho, precisao, escala, identity int
		// O default vem como NULL na maioria das colunas, e o Oracle o entrega de um
		// campo LONG: sql.NullString evita depender de COALESCE em todos os quatro.
		var padrao sql.NullString
		if err := rows.Scan(&tabela, &coluna, &tipo, &tamanho, &precisao, &escala, &nulo, &identity, &padrao); err != nil {
			return nil, err
		}
		if objetoDeSistema(dialect, tabela) {
			continue
		}
		chave := strings.ToLower(tabela)
		atual := tabelas[chave]
		atual.Nome = tabela
		atual.Colunas = append(atual.Colunas, ColunaDoBanco{
			Nome:     coluna,
			Tipo:     strings.ToLower(tipo),
			Tamanho:  tamanho,
			Precisao: precisao,
			Escala:   escala,
			Nulo:     nulo == "YES" || nulo == "Y",
			Identity: identity == 1,
			Default:  normalizaDefault(dialect, padrao.String, identity == 1),
		})
		tabelas[chave] = atual
	}
	return tabelas, rows.Err()
}

func lerPrimaryKeys(ctx context.Context, db *sql.DB, dialect, schema string, tabelas map[string]TabelaDoBanco) error {
	var comando string
	var argumentos []any

	switch dialect {
	case "postgres":
		comando = `SELECT tc.table_name, kcu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
			  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
			WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = $1
			ORDER BY tc.table_name, kcu.ordinal_position`
		argumentos = []any{schemaOr(schema, "public")}

	case "mysql":
		comando = `SELECT table_name, column_name FROM information_schema.key_column_usage
			WHERE constraint_name = 'PRIMARY' AND table_schema = DATABASE()
			ORDER BY table_name, ordinal_position`

	case "sqlserver":
		comando = `SELECT tc.table_name, kcu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
			  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
			WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = @p1
			ORDER BY tc.table_name, kcu.ordinal_position`
		argumentos = []any{schemaOr(schema, "dbo")}

	default:
		comando = `SELECT c.table_name, cc.column_name
			FROM all_constraints c
			JOIN all_cons_columns cc ON cc.owner = c.owner AND cc.constraint_name = c.constraint_name
			WHERE c.constraint_type = 'P' AND c.owner = :1
			ORDER BY c.table_name, cc.position`
		argumentos = []any{strings.ToUpper(schema)}
	}

	rows, err := db.QueryContext(ctx, comando, argumentos...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tabela, coluna string
		if err := rows.Scan(&tabela, &coluna); err != nil {
			return err
		}
		chave := strings.ToLower(tabela)
		atual, existe := tabelas[chave]
		if !existe {
			continue
		}
		atual.PrimaryKey = append(atual.PrimaryKey, coluna)
		tabelas[chave] = atual
	}
	return rows.Err()
}

func lerForeignKeys(ctx context.Context, db *sql.DB, dialect, schema string, tabelas map[string]TabelaDoBanco) error {
	var comando string
	var argumentos []any

	switch dialect {
	case "postgres":
		comando = `SELECT tc.table_name, kcu.column_name, ccu.table_name, ccu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
			  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
			JOIN information_schema.constraint_column_usage ccu
			  ON ccu.constraint_name = tc.constraint_name AND ccu.table_schema = tc.table_schema
			WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = $1
			ORDER BY tc.table_name, kcu.ordinal_position`
		argumentos = []any{schemaOr(schema, "public")}

	case "mysql":
		comando = `SELECT table_name, column_name, referenced_table_name, referenced_column_name
			FROM information_schema.key_column_usage
			WHERE referenced_table_name IS NOT NULL AND table_schema = DATABASE()
			ORDER BY table_name, ordinal_position`

	case "sqlserver":
		comando = `SELECT OBJECT_NAME(fk.parent_object_id), pc.name,
			OBJECT_NAME(fk.referenced_object_id), rc.name
			FROM sys.foreign_keys fk
			JOIN sys.foreign_key_columns fkc ON fkc.constraint_object_id = fk.object_id
			JOIN sys.columns pc ON pc.object_id = fkc.parent_object_id AND pc.column_id = fkc.parent_column_id
			JOIN sys.columns rc ON rc.object_id = fkc.referenced_object_id AND rc.column_id = fkc.referenced_column_id
			ORDER BY OBJECT_NAME(fk.parent_object_id)`

	default:
		comando = `SELECT c.table_name, cc.column_name, rc.table_name, rc.column_name
			FROM all_constraints c
			JOIN all_cons_columns cc ON cc.owner = c.owner AND cc.constraint_name = c.constraint_name
			JOIN all_cons_columns rc ON rc.owner = c.r_owner AND rc.constraint_name = c.r_constraint_name
			  AND rc.position = cc.position
			WHERE c.constraint_type = 'R' AND c.owner = :1
			ORDER BY c.table_name, cc.position`
		argumentos = []any{strings.ToUpper(schema)}
	}

	rows, err := db.QueryContext(ctx, comando, argumentos...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tabela, coluna, pai, colunaPai string
		if err := rows.Scan(&tabela, &coluna, &pai, &colunaPai); err != nil {
			return err
		}
		chave := strings.ToLower(tabela)
		atual, existe := tabelas[chave]
		if !existe {
			continue
		}
		atual.ForeignKeys = append(atual.ForeignKeys, FKDoBanco{Coluna: coluna, TabelaPai: pai, ColunaPai: colunaPai})
		tabelas[chave] = atual
	}
	return rows.Err()
}

// objetoDeSistema descarta o que é do próprio SGBD, e também as duas tabelas de
// controle do gokit: elas são criadas pelo motor, não declaradas em migration.
func objetoDeSistema(dialect, tabela string) bool {
	nome := strings.ToLower(tabela)
	if strings.HasPrefix(nome, "migrations_") || strings.HasPrefix(nome, "seeders_") {
		return true
	}
	switch dialect {
	case "oracle":
		for _, prefixo := range []string{"bin$", "sys_", "aq$", "def$", "logmnr", "mview$", "sqlplus_", "schema_version", "helper$", "redo_", "roll", "olapt"} {
			if strings.HasPrefix(nome, prefixo) {
				return true
			}
		}
	case "sqlserver":
		for _, exato := range []string{"sysdiagrams", "database_firewall_rules"} {
			if nome == exato {
				return true
			}
		}
	}
	return false
}

// IdentidadeForaDaChave devolve as colunas de identidade que NÃO estão na chave
// primária da tabela.
//
// Isso é legal no SQL Server, no Oracle e no Postgres, e ILEGAL no MySQL, que
// exige que a coluna auto-incremento seja chave — ERROR 1075, medido. Num schema
// real de 755 tabelas havia 235 colunas nessa situação, então não é caso de
// canto: é o que decide se o corpus importado aplica nos quatro bancos.
func IdentidadeForaDaChave(tabela TabelaDoBanco) []string {
	if len(tabela.Colunas) == 0 {
		return nil
	}
	naChave := map[string]bool{}
	for _, coluna := range tabela.PrimaryKey {
		naChave[strings.ToLower(coluna)] = true
	}
	var fora []string
	for _, coluna := range tabela.Colunas {
		if coluna.Identity && !naChave[strings.ToLower(coluna.Nome)] {
			fora = append(fora, strings.ToLower(coluna.Nome))
		}
	}
	return fora
}

// NomesOrdenados devolve as tabelas em ordem estável, para o relatório não mudar
// de execução para execução.
func NomesOrdenados(tabelas map[string]TabelaDoBanco) []string {
	nomes := make([]string, 0, len(tabelas))
	for nome := range tabelas {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)
	return nomes
}

// normalizaDefault descasca o que cada catálogo acrescenta em volta da expressão.
//
// Os quatro guardam a MESMA declaração de formas diferentes, e sem descascar o
// corpus importado ficaria preso ao banco de origem:
//
//	DEFAULT 0            → mysql "0"   postgres "0"   oracle "0 "     sqlserver "((0))"
//	DEFAULT 'A'          → mysql "A"   postgres "'A'::character varying"  sqlserver "('A')"
//	DEFAULT current_ts   → mysql "CURRENT_TIMESTAMP"  postgres "now()"  oracle "SYSDATE"
//
// A sequência de identidade NÃO é default: ela chega como `nextval(...)` no
// Postgres e como identidade nos outros. Declará-la faria a coluna ganhar um
// default inexistente ao lado do AutoIncrement.
func normalizaDefault(dialect, valor string, identidade bool) string {
	valor = strings.TrimSpace(valor)
	if valor == "" || identidade {
		return ""
	}

	// O SQL Server embrulha em parênteses, às vezes dois: ((0)) e ('A').
	for strings.HasPrefix(valor, "(") && strings.HasSuffix(valor, ")") {
		interno := strings.TrimSpace(valor[1 : len(valor)-1])
		if interno == "" {
			break
		}
		valor = interno
	}

	// O Postgres anexa o tipo: 'A'::character varying.
	if corte := strings.Index(valor, "::"); corte > 0 {
		valor = strings.TrimSpace(valor[:corte])
	}

	// Sequência de identidade não é default.
	if baixo := strings.ToLower(valor); strings.HasPrefix(baixo, "nextval(") {
		return ""
	}

	// "Agora" tem um nome diferente em cada banco, e o DSL tem um só. Sem esta
	// tabela, a mesma coluna sairia com CURRENT_TIMESTAMP lida do MySQL, do Oracle
	// e do Postgres, e com getdate() lida do SQL Server — corpus divergente por
	// causa de sinônimo. Medido: os três primeiros já concordam, o SQL Server é o
	// que difere.
	switch strings.ToLower(strings.TrimSuffix(strings.TrimSpace(valor), "()")) {
	case "current_timestamp", "getdate", "now", "sysdate", "systimestamp",
		"localtimestamp", "getutcdate", "sysutcdatetime", "current_date":
		return expressaoDeAgora
	}

	// Aspas simples do literal de texto saem: o DSL recebe o valor, não o SQL.
	if len(valor) >= 2 && strings.HasPrefix(valor, "'") && strings.HasSuffix(valor, "'") {
		valor = strings.ReplaceAll(valor[1:len(valor)-1], "''", "'")
	}
	return strings.TrimSpace(valor)
}

// expressaoDeAgora é a forma única para "o instante atual". O executor já traduz
// CURRENT_TIMESTAMP para o que cada dialeto entende.
const expressaoDeAgora = "CURRENT_TIMESTAMP"

// EhExpressao diz se o default é expressão de SQL, e não valor literal. É o que
// decide entre .DefaultExpr() e .Default() na migration gerada: escrever
// CURRENT_TIMESTAMP como literal gravaria a string "CURRENT_TIMESTAMP" na coluna.
func EhExpressao(valor string) bool {
	if valor == expressaoDeAgora {
		return true
	}
	// Chamada de função e operador só existem em expressão; valor literal que o
	// catálogo devolve já vem sem aspas neste ponto.
	return strings.ContainsAny(valor, "()+-*/|") && valor != ""
}

// lerIndices busca os índices comuns — os que NÃO são chave primária nem unique
// constraint, porque esses dois já vêm declarados na própria coluna ou na chave.
func lerIndices(ctx context.Context, db *sql.DB, dialect, schema string, tabelas map[string]TabelaDoBanco) error {
	var comando string
	var argumentos []any

	switch dialect {
	case "postgres":
		// `attnum = ANY(indkey)` perde a ORDEM das colunas do índice: ele casa o
		// conjunto e a ordenação acaba sendo a da tabela. Índice composto (a, b) não
		// é o mesmo que (b, a), então a posição vem de unnest WITH ORDINALITY, que
		// preserva a ordem do indkey.
		comando = `SELECT t.relname, i.relname, a.attname, CASE WHEN ix.indisunique THEN 1 ELSE 0 END
			FROM pg_class t
			JOIN pg_namespace n ON n.oid = t.relnamespace
			JOIN pg_index ix ON ix.indrelid = t.oid
			JOIN pg_class i ON i.oid = ix.indexrelid
			JOIN LATERAL unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord) ON true
			JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
			WHERE n.nspname = $1 AND t.relkind = 'r' AND NOT ix.indisprimary
			  AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = i.oid AND c.contype = 'u')
			ORDER BY t.relname, i.relname, k.ord`
		argumentos = []any{schemaOr(schema, "public")}

	case "mysql":
		comando = `SELECT s.table_name, s.index_name, s.column_name, CASE WHEN s.non_unique = 0 THEN 1 ELSE 0 END
			FROM information_schema.statistics s
			WHERE s.table_schema = DATABASE() AND s.index_name <> 'PRIMARY'
			  AND NOT EXISTS (SELECT 1 FROM information_schema.table_constraints tc
			      WHERE tc.table_schema = s.table_schema AND tc.table_name = s.table_name
			        AND tc.constraint_name = s.index_name AND tc.constraint_type = 'UNIQUE')
			ORDER BY s.table_name, s.index_name, s.seq_in_index`

	case "sqlserver":
		comando = `SELECT t.name, i.name, c.name, CAST(i.is_unique AS INT)
			FROM sys.indexes i
			JOIN sys.tables t ON t.object_id = i.object_id
			JOIN sys.schemas s ON s.schema_id = t.schema_id
			JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
			JOIN sys.columns c ON c.object_id = ic.object_id AND c.column_id = ic.column_id
			WHERE s.name = @p1 AND i.is_primary_key = 0 AND i.is_unique_constraint = 0 AND i.type > 0
			ORDER BY t.name, i.name, ic.key_ordinal`
		argumentos = []any{schemaOr(schema, "dbo")}

	default:
		comando = `SELECT i.table_name, i.index_name, c.column_name,
			CASE WHEN i.uniqueness = 'UNIQUE' THEN 1 ELSE 0 END
			FROM all_indexes i
			JOIN all_ind_columns c ON c.index_owner = i.owner AND c.index_name = i.index_name
			WHERE i.owner = :1 AND NOT EXISTS (
				SELECT 1 FROM all_constraints k WHERE k.owner = i.owner
				  AND k.index_name = i.index_name AND k.constraint_type IN ('P', 'U'))
			ORDER BY i.table_name, i.index_name, c.column_position`
		argumentos = []any{strings.ToUpper(schema)}
	}

	rows, err := db.QueryContext(ctx, comando, argumentos...)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Índice de várias colunas chega em várias linhas: a posição da coluna vem do
	// ORDER BY, então basta anexar na ordem em que aparece.
	posicao := map[string]map[string]int{}
	for rows.Next() {
		var tabela, indice, coluna string
		var unico int
		if err := rows.Scan(&tabela, &indice, &coluna, &unico); err != nil {
			return err
		}
		chave := strings.ToLower(tabela)
		atual, existe := tabelas[chave]
		if !existe {
			continue
		}
		// O Oracle devolve nome de índice em MAIÚSCULAS, como todo identificador não
		// citado. O corpus é sempre em minúsculas.
		indice = strings.ToLower(indice)
		if posicao[chave] == nil {
			posicao[chave] = map[string]int{}
		}
		if onde, tem := posicao[chave][indice]; tem {
			atual.Indices[onde].Colunas = append(atual.Indices[onde].Colunas, strings.ToLower(coluna))
		} else {
			posicao[chave][indice] = len(atual.Indices)
			atual.Indices = append(atual.Indices, IndiceDoBanco{
				Nome:    indice,
				Colunas: []string{strings.ToLower(coluna)},
				Unico:   unico == 1,
			})
		}
		tabelas[chave] = atual
	}
	return rows.Err()
}

// ViewDoBanco é uma view existente, com o texto da definição como o banco a guarda.
type ViewDoBanco struct {
	Nome string
	SQL  string
}

// LerViewsDoBanco devolve as views do schema, indexadas em minúsculas.
//
// O texto vem do dialeto de origem e NÃO é convertido: `TOP` do SQL Server,
// `ROWNUM` do Oracle e `LIMIT` dos outros dois não têm tradução automática confiável,
// e converter errado gravaria mentira num arquivo de histórico. Quem consome grava
// o texto no arquivo do dialeto de onde ele veio.
func LerViewsDoBanco(ctx context.Context, db *sql.DB, dialect, schema string) (map[string]ViewDoBanco, error) {
	var comando string
	var argumentos []any

	switch dialect {
	case "postgres":
		comando = `SELECT table_name, view_definition FROM information_schema.views
			WHERE table_schema = $1 ORDER BY table_name`
		argumentos = []any{schemaOr(schema, "public")}

	case "mysql":
		comando = `SELECT table_name, view_definition FROM information_schema.views
			WHERE table_schema = DATABASE() ORDER BY table_name`

	case "sqlserver":
		// sys.sql_modules traz o CREATE VIEW inteiro; information_schema.views trunca
		// a definição em 4000 caracteres, e view de relatório passa disso fácil.
		comando = `SELECT o.name, m.definition
			FROM sys.sql_modules m
			JOIN sys.objects o ON o.object_id = m.object_id
			JOIN sys.schemas s ON s.schema_id = o.schema_id
			WHERE o.type = 'V' AND s.name = @p1
			ORDER BY o.name`
		argumentos = []any{schemaOr(schema, "dbo")}

	default:
		// all_views.text é LONG: não se corta nem converte em SQL, vem cru.
		comando = `SELECT view_name, text FROM all_views WHERE owner = :1 ORDER BY view_name`
		argumentos = []any{strings.ToUpper(schema)}
	}

	rows, err := db.QueryContext(ctx, comando, argumentos...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	views := map[string]ViewDoBanco{}
	for rows.Next() {
		var nome string
		var definicao sql.NullString
		if err := rows.Scan(&nome, &definicao); err != nil {
			return nil, err
		}
		if objetoDeSistema(dialect, nome) {
			continue
		}
		views[strings.ToLower(nome)] = ViewDoBanco{
			Nome: nome,
			SQL:  normalizaDefinicaoDeView(definicao.String),
		}
	}
	return views, rows.Err()
}

// normalizaDefinicaoDeView deixa só o corpo da consulta.
//
// O SQL Server e o Oracle devolvem o `CREATE VIEW x AS ...` inteiro; o MySQL e o
// Postgres devolvem só o SELECT. O arquivo do gokit guarda o corpo, porque é o
// executor que monta o CREATE VIEW de cada dialeto — guardar o CREATE junto faria o
// DDL sair duplicado.
func normalizaDefinicaoDeView(texto string) string {
	limpo := strings.TrimSpace(texto)
	if limpo == "" {
		return ""
	}
	// Procura o ` AS ` que separa o cabeçalho do corpo, e só quando o texto começa
	// com CREATE: um SELECT que já vem sem cabeçalho tem `AS` de apelido de coluna,
	// e cortar ali destruiria a consulta.
	alto := strings.ToUpper(limpo)
	if strings.HasPrefix(alto, "CREATE") {
		if corte := indiceDoAsDeView(alto); corte > 0 {
			limpo = strings.TrimSpace(limpo[corte:])
		}
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(limpo), ";"))
}

// indiceDoAsDeView acha o fim do primeiro ` AS ` do cabeçalho, ignorando o que
// estiver dentro de parênteses (lista de colunas da view).
func indiceDoAsDeView(alto string) int {
	profundidade := 0
	for posicao := 0; posicao < len(alto)-3; posicao++ {
		switch alto[posicao] {
		case '(':
			profundidade++
		case ')':
			profundidade--
		}
		if profundidade == 0 && alto[posicao] == ' ' && strings.HasPrefix(alto[posicao:], " AS ") {
			return posicao + 4
		}
	}
	return -1
}
