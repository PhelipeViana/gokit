package migraterun

// Criação da chave estrangeira declarada na própria coluna.
//
// O DSL permite duas formas de declarar a mesma coisa:
//
//	migrate.Col("cidade_id").Integer().References("cidades", "id")   // na coluna
//	migrate.AddForeignKey(alias.Users, "fk_users_cidade", ...)        // operação
//
// A segunda sempre funcionou. A primeira era registrada no plano, alimentava o
// grafo de relações da ORM e a ordenação das factories — e **não criava constraint
// nenhuma no banco**. Ficou silenciosa porque só aparece quando alguém tenta
// violar a integridade: até então, tudo parece certo.
//
// A correção mora no executor, e não no Expandir do plano, por um motivo
// concreto: o checksum da migration é calculado sobre as operações expandidas.
// Acrescentar operações lá mudaria o checksum de toda migration já aplicada e
// dispararia drift em todo projeto existente. Aqui o plano segue idêntico e só o
// DDL emitido muda.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// foreignKeySQL monta o ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY.
//
// É o único lugar que escreve esse DDL: a operação explícita e a coluna com
// References passam as duas por aqui, então nome de constraint, ON DELETE e
// quoting por dialeto não podem divergir entre as duas formas.
func foreignKeySQL(dialect, schema, table string, fk acao.ForeignKey) string {
	name := fk.ConstraintName
	if name == "" {
		name = foreignKeyName(table, fk.Column)
	}
	onDelete := ""
	if fk.OnDelete != "" {
		onDelete = " ON DELETE " + fk.OnDelete
	}
	columns, references := fk.Columns, fk.ReferenceColumns
	if len(columns) == 0 {
		columns = []string{fk.Column}
	}
	if len(references) == 0 {
		references = []string{fk.ReferenceColumn}
	}
	return fmt.Sprintf(
		"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)%s",
		qualified(dialect, schema, table), quote(dialect, name),
		strings.Join(quotedColumns(dialect, columns), ", "),
		qualified(dialect, schema, fk.ReferenceTable),
		strings.Join(quotedColumns(dialect, references), ", "), onDelete,
	)
}

// foreignKeyDaColuna traduz o References declarado na coluna para o mesmo
// ForeignKey que a operação explícita usa. Coluna sem referência devolve false.
func foreignKeyDaColuna(column acao.ColunaDefinicao) (acao.ForeignKey, bool) {
	if strings.TrimSpace(column.ReferenceTable) == "" {
		return acao.ForeignKey{}, false
	}
	referencia := column.ReferenceColumn
	if strings.TrimSpace(referencia) == "" {
		// Coluna de destino omitida significa a chave primária do destino, e por
		// convenção do gokit ela se chama id. O CreateTable do destino é validado
		// antes, então a coluna existe.
		referencia = "id"
	}
	return acao.ForeignKey{
		Column:          column.Name,
		ReferenceTable:  column.ReferenceTable,
		ReferenceColumn: referencia,
		ConstraintName:  column.ConstraintName,
		OnDelete:        column.OnDelete,
	}, true
}

// criarForeignKeysDasColunas cria as constraints das colunas que declararam
// References.
//
// Idempotência: a constraint pode já existir de uma execução anterior que falhou
// no meio, e nos quatro bancos "constraint duplicada" é erro. Em vez de consultar
// o catálogo de cada banco — quatro consultas diferentes — a criação tolera
// especificamente o erro de duplicidade e falha em qualquer outro.
func criarForeignKeysDasColunas(ctx context.Context, db *sql.DB, dialect, schema, table string, columns []acao.ColunaDefinicao) error {
	for _, column := range columns {
		fk, tem := foreignKeyDaColuna(column)
		if !tem {
			continue
		}
		// FK tautológica: a coluna referencia ELA MESMA, na própria tabela.
		//
		// Não restringe nada — uma coluna sempre é igual a si mesma —, então pular não perde
		// integridade. O SQL Server aceita e guarda a constraint inútil; o MySQL recusa com
		// "Missing unique key for constraint", mensagem que manda procurar uma chave única que
		// existe. Medido no corpus legado: exatamente 1 caso em 754 tabelas,
		// `CONFIG_PLANO_fk4` (CONFIG_PLANO.CONFIG_PLANO_ID -> CONFIG_PLANO.CONFIG_PLANO_ID),
		// que existe de verdade na origem.
		//
		// Pulada nos QUATRO, não só no MySQL: o objetivo é o mesmo corpus produzir o mesmo
		// schema, e criar em três uma constraint que não restringe nada só carrega o ruído.
		if fkTautologica(table, fk) {
			avisarFKTautologica(table, fk.Column)
			continue
		}
		query := foreignKeySQL(dialect, schema, table, fk)
		if _, err := db.ExecContext(ctx, query); err != nil {
			if constraintJaExiste(err) {
				continue
			}
			return i18n.Errf("run_fk_create_failed", table, fk.Column, err)
		}
	}
	return nil
}

