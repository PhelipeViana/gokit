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
func conciliarForeignKeys(ctx context.Context, db *sql.DB, dialect, schema string, files []migrationFile) (int, error) {
	criadas := 0
	for _, file := range files {
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
				query := foreignKeySQL(dialect, schema, operation.Table, fk)
				if _, err := db.ExecContext(ctx, query); err != nil {
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
