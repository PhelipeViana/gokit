package migraterun

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/migration/acao"
)

type documentedTable struct {
	Name        string
	Columns     []acao.ColunaDefinicao
	Constraints []string
}

type migrationDocumentationMeta struct {
	Author      string
	Link        string
	Uncommitted bool
}

// GenerateDocumentation é o ponto único de geração dos documentos derivados
// das migrations. Pode ser chamado por migrate, reload e futuros processos.
func GenerateDocumentation(root string, state config.ConfigState) error {
	if state.Config == nil {
		return fmt.Errorf("gokit.json não carregado")
	}
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if err != nil {
		return err
	}
	docsDir := filepath.Join(root, filepath.FromSlash(state.Config.Output.Docs))
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return fmt.Errorf("criar diretório de documentação: %w", err)
	}

	generatedAt := time.Now()
	if err := writeGeneratedDocument(filepath.Join(docsDir, "database.md"), renderDatabaseDocumentation(files, generatedAt)); err != nil {
		return err
	}
	metadata := loadMigrationDocumentationMeta(root, docsDir, files)
	if err := writeGeneratedDocument(filepath.Join(docsDir, "migrations.md"), renderMigrationsDocumentation(files, metadata, generatedAt)); err != nil {
		return err
	}
	return nil
}

func writeGeneratedDocument(path, content string) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, []byte(content), 0o644); err != nil {
		return fmt.Errorf("gerar %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publicar %s: %w", filepath.Base(path), err)
	}
	return nil
}

func renderDatabaseDocumentation(files []migrationFile, generatedAt time.Time) string {
	tables := buildDocumentedSchema(files)
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)

	var out strings.Builder
	out.WriteString("# 🗄️ Documentação do Schema de Banco de Dados\n\n")
	out.WriteString("> [!NOTE]\n> Documento gerado automaticamente a partir das migrations Go. Não edite manualmente.\n>\n")
	out.WriteString("> 🕒 **Última atualização:** `" + generatedAt.Format("02/01/2006 às 15:04:05") + "`\n\n---\n")
	for _, name := range names {
		table := tables[name]
		out.WriteString(fmt.Sprintf("\n## 📋 Tabela: `%s`\n\n", table.Name))
		out.WriteString(fmt.Sprintf("> 📊 **Resumo:** %d colunas | %d restrições\n\n", len(table.Columns), len(table.Constraints)))
		out.WriteString("### 📌 Colunas\n\n| Nº | Campo | Descrição | Obrigatório | Tipo de dado | Chave |\n|---:|:---|:---|:---:|:---|:---:|\n")
		for index, column := range table.Columns {
			required := "✅ SIM"
			if column.Nullable {
				required = "❌ NÃO"
			}
			key := "—"
			switch {
			case column.PrimaryKey:
				key = "🔑 PK"
			case column.ReferenceTable != "":
				key = "🔗 FK"
			case column.Unique:
				key = "🔒 UK"
			}
			out.WriteString(fmt.Sprintf("| %d | `%s` | %s | %s | `%s` | %s |\n", index+1, column.Name, humanizeName(column.Name), required, documentedType(column), key))
		}
		if len(table.Constraints) > 0 {
			out.WriteString("\n### 🔒 Restrições\n\n")
			for _, constraint := range table.Constraints {
				out.WriteString("- " + constraint + "\n")
			}
		}
		out.WriteString("\n---\n")
	}
	return out.String()
}