// constraintJaExiste reconhece a recusa por constraint de mesmo nome nos quatro
// bancos. É a única classe de erro tolerada na criação da FK — qualquer outra
// (tipo incompatível, tabela inexistente) tem de interromper a migration.
func constraintJaExiste(err error) bool {
	mensagem := strings.ToLower(err.Error())
	for _, trecho := range []string{
		"already exists",        // Postgres, SQL Server
		"duplicate foreign key", // MySQL
		"duplicate key name",    // MySQL
		"ora-02275",             // Oracle: constraint referencial já existe nesta tabela
		"ora-00955",             // Oracle: nome já usado por objeto existente
		"errno: 121",            // MySQL: nome de constraint duplicado
	} {
		if strings.Contains(mensagem, trecho) {
			return true
		}
	}
	return false
}

// ConciliarForeignKeys cria as constraints declaradas no corpus que não existem
// no banco.
//
// Existe porque a correção do bug não se propaga sozinha: migration já aplicada
// não roda de novo, então todo banco criado antes desta versão ficaria sem FK
// para sempre. A conciliação varre o corpus inteiro e tenta criar cada FK
// declarada; a que já existe é ignorada pelo mesmo tratamento de duplicidade da
// criação normal.
//
// É idempotente por construção, então pode rodar em toda migração sem custo além
// de um ALTER recusado por constraint.
func conciliarForeignKeys(ctx context.Context, db *sql.DB, dialect, schema string, files []migrationFile, history map[string]string) (int, error) {
	criadas := 0
	for _, file := range files {
		// Só migration JÁ APLICADA entra na conciliação, e é por definição: o que ela
		// conserta é FK ausente em tabela que já existe. Migration pendente cria a
		// própria FK quando rodar, e a tabela dela ainda não existe.
		//
		// Sem este filtro, `migrate run` em banco NOVO tentava criar a FK antes do
		// CREATE TABLE e abortava — em qualquer dialeto, com qualquer corpus que
		// tivesse um .References(). Não aparecia porque o projeto de teste já estava
		// migrado (a conciliação achava tudo no lugar e não fazia nada); só quebra na
		// execução limpa. Medido com um corpus de duas migrations.
		if !aplicada(file, history) {
			continue
		}
		for _, operation := range file.Plan.Operations {
			colunas := operation.Columns
			if operation.Column != nil {
				colunas = []acao.ColunaDefinicao{*operation.Column}
			}
			switch operation.Kind {
			case string(acao.CreateTable), string(acao.AddColumn):
			default:
				continue
			}
			for _, column := range colunas {
				fk, tem := foreignKeyDaColuna(column)
				if !tem {
					continue
				}
				// Tautológica: mesma razão da criação normal. Aqui sem avisar de novo — o
				// aviso já saiu quando a tabela foi criada, e repetir a cada run poluiria.
				if fkTautologica(operation.Table, fk) {
					continue
				}
				// Conferir ANTES de tentar, em vez de depender do texto do erro.
				//
				// O SQL Server recusa a constraint duplicada com "Could not create
				// constraint or index. See previous errors." — genérico, sem dizer
				// que já existe. Acrescentar essa frase à lista de tolerância
				// mascararia a falha que mais importa: FK sobre dado órfão, que
				// chega com a MESMA mensagem. Medido importando um banco que já
				// tinha as FKs: MySQL, Postgres e Oracle toleraram, o SQL Server
				// abortou a migração inteira.
				existe, err := constraintExiste(ctx, db, dialect, schema, nomeDaConstraint(operation.Table, fk))
				if err != nil {
					return criadas, i18n.Errf("run_fk_create_failed", operation.Table, fk.Column, err)
				}
				if existe {
					continue
				}
				query := foreignKeySQL(dialect, schema, operation.Table, fk)
				if _, err := db.ExecContext(ctx, query); err != nil {
					// A tolerância por mensagem continua, agora só como rede para a
					// corrida entre a checagem e o ALTER.
					if constraintJaExiste(err) {
						continue
					}
					return criadas, i18n.Errf("run_fk_create_failed", operation.Table, fk.Column, err)
				}
				criadas++
			}
		}
	}
	return criadas, nil
}

