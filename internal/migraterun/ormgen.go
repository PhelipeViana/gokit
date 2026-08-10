package migraterun

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// GenerateORM rebuilds the application-owned mapping layer from migrations.
// Runtime behaviour never goes into this output: it remains in gokit/orm.
func GenerateORM(root string, state config.ConfigState) (int, error) {
	shapes, err := tableShapes(root, state)
	if err != nil {
		return 0, err
	}
	target := filepath.Join(root, filepath.FromSlash(state.Config.Output.ORM), "fields.gen.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	names := make([]string, 0, len(shapes))
	for name := range shapes {
		names = append(names, name)
	}
	sort.Strings(names)

	// Deriva as relações do grafo de FK: belongsTo (FK na própria tabela) e
	// hasMany (inverso — toda FK que aponta para a tabela vira um hasMany nela).
	type genRel struct {
		name       string // nome do campo/relação
		kind       string // "belongsTo" | "hasMany"
		target     string // tabela de destino
		fkColumn   string // belongsTo: FK na própria; hasMany: FK no filho
		fkNullable bool
		jsonName   string // chave json (snake) do campo de relação
	}
	relsByTable := map[string][]genRel{}
	for _, name := range names {
		shape := shapes[name]
		for _, col := range shape.Columns {
			if col.ReferenceTable == "" {
				continue
			}
			relsByTable[shape.Table] = append(relsByTable[shape.Table], genRel{
				name: relNameFromFK(col.Name), kind: "belongsTo",
				target: col.ReferenceTable, fkColumn: col.Name, fkNullable: col.Nullable,
				jsonName: strings.TrimSuffix(strings.ToLower(col.Name), "_id"),
			})
		}
	}
	for _, name := range names {
		child := shapes[name]
		for _, col := range child.Columns {
			if col.ReferenceTable == "" {
				continue
			}
			relsByTable[col.ReferenceTable] = append(relsByTable[col.ReferenceTable], genRel{
				name: exportedORMIdentifier(child.Table), kind: "hasMany",
				target: child.Table, fkColumn: col.Name, fkNullable: col.Nullable,
				jsonName: strings.ToLower(child.Table),
			})
		}
	}
	hasRelations := false
	for _, rs := range relsByTable {
		if len(rs) > 0 {
			hasRelations = true
			break
		}
	}

	var body strings.Builder
	var loaders strings.Builder
	var inits strings.Builder
	needsTime := false
	for _, name := range names {
		shape := shapes[name]
		entity := exportedORMIdentifier(shape.Table)       // ex.: Users
		unexported := unexportedORMIdentifier(shape.Table) // ex.: users

		// Linha tipada da entidade (retorno de Get/First). Coluna nula → ponteiro.
		fmt.Fprintf(&body, "type %sRow struct {\n", entity)
		for _, column := range shape.Columns {
			goType := rowGoType(column)
			if strings.Contains(goType, "time.Time") {
				needsTime = true
			}
			tag := "`json:\"" + column.Name + "\"`"
			fmt.Fprintf(&body, "\t%s %s %s\n", exportedORMIdentifier(column.Name), goType, tag)
		}
		// Campos de relação (preenchidos só quando pedidos via .With; omitempty
		// esconde os que não vieram na resposta JSON).
		for _, rel := range relsByTable[shape.Table] {
			targetRow := exportedORMIdentifier(rel.target) + "Row"
			tag := "`json:\"" + rel.jsonName + ",omitempty\"`"
			if rel.kind == "belongsTo" {
				fmt.Fprintf(&body, "\t%s *%s %s\n", rel.name, targetRow, tag)
			} else {
				fmt.Fprintf(&body, "\t%s []%s %s\n", rel.name, targetRow, tag)
			}
		}
		body.WriteString("}\n\n")

		// Conjunto de operadores por coluna, exposto em orm.Users.Field.<Coluna>.
		fmt.Fprintf(&body, "type %sFieldSet struct {\n", unexported)
		for _, column := range shape.Columns {
			fmt.Fprintf(&body, "\t%s %s\n", exportedORMIdentifier(column.Name), filterType(column.Type))
		}
		body.WriteString("}\n\n")

		// Conjunto de relações (belongsTo/hasMany), exposto em orm.Users.Relation.<Nome>.
		fmt.Fprintf(&body, "type %sRelations struct {\n", unexported)
		for _, rel := range relsByTable[shape.Table] {
			fmt.Fprintf(&body, "\t%s gokitorm.Relation\n", rel.name)
		}
		body.WriteString("}\n\n")

		// Handle único: embute o Model[Row] (promove Where/Select/OrderBy/Get/...
		// direto na entidade — orm.Users.Where(...)), + .Field (operadores) e
		// .Relation (relações).
		fmt.Fprintf(&body, "type %sEntity struct {\n\tgokitorm.Model[%sRow]\n\tField    %sFieldSet\n\tRelation %sRelations\n}\n\n", unexported, entity, unexported, unexported)

		// Scanner por NOME de coluna (robusto à ordem e à caixa que cada banco devolve).
		fmt.Fprintf(&body, "func scan%s(rows *sql.Rows) ([]%sRow, error) {\n", entity, entity)
		body.WriteString("\tcols, err := rows.Columns()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
		fmt.Fprintf(&body, "\tvar out []%sRow\n", entity)
		body.WriteString("\tfor rows.Next() {\n")
		fmt.Fprintf(&body, "\t\tvar row %sRow\n", entity)
		body.WriteString("\t\tdest := make([]any, len(cols))\n\t\tfor i, c := range cols {\n\t\t\tswitch strings.ToLower(c) {\n")
		for _, column := range shape.Columns {
			fmt.Fprintf(&body, "\t\t\tcase %q:\n\t\t\t\tdest[i] = &row.%s\n", strings.ToLower(column.Name), exportedORMIdentifier(column.Name))
		}
		body.WriteString("\t\t\tdefault:\n\t\t\t\tvar descarte any\n\t\t\t\tdest[i] = &descarte\n\t\t\t}\n\t\t}\n")
		body.WriteString("\t\tif err := rows.Scan(dest...); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tout = append(out, row)\n\t}\n\treturn out, rows.Err()\n}\n\n")

		// Construção num func literal para não repetir os literais de Field.
		fmt.Fprintf(&body, "var %s = func() %sEntity {\n", entity, unexported)
		for _, column := range shape.Columns {
			fmt.Fprintf(&body, "\t%s := gokitorm.Field{Name: %q, Entity: %q, Table: %q, Column: %q, DataType: %q, Nullable: %t, PrimaryKey: %t, AutoIncrement: %t, Unique: %t, Reference: %q}\n", localFieldVar(column.Name), exportedORMIdentifier(column.Name), shape.Table, shape.Table, column.Name, column.Type, column.Nullable, column.PrimaryKey, column.AutoIncrement, column.Unique, reference(column))
		}
		fmt.Fprintf(&body, "\treturn %sEntity{\n", unexported)
		fmt.Fprintf(&body, "\t\tModel: gokitorm.NewModel[%sRow](gokitorm.EntityFields{Name: %q, Fields: []gokitorm.Field{", entity, shape.Table)
		for i, column := range shape.Columns {
			if i > 0 {
				body.WriteString(", ")
			}
			body.WriteString(localFieldVar(column.Name))
		}
		fmt.Fprintf(&body, "}}, scan%s),\n", entity)
		fmt.Fprintf(&body, "\t\tField: %sFieldSet{\n", unexported)
		for _, column := range shape.Columns {
			fmt.Fprintf(&body, "\t\t\t%s: %s(%s),\n", exportedORMIdentifier(column.Name), filterConstructor(column.Type), localFieldVar(column.Name))
		}
		body.WriteString("\t\t},\n\t}\n}()\n\n")

		// O wiring das relações vai num init() (não no literal do var) para evitar
		// ciclo de inicialização entre vars de entidade (ex.: Cidades <-> Estados)
		// através dos loaders. Os loaders em si são funções, emitidas ao final.
		for _, rel := range relsByTable[shape.Table] {
			// correlação para EXISTS: belongsTo → FK no pai / id no filho; hasMany → id no pai / FK no filho.
			parentCol, childCol := "id", rel.fkColumn
			if rel.kind == "belongsTo" {
				parentCol, childCol = rel.fkColumn, "id"
			}
			fmt.Fprintf(&inits, "\t%s.Relation.%s = gokitorm.NewRelation(%q, %q, %q, %q, %q, load%s%s)\n",
				entity, rel.name, rel.name, rel.kind, rel.target, parentCol, childCol, entity, rel.name)
			emitLoader(&loaders, entity, rel.name, rel.kind, rel.target, rel.fkColumn, rel.fkNullable)
		}
	}
	if hasRelations {
		body.WriteString("func init() {\n")
		body.WriteString(inits.String())
		body.WriteString("}\n\n")
	}
	body.WriteString(loaders.String())

	var out strings.Builder
	out.WriteString("// Code generated by GoKit from migrations. DO NOT EDIT.\npackage orm\n\n")
	if len(names) > 0 {
		out.WriteString("import (\n\t\"database/sql\"\n")
		if hasRelations {
			out.WriteString("\t\"context\"\n\t\"fmt\"\n")
		}
		out.WriteString("\t\"strings\"\n")
		if needsTime {
			out.WriteString("\t\"time\"\n")
		}
		out.WriteString("\n\tgokitorm \"github.com/PhelipeViana/gokit/orm\"\n)\n\n")
	}
	out.WriteString(body.String())
	formatted, err := format.Source([]byte(out.String()))
	if err != nil {
		return 0, err
	}
	_ = os.Remove(target)
	if err := os.WriteFile(target, formatted, 0o644); err != nil {
		return 0, err
	}

	// response.gen.go — envelope padrão de resposta JSON (data/meta/error).
	if err := writeResponseEnvelope(filepath.Dir(target)); err != nil {
		return len(names), err
	}
	return len(names), nil
}

// writeResponseEnvelope gera o response.gen.go com o formato único de resposta
// da API (Response{data,meta,error} + helpers Ok/Paginated/Fail).
func writeResponseEnvelope(dir string) error {
	src := "// Code generated by GoKit. DO NOT EDIT.\n" +
		"package orm\n\n" +
		"import gokitorm \"github.com/PhelipeViana/gokit/orm\"\n\n" +
		"// Response é o envelope padrão de resposta JSON da API.\n" +
		"type Response struct {\n" +
		"\tData  any   `json:\"data,omitempty\"`\n" +
		"\tMeta  *Meta `json:\"meta,omitempty\"`\n" +
		"\tError *Erro `json:\"error,omitempty\"`\n" +
		"}\n\n" +
		"// Meta traz os metadados de paginação.\n" +
		"type Meta struct {\n" +
		"\tTotal int64 `json:\"total\"`\n" +
		"\tPage  int   `json:\"page\"`\n" +
		"\tSize  int   `json:\"size\"`\n" +
		"\tPages int   `json:\"pages\"`\n" +
		"}\n\n" +
		"type Erro struct {\n" +
		"\tMessage string `json:\"message\"`\n" +
		"}\n\n" +
		"// Ok embrulha um payload de sucesso.\n" +
		"func Ok(data any) Response { return Response{Data: data} }\n\n" +
		"// Paginated embrulha uma página (itens + metadados).\n" +
		"func Paginated[T any](p gokitorm.Page[T]) Response {\n" +
		"\treturn Response{Data: p.Items, Meta: &Meta{Total: p.Total, Page: p.Page, Size: p.Size, Pages: p.Pages}}\n" +
		"}\n\n" +
		"// Fail embrulha um erro.\n" +
		"func Fail(err error) Response { return Response{Error: &Erro{Message: err.Error()}} }\n"
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "response.gen.go")
	_ = os.Remove(target)
	return os.WriteFile(target, formatted, 0o644)
}

func reference(column acao.ColunaDefinicao) string {
	if column.ReferenceTable == "" {
		return ""
	}
	return column.ReferenceTable + "." + column.ReferenceColumn
}

// rowGoType mapeia o tipo SQL da coluna para o tipo Go da linha tipada.
// Coluna anulável vira ponteiro para representar NULL.
func rowGoType(column acao.ColunaDefinicao) string {
	base := "string"
	switch strings.ToLower(column.Type) {
	case "integer", "int":
		base = "int64"
	case "decimal":
		base = "float64"
	case "boolean":
		base = "bool"
	case "date", "datetime", "timestamp":
		base = "time.Time"
	}
	if column.Nullable {
		return "*" + base
	}
	return base
}

func filterType(kind string) string {
	switch strings.ToLower(kind) {
	case "integer", "int", "decimal":
		return "gokitorm.NumberFilterField"
	case "boolean":
		return "gokitorm.BoolFilterField"
	case "date", "datetime", "timestamp":
		return "gokitorm.DateFilterField"
	default:
		return "gokitorm.StringFilterField"
	}
}
func filterConstructor(kind string) string {
	switch filterType(kind) {
	case "gokitorm.NumberFilterField":
		return "gokitorm.NumberFilter"
	case "gokitorm.BoolFilterField":
		return "gokitorm.BoolFilter"
	case "gokitorm.DateFilterField":
		return "gokitorm.DateFilter"
	default:
		return "gokitorm.StringFilter"
	}
}
func exportedORMIdentifier(value string) string {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == '_' || r == '-' || r == ' ' })
	for i := range parts {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// relNameFromFK deriva o nome da relação belongsTo a partir da coluna FK:
// cidade_id -> Cidade, pais_id -> Pais. Sem sufixo _id, usa a coluna inteira.
func relNameFromFK(col string) string {
	base := col
	if strings.HasSuffix(strings.ToLower(col), "_id") {
		base = col[:len(col)-3]
	}
	return exportedORMIdentifier(base)
}

// emitLoader escreve o loader tipado de uma relação (belongsTo ou hasMany) que
// o descritor gokitorm.Relation carrega. Faz o type-assert dos pais e delega
// para os helpers genéricos do motor.
func emitLoader(b *strings.Builder, selfEntity, relName, kind, target, fkColumn string, fkNullable bool) {
	selfRow := selfEntity + "Row"
	targetEntity := exportedORMIdentifier(target)
	targetRow := targetEntity + "Row"
	loaderName := "load" + selfEntity + relName

	fmt.Fprintf(b, "func %s(ctx context.Context, r gokitorm.Runner, parentsAny any, rel gokitorm.Relation) error {\n", loaderName)
	fmt.Fprintf(b, "\tparents, ok := parentsAny.([]%s)\n", selfRow)
	fmt.Fprintf(b, "\tif !ok {\n\t\treturn fmt.Errorf(\"relação %s: pais não são []%s\")\n\t}\n", relName, selfRow)

	if kind == "belongsTo" {
		fkField := exportedORMIdentifier(fkColumn)
		b.WriteString("\treturn gokitorm.BelongsTo(ctx, r, parents,\n")
		if fkNullable {
			fmt.Fprintf(b, "\t\tfunc(p %s) (int64, bool) {\n\t\t\tif p.%s == nil {\n\t\t\t\treturn 0, false\n\t\t\t}\n\t\t\treturn *p.%s, true\n\t\t},\n", selfRow, fkField, fkField)
		} else {
			fmt.Fprintf(b, "\t\tfunc(p %s) (int64, bool) { return p.%s, true },\n", selfRow, fkField)
		}
		fmt.Fprintf(b, "\t\t%s.Model, %s.Field.Id,\n", targetEntity, targetEntity)
		fmt.Fprintf(b, "\t\tfunc(c %s) int64 { return c.Id },\n", targetRow)
		fmt.Fprintf(b, "\t\tfunc(p *%s, c *%s) { p.%s = c },\n", selfRow, targetRow, relName)
		b.WriteString("\t\trel)\n}\n\n")
		return
	}

	childFk := exportedORMIdentifier(fkColumn)
	b.WriteString("\treturn gokitorm.HasMany(ctx, r, parents,\n")
	fmt.Fprintf(b, "\t\tfunc(p %s) int64 { return p.Id },\n", selfRow)
	fmt.Fprintf(b, "\t\t%s.Model, %s.Field.%s,\n", targetEntity, targetEntity, childFk)
	if fkNullable {
		fmt.Fprintf(b, "\t\tfunc(c %s) (int64, bool) {\n\t\t\tif c.%s == nil {\n\t\t\t\treturn 0, false\n\t\t\t}\n\t\t\treturn *c.%s, true\n\t\t},\n", targetRow, childFk, childFk)
	} else {
		fmt.Fprintf(b, "\t\tfunc(c %s) (int64, bool) { return c.%s, true },\n", targetRow, childFk)
	}
	fmt.Fprintf(b, "\t\tfunc(p *%s, cs []%s) { p.%s = cs },\n", selfRow, targetRow, relName)
	b.WriteString("\t\trel)\n}\n\n")
}

// unexportedORMIdentifier devolve o identificador em camelCase não-exportado,
// usado só como prefixo de tipos gerados (usersEntity, usersFieldSet).
func unexportedORMIdentifier(value string) string {
	e := exportedORMIdentifier(value)
	if e == "" {
		return ""
	}
	return strings.ToLower(e[:1]) + e[1:]
}

// localFieldVar nomeia a variável local do Field dentro do func literal gerado.
// O prefixo "col" garante identificador válido mesmo para colunas com nome de
// palavra reservada (type, func, range...).
func localFieldVar(column string) string {
	return "col" + exportedORMIdentifier(column)
}