func buildDocumentedSchema(files []migrationFile) map[string]*documentedTable {
	tables := map[string]*documentedTable{}
	aliases := map[string]string{}
	for _, file := range files {
		for _, original := range file.Plan.Operations {
			op := resolveAlias(original, aliases)
			name := strings.ToLower(op.Table)
			switch acao.Tipo(op.Kind) {
			case acao.CreateTable:
				table := &documentedTable{Name: op.Table, Columns: append([]acao.ColunaDefinicao(nil), op.Columns...)}
				table.Constraints = constraintsFromOperation(op)
				tables[name] = table
			case acao.DropTable:
				delete(tables, name)
			case acao.AddColumn:
				if table := tables[name]; table != nil && op.Column != nil {
					table.Columns = upsertDocumentedColumn(table.Columns, *op.Column)
					table.Constraints = append(table.Constraints, constraintsFromOperation(op)...)
				}
			case acao.AlterColumn:
				if table := tables[name]; table != nil && op.Column != nil {
					table.Columns = upsertDocumentedColumn(table.Columns, *op.Column)
				}
			case acao.DropColumn:
				if table := tables[name]; table != nil && op.Column != nil {
					table.Columns = removeDocumentedColumn(table.Columns, op.Column.Name)
				}
			case acao.RenameTable:
				if table := tables[name]; table != nil && op.NewName != "" {
					delete(tables, name)
					table.Name = op.NewName
					tables[strings.ToLower(op.NewName)] = table
				}
			case acao.RenameColumn:
				if table := tables[name]; table != nil && op.Column != nil {
					for index := range table.Columns {
						if strings.EqualFold(table.Columns[index].Name, op.Column.Name) {
							table.Columns[index].Name = op.NewName
						}
					}
				}
			case acao.AddForeignKey, acao.AddPrimaryKey, acao.AddUnique, acao.AddCheck:
				if table := tables[name]; table != nil {
					table.Constraints = append(table.Constraints, constraintsFromOperation(op)...)
				}
			case acao.DropConstraint:
				if table := tables[name]; table != nil {
					table.Constraints = removeDocumentedConstraint(table.Constraints, op.Name)
				}
			}
			advanceAliases(aliases, []acao.Operacao{original})
		}
	}
	return tables
}

func constraintsFromOperation(op acao.Operacao) []string {
	var result []string
	for _, column := range append(append([]acao.ColunaDefinicao(nil), op.Columns...), columnValue(op.Column)...) {
		if column.PrimaryKey {
			result = append(result, fmt.Sprintf("`PK_%s_%s` — PRIMARY KEY (`%s`)", op.Table, column.Name, column.Name))
		}
		if column.Unique {
			result = append(result, fmt.Sprintf("`UK_%s_%s` — UNIQUE (`%s`)", op.Table, column.Name, column.Name))
		}
		if column.ReferenceTable != "" {
			name := column.ConstraintName
			if name == "" {
				name = "FK_" + op.Table + "_" + column.Name
			}
			result = append(result, fmt.Sprintf("`%s` — FOREIGN KEY (`%s`) → `%s` (`%s`)", name, column.Name, column.ReferenceTable, column.ReferenceColumn))
		}
	}
	if op.ForeignKey != nil {
		name := op.ForeignKey.ConstraintName
		if name == "" {
			name = "FK_" + op.Table + "_" + op.ForeignKey.Column
		}
		result = append(result, fmt.Sprintf("`%s` — FOREIGN KEY (`%s`) → `%s` (`%s`)", name, op.ForeignKey.Column, op.ForeignKey.ReferenceTable, op.ForeignKey.ReferenceColumn))
	}
	if op.Kind == string(acao.AddPrimaryKey) {
		result = append(result, fmt.Sprintf("`%s` — PRIMARY KEY (`%s`)", op.Name, strings.Join(op.IndexColumns, "`, `")))
	}
	if op.Kind == string(acao.AddUnique) {
		result = append(result, fmt.Sprintf("`%s` — UNIQUE (`%s`)", op.Name, strings.Join(op.IndexColumns, "`, `")))
	}
	if op.Kind == string(acao.AddCheck) {
		result = append(result, fmt.Sprintf("`%s` — CHECK `%s`", op.Name, op.SQL))
	}
	return result
}

func columnValue(column *acao.ColunaDefinicao) []acao.ColunaDefinicao {
	if column == nil {
		return nil
	}
	return []acao.ColunaDefinicao{*column}
}

func upsertDocumentedColumn(columns []acao.ColunaDefinicao, column acao.ColunaDefinicao) []acao.ColunaDefinicao {
	for index := range columns {
		if strings.EqualFold(columns[index].Name, column.Name) {
			columns[index] = column
			return columns
		}
	}
	return append(columns, column)
}

func removeDocumentedColumn(columns []acao.ColunaDefinicao, name string) []acao.ColunaDefinicao {
	result := columns[:0]
	for _, column := range columns {
		if !strings.EqualFold(column.Name, name) {
			result = append(result, column)
		}
	}
	return result
}

