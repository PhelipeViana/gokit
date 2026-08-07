// Package migrate is the public, autocomplete-friendly migration DSL.
package migrate

import (
	"strings"
	"time"

	"gokit/migration/acao"
)

// SeedTimeLayout é o formato aceito por Time em um seed.
const SeedTimeLayout = "2006-01-02 15:04:05"

// Time declara um valor de data/hora numa linha de seed.
//
//	{"id": 1, "created_at": migrate.Time("2026-07-01 18:06:57")}
//
// Sem isso o valor iria como texto e dependeria da conversão implícita do
// banco, que no Oracle varia conforme o NLS da sessão.
func Time(value string) time.Time {
	parsed, err := time.Parse(SeedTimeLayout, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

type Table string
type View struct{ name string }
type Column = acao.Coluna
type Operation = acao.Operacao

// Row é uma linha de seed: coluna -> valor literal.
type Row = acao.Linha

// Rows é o seed fixo de uma tabela, declarado ao lado do CreateTable em uma
// func Seeder() no mesmo arquivo. Os IDs escritos aqui são fixos: valem em
// todos os ambientes e são os únicos que um seeder posterior pode editar.
//
//	func Seeder() migrate.Rows {
//	    return migrate.Rows{
//	        {"id": 10, "nome": "Phelipe", "cidade": "Cuiaba"},
//	    }
//	}
type Rows []Row

// Definition é a lista ordenada de ações de uma migration. Todas compartilham
// o mesmo registro de histórico e são aplicadas na ordem declarada.
type Definition = []Operation

// Define agrupa uma ou mais ações em uma única migration.
func Define(operations ...Operation) Definition { return operations }

func TODO() Definition { return Definition{{Kind: string(acao.Todo)}} }

func Col(name string) Column { return acao.NovaColuna(name) }

// RegisteredView is used by AgendaGoKit's generated view catalog.
func RegisteredView(name string) View { return View{name: name} }

func CreateTable(name string, columns ...Column) Operation {
	return withColumns(acao.CreateTable, Table(name), columns)
}
func DropTable(table Table) Operation { return acao.Nova(acao.DropTable, string(table)) }
func AddColumn(table Table, columns ...Column) Operation {
	return withColumns(acao.AddColumn, table, columns)
}
func AlterColumn(table Table, columns ...Column) Operation {
	return withColumns(acao.AlterColumn, table, columns)
}
func DropColumn(table Table, columns ...Column) Operation {
	return withColumns(acao.DropColumn, table, columns)
}
func AddForeignKey(table Table, columns ...Column) Operation {
	return withColumns(acao.AddForeignKey, table, columns)
}
func AddCompositeForeignKey(table Table, name, referenceTable string, mappings ...string) Operation {
	foreignKey := &acao.ForeignKey{ConstraintName: name, ReferenceTable: referenceTable}
	for _, mapping := range mappings {
		parts := strings.SplitN(mapping, ":", 2)
		if len(parts) == 2 {
			foreignKey.Columns = append(foreignKey.Columns, parts[0])
			foreignKey.ReferenceColumns = append(foreignKey.ReferenceColumns, parts[1])
		}
	}
	return Operation{Kind: string(acao.AddForeignKey), Table: string(table), ForeignKey: foreignKey}
}
func DropForeignKey(table Table, columns ...Column) Operation {
	return withColumns(acao.DropForeignKey, table, columns)
}

func CreateIndex(table Table, name string, columns ...string) Operation {
	return Operation{Kind: string(acao.CreateIndex), Table: string(table), Name: name, IndexColumns: columns}
}
func CreateUniqueIndex(table Table, name string, columns ...string) Operation {
	op := CreateIndex(table, name, columns...)
	op.Unique = true
	return op
}
func DropIndex(table Table, name string) Operation {
	return Operation{Kind: string(acao.DropIndex), Table: string(table), Name: name}
}
func CreateView(view View) Operation {
	return Operation{Kind: string(acao.CreateView), Name: view.name}
}
func AlterView(view View) Operation {
	return Operation{Kind: string(acao.AlterView), Name: view.name}
}
func DropView(view View) Operation { return Operation{Kind: string(acao.DropView), Name: view.name} }
func CreateSequence(name string) Operation {
	return Operation{Kind: string(acao.CreateSequence), Name: name}
}
func DropSequence(name string) Operation {
	return Operation{Kind: string(acao.DropSequence), Name: name}
}
func RenameTable(table Table, newName string) Operation {
	return Operation{Kind: string(acao.RenameTable), Table: string(table), NewName: newName}
}
func RenameColumn(table Table, oldName, newName string) Operation {
	return Operation{Kind: string(acao.RenameColumn), Table: string(table), Column: &acao.ColunaDefinicao{Name: oldName}, NewName: newName}
}
func AddPrimaryKey(table Table, name string, columns ...string) Operation {
	return Operation{Kind: string(acao.AddPrimaryKey), Table: string(table), Name: name, IndexColumns: columns}
}
func AddUnique(table Table, name string, columns ...string) Operation {
	return Operation{Kind: string(acao.AddUnique), Table: string(table), Name: name, IndexColumns: columns}
}
func AddCheck(table Table, name, expression string) Operation {
	return Operation{Kind: string(acao.AddCheck), Table: string(table), Name: name, SQL: expression}
}
func DropConstraint(table Table, name string) Operation {
	return Operation{Kind: string(acao.DropConstraint), Table: string(table), Name: name}
}
func SQL(dialect, statement string) Operation {
	return Operation{Kind: string(acao.RawSQL), Dialect: dialect, SQL: statement}
}

func withColumns(kind acao.Tipo, table Table, columns []Column) Operation {
	items := make([]acao.Item, len(columns))
	for i := range columns {
		items[i] = columns[i]
	}
	return acao.Nova(kind, string(table), items...)
}