// aplicada diz se a migration já consta no histórico. As duas chaves são
// consultadas porque o histórico antigo era indexado pelo NOME do arquivo, e o atual
// pelo ID — o mesmo par que o laço de aplicação verifica.
func aplicada(file migrationFile, history map[string]string) bool {
	if _, tem := history[file.ID]; tem {
		return true
	}
	_, tem := history[file.Name]
	return tem
}

// nomeDaConstraint devolve o nome que foreignKeySQL usaria, para a checagem
// prévia olhar exatamente o mesmo objeto que o ALTER criaria.
func nomeDaConstraint(table string, fk acao.ForeignKey) string {
	if fk.ConstraintName != "" {
		return fk.ConstraintName
	}
	return foreignKeyName(table, fk.Column)
}

// constraintExiste pergunta ao catálogo se a constraint já está lá.
//
// O escopo do nome difere entre os bancos e é isso que a consulta reflete: no
// Oracle e no SQL Server o nome é único por SCHEMA, no MySQL e no Postgres é por
// TABELA. Consultar pelo nome no escopo certo é o que evita tanto o falso positivo
// quanto o falso negativo.
func constraintExiste(ctx context.Context, db *sql.DB, dialect, schema, nome string) (bool, error) {
	var comando string
	var argumentos []any

	switch dialect {
	case "postgres":
		comando = `SELECT COUNT(*) FROM information_schema.table_constraints
			WHERE constraint_schema = $1 AND constraint_name = $2`
		argumentos = []any{schemaOr(schema, "public"), nome}
	case "mysql":
		comando = `SELECT COUNT(*) FROM information_schema.table_constraints
			WHERE constraint_schema = DATABASE() AND constraint_name = ?`
		argumentos = []any{nome}
	case "sqlserver":
		comando = `SELECT COUNT(*) FROM sys.objects o
			JOIN sys.schemas s ON s.schema_id = o.schema_id
			WHERE s.name = @p1 AND o.name = @p2`
		argumentos = []any{schemaOr(schema, "dbo"), nome}
	default:
		// O Oracle guarda o nome em maiúsculas, como todo identificador não citado.
		comando = `SELECT COUNT(*) FROM all_constraints WHERE owner = :1 AND constraint_name = :2`
		argumentos = []any{strings.ToUpper(schema), strings.ToUpper(nome)}
	}

	var total int
	if err := db.QueryRowContext(ctx, comando, argumentos...).Scan(&total); err != nil {
		return false, err
	}
	return total > 0, nil
}

// motivoImpossivel devolve a chave i18n do motivo, ou vazio se o erro não é uma recusa
// estrutural de índice.
func motivoImpossivel(err error) string {
	chave, _ := indiceEstruturalmenteImpossivel(err)
	return chave
}

// indiceEstruturalmenteImpossivel reconhece a recusa de índice que é LIMITE DO PRODUTO,
// em qualquer um dos quatro, e devolve a chave da mensagem que explica cada uma.
//
// São limites do motor de banco, não defeito da declaração: nenhum ajuste no corpus faz
// um índice de 58 colunas caber onde o teto é 32, nem indexar um LOB.
//
// Vale só para índice NÃO único — a decisão de tolerar é de quem chama. Índice comum é
// desempenho; único é integridade, e integridade não se tolera perder.
//
// Isto já foi escrito como "impossível no Oracle", com o comentário afirmando que "os
// três outros dialetos aceitam". Medido no corpus legado de 754 tabelas: **o SQL Server
// recusa igual**, com teto de 32 colunas na CHAVE do índice. A afirmação era suposição, e
// o efeito era o corpus morrer no primeiro índice largo do dialeto de ORIGEM. Cada marca
// abaixo veio de execução, não de documentação.
func indiceEstruturalmenteImpossivel(err error) (string, bool) {
	mensagem := strings.ToLower(err.Error())
	switch {
	// ── teto de colunas na chave ───────────────────────────────────────────────────
	case strings.Contains(mensagem, "ora-01793"):
		// Oracle: máximo de 32 colunas por índice.
		return "run_index_skip_too_many", true
	case strings.Contains(mensagem, "maximum limit for index key column list"):
		// SQL Server: mesmo teto de 32, e a mensagem diz quantas foram pedidas.
		return "run_index_skip_too_many", true
	case strings.Contains(mensagem, "too many key parts specified"):
		// MySQL: teto de 16 — o mais baixo dos quatro.
		return "run_index_skip_too_many", true
	case strings.Contains(mensagem, "cannot use more than") && strings.Contains(mensagem, "columns in an index"):
		// Postgres: teto de 32.
		return "run_index_skip_too_many", true

	// ── tipo que não se indexa ─────────────────────────────────────────────────────
	case strings.Contains(mensagem, "ora-02327"):
		// Oracle: coluna de tipo LOB. Chega aqui porque VARCHAR acima de 4000 no SQL
		// Server não cabe em VARCHAR2 e vira CLOB — e CLOB não se indexa.
		return "run_index_skip_lob", true
	case strings.Contains(mensagem, "is of a type that is invalid for use as a key column"):
		// SQL Server: text/ntext/image/xml/varchar(max) na chave.
		return "run_index_skip_lob", true
	}
	return "", false
}