func removeDocumentedConstraint(constraints []string, name string) []string {
	result := constraints[:0]
	for _, constraint := range constraints {
		if !strings.Contains(strings.ToLower(constraint), "`"+strings.ToLower(name)+"`") {
			result = append(result, constraint)
		}
	}
	return result
}

func documentedType(column acao.ColunaDefinicao) string {
	switch column.Type {
	case "string", "char":
		if column.Length > 0 {
			return fmt.Sprintf("%s(%d)", strings.ToUpper(column.Type), column.Length)
		}
	case "decimal":
		if column.Precision > 0 {
			return fmt.Sprintf("DECIMAL(%d,%d)", column.Precision, column.Scale)
		}
	}
	return strings.ToUpper(column.Type)
}

func humanizeName(value string) string {
	words := strings.Fields(strings.ReplaceAll(value, "_", " "))
	for index := range words {
		if index == 0 {
			words[index] = strings.Title(strings.ToLower(words[index]))
		}
	}
	return strings.Join(words, " ")
}

func loadMigrationDocumentationMeta(root, docsDir string, files []migrationFile) map[string]migrationDocumentationMeta {
	metadata := make(map[string]migrationDocumentationMeta, len(files))
	projectRoot := root
	if output, err := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Output(); err == nil {
		projectRoot = strings.TrimSpace(string(output))
	}
	fallbackAuthor := "Não identificado"
	if output, err := exec.Command("git", "config", "user.name").Output(); err == nil && strings.TrimSpace(string(output)) != "" {
		fallbackAuthor = strings.TrimSpace(string(output))
	}
	absDocs, _ := filepath.Abs(docsDir)
	for _, file := range files {
		absFile, _ := filepath.Abs(file.Path)
		link, err := filepath.Rel(absDocs, absFile)
		if err != nil {
			link = file.Path
		}
		link = filepath.ToSlash(link)

		author := ""
		gitPath, relErr := filepath.Rel(projectRoot, absFile)
		if relErr == nil && !strings.HasPrefix(gitPath, "..") {
			output, logErr := exec.Command("git", "-C", projectRoot, "log", "--follow", "--diff-filter=A", "-1", "--format=%an", "--", gitPath).Output()
			if logErr == nil {
				author = strings.TrimSpace(string(output))
			}
		}
		metadata[file.ID] = migrationDocumentationMeta{
			Author: fallbackIfEmpty(author, fallbackAuthor), Link: link, Uncommitted: author == "",
		}
	}
	return metadata
}

func fallbackIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func renderMigrationsDocumentation(files []migrationFile, metadata map[string]migrationDocumentationMeta, generatedAt time.Time) string {
	ordered := append([]migrationFile(nil), files...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID > ordered[j].ID })
	var out strings.Builder
	out.WriteString("# ⚙️ Histórico de Migrations\n\n")
	out.WriteString("> [!NOTE]\n> Documento gerado automaticamente. As migrations estão em ordem decrescente de criação.\n>\n")
	out.WriteString("> 🕒 **Última atualização:** `" + generatedAt.Format("02/01/2006 às 15:04:05") + "`\n\n")
	for _, file := range ordered {
		meta := metadata[file.ID]
		author := meta.Author
		if meta.Uncommitted {
			author += " · não commitada"
		}
		out.WriteString(fmt.Sprintf("- [`%s`](%s)\n", file.Name, meta.Link))
		out.WriteString(fmt.Sprintf("  - **Criada por:** %s\n  - **Criada em:** `%s`\n  - **ID:** `%s`\n  - **Checksum:** `%s`\n", author, file.Plan.CreatedAt.Format("02/01/2006 15:04:05"), file.ID, file.Checksum))
		out.WriteString("  - **Operações:**\n")
		for _, op := range file.Plan.Operations {
			target := op.Table
			if target == "" {
				target = op.Name
			}
			out.WriteString(fmt.Sprintf("    - `%s` em `%s`", op.Kind, target))
			if op.Column != nil {
				out.WriteString(fmt.Sprintf(" — coluna `%s`", op.Column.Name))
			}
			if len(op.Columns) > 0 {
				out.WriteString(fmt.Sprintf(" — %d coluna(s)", len(op.Columns)))
			}
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}
	return out.String()
}