// indiceRedundante reconhece a recusa de índice cuja lista de colunas JÁ está
// indexada — não por nome duplicado, mas por conteúdo duplicado.
//
// Só o Oracle recusa isso; os outros três criam o segundo índice sem reclamar. É a
// divergência mais comum ao levar schema legado do SQL Server para o Oracle, porque a
// redundância se acumula ao longo de anos: o índice da FK e o índice "manual" sobre a
// mesma coluna.
//
// Nome duplicado NÃO entra aqui: aquilo é ORA-00955 e continua sendo erro, porque
// significa outro objeto ocupando o nome — situação diferente e que o usuário resolve.
// São dois códigos porque são dois níveis do mesmo fato:
//
//	ORA-01408  segundo ÍNDICE sobre a mesma lista de colunas
//	ORA-02261  segunda CONSTRAINT de unicidade sobre a mesma lista
//
// O segundo passou a aparecer quando o índice único começou a ser criado como
// constraint no Oracle — a mudança que a FK exige. Nos dois casos a unicidade da lista
// está garantida; o que não existe é o objeto com o nome declarado.
func indiceRedundante(err error) bool {
	mensagem := strings.ToLower(err.Error())
	return strings.Contains(mensagem, "ora-01408") || strings.Contains(mensagem, "ora-02261")
}

// avisarIndiceRedundante conta o que foi tolerado. Sai no fim do run, agrupado.
//
// O motivo entra na linha porque os dois casos tolerados têm consequências
// diferentes: redundante significa "a lista está indexada, só com outro nome";
// excedeu o limite significa "não há índice nenhum aqui".
func avisarIndiceRedundante(nome, tabela string, colunas []string, motivo string) {
	indicesRedundantes = append(indicesRedundantes, fmt.Sprintf("%s em %s(%d coluna(s)) — %s",
		nome, strings.ToLower(tabela), len(colunas), motivo))
}

// indicesRedundantes acumula os índices que o banco recusou por redundância.
//
// Variável de pacote porque o executor de operação não carrega um coletor — e a
// alternativa, propagar um por toda a cadeia de execução, tocaria dezenas de
// assinaturas para um aviso. É lida e zerada por quem inicia o run.
var indicesRedundantes []string

// IndicesRedundantes devolve e ZERA a lista acumulada.
func IndicesRedundantes() []string {
	saida := indicesRedundantes
	indicesRedundantes = nil
	return saida
}

// fkTautologica diz se a chave estrangeira aponta a coluna para ela mesma.
//
// Mesma tabela E mesma coluna. Auto-referência LEGÍTIMA — `pai_id` apontando para `id` na
// mesma tabela, que é como se modela hierarquia — não entra aqui: ali a restrição existe e
// vale.
func fkTautologica(tabela string, fk acao.ForeignKey) bool {
	return strings.EqualFold(tabela, fk.ReferenceTable) &&
		strings.EqualFold(fk.Column, fk.ReferenceColumn)
}

// fksTautologicas acumula as puladas, para o relatório do fim do run.
var fksTautologicas []string

func avisarFKTautologica(tabela, coluna string) {
	fksTautologicas = append(fksTautologicas,
		strings.ToLower(tabela)+"."+strings.ToLower(coluna))
}

// FKsTautologicas devolve e ZERA a lista acumulada.
func FKsTautologicas() []string {
	saida := fksTautologicas
	fksTautologicas = nil
	return saida
}
