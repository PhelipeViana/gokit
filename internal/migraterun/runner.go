package migraterun

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gokit/internal/cliui"
	"gokit/internal/config"
	"gokit/internal/migrationgo"
	"gokit/migration/acao"
)

type Plan struct {
	Version    int             `json:"version"`
	Migration  string          `json:"migration"`
	CreatedAt  time.Time       `json:"created_at"`
	Operations []acao.Operacao `json:"operations"`
}

type migrationFile struct {
	Name, ID, Path, Checksum string
	LegacyChecksums          []string
	Plan                     Plan
}

// LoadIssue é um problema encontrado em um arquivo de migration durante a
// leitura ou a pré-validação.
type LoadIssue struct {
	File   string
	Detail string
}

// LoadError agrupa todos os problemas do corpus. Antes o carregamento abortava
// no primeiro erro, o que escondia os demais e obrigava a corrigir um por vez.
type LoadError struct {
	Issues []LoadIssue
}

func (e *LoadError) Error() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%d migration(s) com problema:", len(e.Issues))
	for _, issue := range e.Issues {
		fmt.Fprintf(&builder, "\n  %s\n    %s", issue.File, issue.Detail)
	}
	return builder.String()
}

// Summary agrupa os problemas por mensagem, para que 100 arquivos com a mesma
// causa apareçam como uma linha em vez de cem.
func (e *LoadError) Summary() string {
	counts := map[string]int{}
	order := []string{}
	for _, issue := range e.Issues {
		key := generalizeDetail(issue.Detail)
		if _, seen := counts[key]; !seen {
			order = append(order, key)
		}
		counts[key]++
	}
	sort.Slice(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	var builder strings.Builder
	for _, key := range order {
		fmt.Fprintf(&builder, "\n  %4dx  %s", counts[key], key)
	}
	return builder.String()
}

var detailNoise = regexp.MustCompile(`"[^"]*"`)
var detailPosition = regexp.MustCompile(`^.*?\.go:\d+: `)

func generalizeDetail(detail string) string {
	detail = detailPosition.ReplaceAllString(detail, "")
	return detailNoise.ReplaceAllString(detail, `"…"`)
}

func (e *LoadError) add(file, format string, arguments ...any) {
	e.Issues = append(e.Issues, LoadIssue{File: file, Detail: fmt.Sprintf(format, arguments...)})
}

func (e *LoadError) orNil() error {
	if len(e.Issues) == 0 {
		return nil
	}
	return e
}

func migrationfsFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

var goMigrationName = regexp.MustCompile(`^(\d{4}_\d{2}_\d{2}_\d{6})(?:.*)?\.go$`)

// Validate parses and pre-validates every migration without opening a database.
func Validate(root string, state config.ConfigState) (int, error) {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

// ValidateReport imprime o resultado da pré-validação de todo o corpus sem
// abrir conexão com o banco.
func ValidateReport(root string, state config.ConfigState) error {
	cliui.PrintTitle("GoKit · Validate")
	folder := filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate))
	files, err := loadPlans(folder)

	var loadErr *LoadError
	if errors.As(err, &loadErr) {
		fmt.Printf("%s %d problema(s) encontrado(s):\n", cliui.Failure("✗"), len(loadErr.Issues))
		fmt.Println(cliui.Muted("\nPor causa:") + loadErr.Summary())
		fmt.Println(cliui.Muted("\nPor arquivo:"))
		for _, issue := range loadErr.Issues {
			fmt.Printf("  %s %s\n      %s\n", cliui.Failure("•"), issue.File, issue.Detail)
		}
		return cliui.NewUserError(
			fmt.Sprintf("%d migration(s) não passaram na pré-validação.", len(loadErr.Issues)),
			"Corrija os arquivos listados acima e rode gokit migrate validate novamente.",
		)
	}
	if err != nil {
		return err
	}

	operations := 0
	kinds := map[string]int{}
	for _, file := range files {
		operations += len(file.Plan.Operations)
		for _, operation := range file.Plan.Operations {
			kinds[operation.Kind]++
		}
	}
	names := make([]string, 0, len(kinds))
	for kind := range kinds {
		names = append(names, kind)
	}
	sort.Strings(names)

	fmt.Printf("%s %d migration(s), %d operação(ões)\n", cliui.Success("✓ OK"), len(files), operations)
	for _, kind := range names {
		fmt.Printf("  %s %-20s %d\n", cliui.Muted("·"), kind, kinds[kind])
	}
	return nil
}

func Run(root string, state config.ConfigState) error {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if err != nil {
		return describeLoadError(err)
	}
	if len(files) == 0 {
		return cliui.NewUserError(
			"Nenhuma migration foi gerada.",
			fmt.Sprintf("Crie um arquivo de migration em Go sob %s para começar.", state.Config.Output.Migrate),
		)
	}

	cliui.PrintTitle("GoKit · Migrations")
	connection := state.Config.Connections[state.ActiveClient]
	dialect := state.ActiveDialect
	historyTable := state.Config.Migrate.Table
	if historyTable == "" {
		historyTable = "migrations_gokit"
	}

	fmt.Printf("%s %s (%s) %s\n", cliui.Info("→"), state.ActiveClient, dialect, cliui.Info("[ATIVO]"))
	fmt.Printf("  %s %s\n", cliui.Muted("Histórico:"), historyLocation(connection, historyTable))
	applied, skipped, runErr := runConnection(connection, historyTable, files)
	if runErr != nil {
		fmt.Printf("  %s %s\n", cliui.Failure("✗ ERRO:"), runErr)
		message, solution := migrationConnectionAdvice(connection, runErr)
		fmt.Printf("  %s %s\n", cliui.Warning("⚠ Diagnóstico:"), message)
		return cliui.NewUserError(
			"A conexão ativa não concluiu as migrations.",
			solution,
		)
	}
	fmt.Printf("  %s %d aplicada(s), %d já executada(s)\n", cliui.Success("✓ OK"), applied, skipped)
	fmt.Println("\n" + cliui.Success("✓ Migrations atualizadas na conexão padrão."))
	return nil
}

// describeLoadError transforma a lista crua de problemas em um resumo por
// causa, mantendo a saída legível mesmo com centenas de arquivos quebrados.
func describeLoadError(err error) error {
	var loadErr *LoadError
	if !errors.As(err, &loadErr) {
		return err
	}
	fmt.Printf("%s %d migration(s) não passaram na pré-validação:\n%s\n",
		cliui.Failure("✗"), len(loadErr.Issues), loadErr.Summary())
	return cliui.NewUserError(
		"O corpus de migrations tem erros e nada foi aplicado.",
		"Rode gokit migrate validate para ver arquivo por arquivo.",
	)
}

func Rollback(root string, state config.ConfigState) error {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if err != nil {
		return describeLoadError(err)
	}
	if len(files) == 0 {
		return cliui.NewUserError("Nenhuma migration encontrada.", "Nenhuma migration disponível.")
	}

	cliui.PrintTitle("GoKit · Rollback")
	connection := state.Config.Connections[state.ActiveClient]
	dialect := state.ActiveDialect
	activeURL := state.ActiveURL
	historyTable := state.Config.Migrate.Table
	if historyTable == "" {
		historyTable = "migrations_gokit"
	}
	schema := connection.Schema

	driver := map[string]string{
		"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver",
	}[dialect]
	if driver == "" {
		return fmt.Errorf("dialeto não suportado: %s", dialect)
	}
	db, err := sql.Open(driver, activeURL)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return err
	}

	if err := ensureHistory(ctx, db, dialect, schema, historyTable); err != nil {
		return err
	}

	var maxBatch int
	err = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COALESCE(MAX(batch), 0) FROM %s", qualified(dialect, schema, historyTable))).Scan(&maxBatch)
	if err != nil {
		return fmt.Errorf("erro ao buscar último batch: %w", err)
	}

	if maxBatch == 0 {
		fmt.Println("Nenhuma migration para reverter.")
		return nil
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT migration FROM %s WHERE batch = %d ORDER BY id DESC", qualified(dialect, schema, historyTable), maxBatch))
	if err != nil {
		return fmt.Errorf("erro ao buscar histórico do lote %d: %w", maxBatch, err)
	}
	defer rows.Close()

	var migrationsToRollback []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		migrationsToRollback = append(migrationsToRollback, name)
	}

	if len(migrationsToRollback) == 0 {
		fmt.Println("Nenhuma migration encontrada para reverter no lote", maxBatch)
		return nil
	}

	filesMap := make(map[string]migrationFile)
	for _, file := range files {
		filesMap[file.ID] = file
		filesMap[file.Name] = file
	}

	fmt.Printf("Revertendo lote %d (%d migrations)...\n", maxBatch, len(migrationsToRollback))

	// Os aliases precisam estar resolvidos aqui também: o plano guarda o
	// apelido, e o banco só conhece o nome físico.
	aliases := map[string]string{}
	for _, file := range files {
		advanceAliases(aliases, file.Plan.Operations)
	}

	for _, name := range migrationsToRollback {
		file, exists := filesMap[name]
		if !exists {
			return fmt.Errorf("arquivo da migration %s não encontrado no projeto. Impossível reverter sem a definição Go", name)
		}

		fmt.Printf("  %s %s\n", cliui.Warning("←"), file.Name)

		for i := len(file.Plan.Operations) - 1; i >= 0; i-- {
			operation := resolveAlias(file.Plan.Operations[i], aliases)
			rollbackQuery, err := rollbackSQL(dialect, schema, operation)
			if err != nil {
				return fmt.Errorf("%s: %w", file.Name, err)
			}
			if rollbackQuery == "" {
				continue
			}
			if _, err := db.ExecContext(ctx, rollbackQuery); err != nil {
				return fmt.Errorf("falha ao reverter operação %s na tabela %s: %w", operation.Kind, operation.Table, err)
			}
		}

		deleteQuery := map[string]string{
			"postgres":  fmt.Sprintf("DELETE FROM %s WHERE migration = $1", qualified(dialect, schema, historyTable)),
			"oracle":    fmt.Sprintf("DELETE FROM %s WHERE migration = :1", qualified(dialect, schema, historyTable)),
			"mysql":     fmt.Sprintf("DELETE FROM %s WHERE migration = ?", qualified(dialect, schema, historyTable)),
			"sqlserver": fmt.Sprintf("DELETE FROM %s WHERE migration = @p1", qualified(dialect, schema, historyTable)),
		}[dialect]

		_, err = db.ExecContext(ctx, deleteQuery, name)
		if err != nil {
			return fmt.Errorf("erro ao remover histórico da migration %s: %w", name, err)
		}
	}

	fmt.Println("\n" + cliui.Success("✓ Rollback concluído com sucesso."))
	return nil
}

// rollbackSQL devolve o comando que desfaz a operação, "" quando não há nada a
// desfazer, ou erro quando a operação não é reversível automaticamente.
//
// Antes, tudo que não fosse um dos quatro tipos conhecidos caía num `continue`
// silencioso: o histórico era apagado sem que nada fosse revertido, deixando o
// banco à frente do que o histórico dizia.
func rollbackSQL(dialect, schema string, operation acao.Operacao) (string, error) {
	switch acao.Tipo(operation.Kind) {
	case acao.CreateTable:
		return dropTableSQL(dialect, schema, operation.Table), nil
	case acao.AddColumn:
		if operation.Column == nil {
			return "", fmt.Errorf("add_column sem coluna")
		}
		return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s",
			qualified(dialect, schema, operation.Table), quote(dialect, operation.Column.Name)), nil
	case acao.CreateIndex:
		if dialect == "mysql" {
			return fmt.Sprintf("DROP INDEX %s ON %s",
				quote(dialect, operation.Name), qualified(dialect, schema, operation.Table)), nil
		}
		return "DROP INDEX " + quote(dialect, operation.Name), nil
	case acao.AddForeignKey:
		if operation.ForeignKey == nil {
			return "", fmt.Errorf("add_foreign_key sem definição")
		}
		// O nome precisa ser recalculado igual ao da criação: quando a FK não
		// declara ConstraintName, o executor gera um a partir de tabela+coluna.
		name := operation.ForeignKey.ConstraintName
		if name == "" {
			name = foreignKeyName(operation.Table, operation.ForeignKey.Column)
		}
		return dropForeignKeySQL(dialect, schema, operation.Table, name), nil
	case acao.AddPrimaryKey, acao.AddUnique, acao.AddCheck:
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s",
			qualified(dialect, schema, operation.Table), quote(dialect, operation.Name)), nil
	case acao.CreateSequence:
		return "DROP SEQUENCE " + qualified(dialect, schema, operation.Name), nil
	case acao.CreateView:
		return "DROP VIEW " + qualified(dialect, schema, operation.Name), nil
	case acao.RenameTable:
		if dialect == "sqlserver" {
			return fmt.Sprintf("EXEC sp_rename N'%s', N'%s'", objectName(schema, operation.NewName), operation.Table), nil
		}
		return fmt.Sprintf("ALTER TABLE %s RENAME TO %s",
			qualified(dialect, schema, operation.NewName), quote(dialect, operation.Table)), nil
	case acao.RenameColumn:
		if operation.Column == nil {
			return "", fmt.Errorf("rename_column sem coluna")
		}
		if dialect == "sqlserver" {
			return fmt.Sprintf("EXEC sp_rename N'%s.%s', N'%s', N'COLUMN'",
				objectName(schema, operation.Table), operation.NewName, operation.Column.Name), nil
		}
		return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s",
			qualified(dialect, schema, operation.Table), quote(dialect, operation.NewName), quote(dialect, operation.Column.Name)), nil
	case acao.SeedRows:
		// O seed fixo só existe junto com o CreateTable do mesmo arquivo, e a
		// reversão desse CreateTable derruba a tabela inteira — inclusive as
		// linhas. Não há nada separado a desfazer aqui.
		return "", nil
	case acao.RawSQL, acao.AlterView:
		// Sem informação do estado anterior não há como desfazer com segurança.
		return "", fmt.Errorf("a operação %s não é reversível automaticamente; reverta manualmente ou crie uma migration de correção", operation.Kind)
	case acao.DropTable, acao.DropColumn, acao.DropIndex, acao.DropView,
		acao.DropSequence, acao.DropConstraint, acao.DropForeignKey:
		return "", fmt.Errorf("a operação %s removeu um objeto e não pode ser recriada automaticamente; reverta manualmente", operation.Kind)
	case acao.AlterColumn:
		return "", fmt.Errorf("alter_column não guarda a definição anterior da coluna; reverta manualmente")
	default:
		return "", fmt.Errorf("operação %s não tem rollback definido", operation.Kind)
	}
}

func pingConnection(connection config.ConnConfig) error {
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[strings.ToLower(connection.Dialect)]
	if driver == "" {
		return fmt.Errorf("dialeto não suportado: %s", connection.Dialect)
	}
	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func resetConnection(connection config.ConnConfig, historyTable string, files []migrationFile) error {
	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	if dialect == "mysql" {
		if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
			return err
		}
		defer db.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS=1")
	}
	for _, view := range managedViews(files) {
		exists, err := viewExists(ctx, db, dialect, connection.Schema, view)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := db.ExecContext(ctx, "DROP VIEW "+qualified(dialect, connection.Schema, view)); err != nil {
			return fmt.Errorf("remover view %s: %w", view, err)
		}
		fmt.Printf("  - view %s removida\n", view)
	}
	tables := managedTables(files)
	for _, table := range tables {
		exists, err := tableExists(ctx, db, dialect, connection.Schema, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := db.ExecContext(ctx, dropTableSQL(dialect, connection.Schema, table)); err != nil {
			return fmt.Errorf("remover tabela %s: %w", table, err)
		}
		fmt.Printf("  - tabela %s removida\n", table)
	}
	exists, err := tableExists(ctx, db, dialect, connection.Schema, historyTable)
	if err != nil {
		return err
	}
	if exists {
		if _, err := db.ExecContext(ctx, dropTableSQL(dialect, connection.Schema, historyTable)); err != nil {
			return fmt.Errorf("remover histórico %s: %w", historyTable, err)
		}
	}
	return nil
}

func managedViews(files []migrationFile) []string {
	set := map[string]bool{}
	for _, file := range files {
		for _, operation := range file.Plan.Operations {
			switch operation.Kind {
			case string(acao.CreateView), string(acao.AlterView):
				set[operation.Name] = true
			case string(acao.DropView):
				set[operation.Name] = false
			}
		}
	}
	result := make([]string, 0, len(set))
	for name, active := range set {
		if active {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

func managedTables(files []migrationFile) []string {
	all, children := map[string]bool{}, map[string][]string{}
	for _, file := range files {
		for _, operation := range file.Plan.Operations {
			if operation.Kind == "create_table" {
				all[operation.Table] = true
			}
			if operation.Kind == "add_foreign_key" && operation.ForeignKey != nil {
				parent := operation.ForeignKey.ReferenceTable
				children[parent] = append(children[parent], operation.Table)
			}
		}
	}
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)
	seen, result := map[string]bool{}, []string{}
	var visit func(string)
	visit = func(table string) {
		if seen[table] {
			return
		}
		seen[table] = true
		for _, child := range children[table] {
			visit(child)
		}
		result = append(result, table)
	}
	for _, name := range names {
		visit(name)
	}
	return result
}

func migrationConnectionAdvice(connection config.ConnConfig, err error) (string, string) {
	detail := strings.ToLower(err.Error())
	host := connectionHost(connection)
	local := isLocalHost(host)

	switch {
	case strings.Contains(detail, "ora-12564"),
		strings.Contains(detail, "connection refused"),
		strings.Contains(detail, "actively refused"),
		strings.Contains(detail, "no connection could be made"):
		if local {
			return fmt.Sprintf(
					"%s é uma conexão local; este erro não depende da internet e indica que a porta, o container ou o listener não aceitou a sessão",
					host,
				),
				"Confirme o container, a porta publicada e o listener Oracle; depois execute Connection e Migrate Run novamente."
		}
		return fmt.Sprintf(
				"%s é um host remoto; o servidor recusou a conexão e pode haver indisponibilidade, firewall, VPN ou falha de internet",
				host,
			),
			"Confira sua internet/VPN, DNS, firewall, host e porta; depois execute Connection e Migrate Run novamente."
	case strings.Contains(detail, "no such host"),
		strings.Contains(detail, "server misbehaving"),
		strings.Contains(detail, "name resolution"):
		return fmt.Sprintf(
				"não foi possível resolver o host %s; a causa pode ser DNS, VPN ou conexão com a internet",
				host,
			),
			"Confira sua internet/VPN e o nome do host no connection.yaml; depois execute Connection novamente."
	case strings.Contains(detail, "timeout"),
		strings.Contains(detail, "deadline exceeded"):
		if local {
			return fmt.Sprintf(
					"%s é local; o serviço não respondeu no tempo esperado e a internet não é necessária",
					host,
				),
				"Confira a saúde do container, a porta e os logs do banco; depois execute Connection novamente."
		}
		return fmt.Sprintf(
				"%s não respondeu; verifique internet, VPN, rota, firewall e disponibilidade do servidor",
				host,
			),
			"Teste sua internet/VPN e a conectividade com o host e a porta antes de repetir Migrate Run."
	case strings.Contains(detail, "ora-12514"):
		return "o listener respondeu, mas o serviço Oracle configurado não foi encontrado",
			"Confira ORACLE_SERVICE no .env e os serviços registrados no listener; depois execute Connection novamente."
	default:
		return "a conexão foi alcançada, mas o banco recusou ou interrompeu a operação",
			"Confira a mensagem técnica acima, execute Connection e corrija a configuração antes de repetir Migrate Run."
	}
}

func connectionHost(connection config.ConnConfig) string {
	if strings.EqualFold(connection.Dialect, "mysql") {
		if start := strings.Index(connection.BuildURL(), "@tcp("); start >= 0 {
			remainder := connection.BuildURL()[start+5:]
			if end := strings.Index(remainder, ")"); end >= 0 {
				hostPort := remainder[:end]
				if parsed, err := url.Parse("tcp://" + hostPort); err == nil {
					return parsed.Hostname()
				}
			}
		}
	}
	if parsed, err := url.Parse(connection.BuildURL()); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return "host configurado"
}

func isLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func loadPlans(folder string) ([]migrationFile, error) {
	paths, err := migrationfsFiles(folder)
	if err != nil {
		return nil, err
	}
	projectRoot := findProjectRoot(folder)
	if projectRoot != "" {
		if err := migrationgo.PrimeCoreCatalog(projectRoot, paths); err != nil {
			return nil, fmt.Errorf("reconstruir catálogo inicial de aliases: %w", err)
		}
	}
	failures := &LoadError{}
	var result []migrationFile
	for _, path := range paths {
		name := filepath.Base(path)
		display := displayMigrationPath(folder, path)
		goMatch := goMigrationName.FindStringSubmatch(name)
		if !strings.HasSuffix(name, "_migration.json") &&
			!strings.HasSuffix(name, "_schema.json") &&
			len(goMatch) == 0 {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			failures.add(display, "%v", err)
			continue
		}
		var plan Plan
		checksumData := data
		if strings.HasSuffix(name, ".go") {
			operations, err := migrationgo.ParseFile(path)
			if err != nil {
				failures.add(display, "%v", err)
				continue
			}
			plan = Plan{Version: 1, Migration: name, Operations: operations}
			checksumData, err = json.Marshal(operations)
			if err != nil {
				failures.add(display, "%v", err)
				continue
			}
		} else if err := json.Unmarshal(data, &plan); err != nil {
			failures.add(display, "%v", err)
			continue
		}
		sum := sha256.Sum256(checksumData)
		legacyChecksums := legacyViewChecksums(plan.Operations)
		id := name
		if len(goMatch) > 0 {
			id = goMatch[1]
		}
		result = append(result, migrationFile{Name: name, ID: id, Path: path,
			Checksum: hex.EncodeToString(sum[:]), LegacyChecksums: legacyChecksums, Plan: plan})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == result[j].ID {
			return result[i].Path < result[j].Path
		}
		return result[i].ID < result[j].ID
	})
	for index := 1; index < len(result); index++ {
		if result[index-1].ID == result[index].ID {
			failures.add(displayMigrationPath(folder, result[index].Path), "ID de migration duplicado %s, já usado por %s",
				result[index].ID, displayMigrationPath(folder, result[index-1].Path))
		}
	}
	validatePlans(result, failures)
	if err := failures.orNil(); err != nil {
		return nil, err
	}
	aliases := map[string]string{}
	views := map[string]bool{}
	for _, file := range result {
		for _, operation := range file.Plan.Operations {
			if operation.Kind == string(acao.CreateTable) {
				aliases[operation.AliasName] = operation.Table
			} else if operation.Kind == string(acao.RenameTable) {
				if _, exists := aliases[operation.Table]; exists {
					aliases[operation.Table] = operation.NewName
				}
			}
			if operation.Kind == string(acao.CreateView) || operation.Kind == string(acao.AlterView) || operation.Kind == string(acao.DropView) {
				views[operation.Name] = true
			}
		}
	}
	if projectRoot != "" {
		if err := migrationgo.WriteCoreCatalog(projectRoot, aliases); err != nil {
			return nil, fmt.Errorf("gerar catálogo de aliases no core: %w", err)
		}
		if err := migrationgo.WriteCoreViewCatalog(projectRoot, views); err != nil {
			return nil, fmt.Errorf("gerar catálogo de views no core: %w", err)
		}
	}
	return result, nil
}

func displayMigrationPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(relative)
}

func findProjectRoot(path string) string {
	current, _ := filepath.Abs(path)
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// validatePlans percorre todo o corpus registrando cada problema em failures.
// O estado (aliases, tabelas, views) avança mesmo quando uma operação falha,
// para que um erro isolado não gere uma cascata de falsos positivos nos
// arquivos seguintes.
func validatePlans(files []migrationFile, failures *LoadError) {
	physical := map[string]string{}
	aliases := map[string]string{}
	views := map[string]bool{}
	// Chaves candidatas de FK: por tabela física, o conjunto de colunas coberto
	// por uma PK ou UNIQUE já declarada. Sem isso a FK só falha no banco.
	keyed := newKeyIndex()
	constraintNames := map[string]string{}
	for _, file := range files {
		for _, operation := range file.Plan.Operations {
			keyed.observe(operation)
			if err := acao.Validar(operation); err != nil {
				failures.add(file.Name, "%v", err)
			}
			switch operation.Kind {
			case string(acao.CreateView):
				if views[operation.Name] {
					failures.add(file.Name, "view %q já existe", operation.Name)
				}
				views[operation.Name] = true
				continue
			case string(acao.AlterView):
				if !views[operation.Name] {
					failures.add(file.Name, "AlterView exige uma view existente")
				}
				continue
			case string(acao.DropView):
				if !views[operation.Name] {
					failures.add(file.Name, "DropView exige uma view existente")
				}
				views[operation.Name] = false
				continue
			}
			if referenced := referencedColumns(operation); len(referenced) > 0 {
				if missing := keyed.unknownColumns(operation.Table, referenced); len(missing) > 0 {
					failures.add(file.Name, "%s em %q usa a(s) coluna(s) %s, que nenhuma migration anterior criou",
						operation.Kind, operation.Table, strings.Join(missing, ", "))
				}
			}
			if operation.Kind == string(acao.AddForeignKey) && operation.ForeignKey != nil {
				if detail := keyed.missingParentKey(operation); detail != "" {
					failures.add(file.Name, "%s", detail)
				}
				if detail := keyed.mismatchedForeignKeyTypes(operation); detail != "" {
					failures.add(file.Name, "%s", detail)
				}
			}
			// No Oracle o nome da constraint é único no schema inteiro, então a
			// colisão é verificada globalmente e não por tabela.
			if name := constraintNameOf(operation); name != "" {
				key := strings.ToLower(name)
				if previous, taken := constraintNames[key]; taken {
					failures.add(file.Name, "o nome de constraint/índice %q já foi usado em %s; nomes precisam ser únicos no schema", name, previous)
				} else {
					constraintNames[key] = file.Name
				}
			}
			if operation.Kind != string(acao.CreateTable) {
				if requiresTableAlias(operation.Kind) {
					reference := strings.ToLower(operation.Table)
					_, knownAlias := aliases[reference]
					_, knownPhysical := physical[reference]
					if !knownAlias && !knownPhysical {
						failures.add(file.Name, "alias de tabela %q não foi declarado por nenhum CreateTable anterior", operation.Table)
					}
					if operation.Kind == string(acao.RenameTable) {
						newKey := strings.ToLower(operation.NewName)
						if previous, exists := physical[newKey]; exists {
							failures.add(file.Name, "RenameTable usaria o nome físico %q já declarado em %s", operation.NewName, previous)
						}
						physical[newKey] = file.Name
					}
				}
				continue
			}
			key := strings.ToLower(operation.Table)
			if previous, exists := physical[key]; exists {
				failures.add(file.Name, "CreateTable duplicado para %q, já declarado em %s", operation.Table, previous)
			}
			alias := strings.ToLower(operation.AliasName)
			if previous, exists := aliases[alias]; exists {
				failures.add(file.Name, "alias %q duplicado, já declarado em %s", operation.AliasName, previous)
			}
			physical[key], aliases[alias] = file.Name, file.Name
		}
	}
}

// keyIndex acompanha, ao longo do corpus, quais conjuntos de colunas de cada
// tabela já são chave (PK ou UNIQUE). Uma FK só é aceita pelo banco quando o
// lado referenciado tem exatamente esse conjunto como chave.
type keyIndex struct {
	physicalOf map[string]string            // alias -> nome físico
	keys       map[string]map[string]bool   // tabela física -> conjunto normalizado
	tables     map[string]bool              // tabelas declaradas por CreateTable
	columns    map[string]map[string]bool   // tabela física -> colunas conhecidas
	types      map[string]map[string]string // tabela física -> coluna -> assinatura de tipo
	opaque     map[string]bool              // tabelas tocadas por SQL bruto
}

func newKeyIndex() *keyIndex {
	return &keyIndex{
		physicalOf: map[string]string{},
		keys:       map[string]map[string]bool{},
		tables:     map[string]bool{},
		columns:    map[string]map[string]bool{},
		types:      map[string]map[string]string{},
		opaque:     map[string]bool{},
	}
}

func (k *keyIndex) addColumns(table string, columns ...acao.ColunaDefinicao) {
	physical := k.resolve(table)
	if k.columns[physical] == nil {
		k.columns[physical] = map[string]bool{}
	}
	if k.types[physical] == nil {
		k.types[physical] = map[string]string{}
	}
	for _, column := range columns {
		name := strings.ToLower(column.Name)
		k.columns[physical][name] = true
		if column.Type != "" {
			k.types[physical][name] = columnTypeSignature(column)
		}
	}
}

// columnTypeSignature descreve o tipo de forma comparável entre duas colunas.
// O SQL Server recusa FK entre VARCHAR de tamanhos diferentes, então o
// tamanho faz parte da assinatura.
func columnTypeSignature(column acao.ColunaDefinicao) string {
	switch column.Type {
	case "string", "char":
		length := column.Length
		if length == 0 {
			length = 255
		}
		return fmt.Sprintf("%s(%d)", column.Type, length)
	case "decimal":
		precision, scale := column.Precision, column.Scale
		if precision == 0 {
			precision, scale = 19, 4
		}
		return fmt.Sprintf("decimal(%d,%d)", precision, scale)
	default:
		return column.Type
	}
}

// mismatchedForeignKeyTypes compara o tipo declarado das colunas dos dois lados
// da FK. Divergências só apareciam ao rodar, e no SQL Server com uma mensagem
// genérica ("Could not create constraint or index").
func (k *keyIndex) mismatchedForeignKeyTypes(operation acao.Operacao) string {
	fk := operation.ForeignKey
	child, parent := k.resolve(operation.Table), k.resolve(fk.ReferenceTable)
	if k.opaque[child] || k.opaque[parent] {
		return ""
	}
	locals, references := fkColumns(fk), fk.ReferenceColumns
	if len(references) == 0 && fk.ReferenceColumn != "" {
		references = []string{fk.ReferenceColumn}
	}
	if len(locals) != len(references) {
		return ""
	}
	for index := range locals {
		childType := k.types[child][strings.ToLower(locals[index])]
		parentType := k.types[parent][strings.ToLower(references[index])]
		if childType == "" || parentType == "" || childType == parentType {
			continue
		}
		return fmt.Sprintf("a FK %s.%s é %s mas %s.%s é %s; os dois lados precisam ter o mesmo tipo",
			operation.Table, locals[index], childType, fk.ReferenceTable, references[index], parentType)
	}
	return ""
}

// unknownColumns devolve as colunas citadas que não existem na tabela segundo
// o próprio corpus. Devolve nil quando a tabela é desconhecida ou foi tocada
// por SQL bruto, casos em que não há como afirmar nada.
func (k *keyIndex) unknownColumns(table string, names []string) []string {
	physical := k.resolve(table)
	if !k.tables[physical] || k.opaque[physical] {
		return nil
	}
	known := k.columns[physical]
	var missing []string
	for _, name := range names {
		if name != "" && !known[strings.ToLower(name)] {
			missing = append(missing, name)
		}
	}
	return missing
}

func (k *keyIndex) resolve(table string) string {
	key := strings.ToLower(table)
	if physical, exists := k.physicalOf[key]; exists {
		return physical
	}
	return key
}

func columnSetKey(columns []string) string {
	normalized := make([]string, 0, len(columns))
	for _, column := range columns {
		normalized = append(normalized, strings.ToLower(column))
	}
	sort.Strings(normalized)
	return strings.Join(normalized, ",")
}

func (k *keyIndex) addKey(table string, columns []string) {
	if len(columns) == 0 {
		return
	}
	physical := k.resolve(table)
	if k.keys[physical] == nil {
		k.keys[physical] = map[string]bool{}
	}
	k.keys[physical][columnSetKey(columns)] = true
}

func (k *keyIndex) observe(operation acao.Operacao) {
	switch acao.Tipo(operation.Kind) {
	case acao.CreateTable:
		physical := strings.ToLower(operation.Table)
		k.tables[physical] = true
		if operation.AliasName != "" {
			k.physicalOf[strings.ToLower(operation.AliasName)] = physical
		}
		for _, column := range operation.Columns {
			k.addColumns(operation.Table, column)
			if column.PrimaryKey || column.Unique {
				k.addKey(operation.Table, []string{column.Name})
			}
		}
	case acao.AddPrimaryKey, acao.AddUnique:
		k.addKey(operation.Table, operation.IndexColumns)
	case acao.CreateIndex:
		if operation.Unique {
			k.addKey(operation.Table, operation.IndexColumns)
		}
	case acao.AddColumn, acao.AlterColumn:
		if operation.Column != nil {
			k.addColumns(operation.Table, *operation.Column)
			if operation.Column.PrimaryKey || operation.Column.Unique {
				k.addKey(operation.Table, []string{operation.Column.Name})
			}
		}
	case acao.DropColumn:
		if operation.Column != nil {
			delete(k.columns[k.resolve(operation.Table)], strings.ToLower(operation.Column.Name))
		}
	case acao.RenameColumn:
		if operation.Column != nil {
			table := k.resolve(operation.Table)
			previous := strings.ToLower(operation.Column.Name)
			renamed := acao.ColunaDefinicao{Name: operation.NewName, Type: k.types[table][previous]}
			delete(k.columns[table], previous)
			delete(k.types[table], previous)
			k.addColumns(operation.Table, renamed)
		}
	case acao.DropTable:
		delete(k.tables, k.resolve(operation.Table))
	case acao.RawSQL:
		// SQL bruto pode criar tabelas e colunas que o DSL não enxerga; as
		// tabelas citadas viram opacas para não gerar falso positivo.
		k.markOpaque(operation.SQL)
	}
}

var rawSQLTable = regexp.MustCompile(`(?i)\b(?:alter|create)\s+table\s+(?:if\s+not\s+exists\s+)?([a-z0-9_."]+)`)

func (k *keyIndex) markOpaque(statement string) {
	for _, match := range rawSQLTable.FindAllStringSubmatch(statement, -1) {
		table := strings.ToLower(strings.Trim(match[1], `"`))
		if index := strings.LastIndex(table, "."); index >= 0 {
			table = table[index+1:]
		}
		k.opaque[table] = true
	}
}

// missingParentKey devolve a descrição do problema quando a FK aponta para um
// conjunto de colunas que ainda não é chave. Devolve "" quando está tudo certo
// ou quando não há informação suficiente para afirmar o contrário.
func (k *keyIndex) missingParentKey(operation acao.Operacao) string {
	fk := operation.ForeignKey
	references := fk.ReferenceColumns
	if len(references) == 0 && fk.ReferenceColumn != "" {
		references = []string{fk.ReferenceColumn}
	}
	if len(references) == 0 {
		return ""
	}
	parent := k.resolve(fk.ReferenceTable)
	// Tabela desconhecida ou tocada por SQL bruto: não dá para afirmar nada.
	if !k.tables[parent] || k.opaque[parent] {
		return ""
	}
	if k.keys[parent][columnSetKey(references)] {
		return ""
	}
	return fmt.Sprintf(
		"a FK de %s(%s) referencia %s(%s), mas essas colunas ainda não são PRIMARY KEY nem UNIQUE; declare AddUnique/AddPrimaryKey em %s antes desta migration",
		operation.Table, strings.Join(fkColumns(fk), ", "), fk.ReferenceTable, strings.Join(references, ", "), fk.ReferenceTable)
}

// constraintNameOf devolve o nome que a operação vai reservar no schema, já
// resolvendo o nome gerado automaticamente quando a FK não declara um.
func constraintNameOf(operation acao.Operacao) string {
	switch acao.Tipo(operation.Kind) {
	case acao.CreateIndex, acao.AddPrimaryKey, acao.AddUnique, acao.AddCheck:
		return operation.Name
	case acao.AddForeignKey:
		if operation.ForeignKey == nil {
			return ""
		}
		if operation.ForeignKey.ConstraintName != "" {
			return operation.ForeignKey.ConstraintName
		}
		return foreignKeyName(operation.Table, operation.ForeignKey.Column)
	default:
		return ""
	}
}

// referencedColumns lista as colunas que a operação exige que já existam na
// própria tabela. CreateTable e AddColumn ficam de fora: elas criam colunas.
func referencedColumns(operation acao.Operacao) []string {
	switch acao.Tipo(operation.Kind) {
	case acao.CreateIndex, acao.AddPrimaryKey, acao.AddUnique:
		return operation.IndexColumns
	case acao.AddForeignKey:
		if operation.ForeignKey == nil {
			return nil
		}
		return fkColumns(operation.ForeignKey)
	case acao.AlterColumn, acao.DropColumn, acao.RenameColumn:
		if operation.Column == nil {
			return nil
		}
		return []string{operation.Column.Name}
	case acao.SeedRows:
		seen := map[string]bool{}
		var columns []string
		for _, row := range operation.Rows {
			for column := range row {
				if !seen[column] {
					seen[column] = true
					columns = append(columns, column)
				}
			}
		}
		sort.Strings(columns)
		return columns
	default:
		return nil
	}
}

func fkColumns(fk *acao.ForeignKey) []string {
	if len(fk.Columns) > 0 {
		return fk.Columns
	}
	return []string{fk.Column}
}

func requiresTableAlias(kind string) bool {
	switch acao.Tipo(kind) {
	case acao.DropTable, acao.AddColumn, acao.AlterColumn, acao.DropColumn,
		acao.AddForeignKey, acao.DropForeignKey, acao.CreateIndex, acao.DropIndex, acao.RenameTable,
		acao.RenameColumn, acao.AddPrimaryKey, acao.AddUnique, acao.AddCheck, acao.DropConstraint:
		return true
	default:
		return false
	}
}

func runConnection(connection config.ConnConfig, historyTable string, files []migrationFile) (int, int, error) {
	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{
		"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver",
	}[dialect]
	if driver == "" {
		return 0, 0, fmt.Errorf("dialeto não suportado: %s", dialect)
	}
	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return 0, 0, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return 0, 0, err
	}
	if err := ensureHistory(ctx, db, dialect, connection.Schema, historyTable); err != nil {
		return 0, 0, fmt.Errorf("criar %s: %w", historyTable, err)
	}
	history, batch, err := loadHistory(ctx, db, dialect, connection.Schema, historyTable)
	if err != nil {
		return 0, 0, err
	}
	batch++
	applied, skipped := 0, 0
	aliases := map[string]string{}
	cache := newSchemaCache(dialect, connection.Schema)
	for _, file := range files {
		checksum, exists := history[file.ID]
		if !exists { // Compatibilidade com históricos antigos baseados no nome do arquivo.
			checksum, exists = history[file.Name]
		}
		if exists {
			if checksum != file.Checksum && !containsChecksum(file.LegacyChecksums, checksum) {
				return applied, skipped, fmt.Errorf("migration %s foi alterada depois de executada", file.Name)
			}
			skipped++
			advanceAliases(aliases, file.Plan.Operations)
			continue
		}
		created := map[string]bool{}
		for _, operation := range file.Plan.Operations {
			resolved := resolveAlias(operation, aliases)
			if err := executeOperation(ctx, db, dialect, connection.Schema, resolved, created, cache); err != nil {
				return applied, skipped, fmt.Errorf("%s: %w", file.Name, err)
			}
		}
		if err := insertHistory(ctx, db, dialect, connection.Schema, historyTable, file, batch); err != nil {
			return applied, skipped, fmt.Errorf("registrar %s: %w", file.Name, err)
		}
		advanceAliases(aliases, file.Plan.Operations)
		applied++
	}
	return applied, skipped, nil
}

func legacyViewChecksums(operations []acao.Operacao) []string {
	hasView := false
	legacy := make([]acao.Operacao, len(operations))
	copy(legacy, operations)
	for index := range legacy {
		if legacy[index].Kind != string(acao.CreateView) || len(legacy[index].ViewSQL) == 0 {
			continue
		}
		hasView = true
		legacy[index].SQL = legacy[index].ViewSQL["common"]
		legacy[index].ViewSQL = nil
	}
	if !hasView {
		return nil
	}
	values := []string{checksumOperations(legacy)}
	crlf := make([]acao.Operacao, len(legacy))
	copy(crlf, legacy)
	for index := range crlf {
		if crlf[index].Kind == string(acao.CreateView) {
			crlf[index].SQL = strings.ReplaceAll(crlf[index].SQL, "\n", "\r\n")
		}
	}
	values = append(values, checksumOperations(crlf))
	return values
}

func checksumOperations(operations []acao.Operacao) string {
	data, _ := json.Marshal(operations)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func containsChecksum(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func resolveAlias(operation acao.Operacao, aliases map[string]string) acao.Operacao {
	if operation.Kind != string(acao.CreateTable) {
		if physical, exists := aliases[operation.Table]; exists {
			operation.Table = physical
		}
	}
	if operation.ForeignKey != nil {
		copyFK := *operation.ForeignKey
		if physical, exists := aliases[copyFK.ReferenceTable]; exists {
			copyFK.ReferenceTable = physical
		}
		operation.ForeignKey = &copyFK
	}
	return operation
}

func advanceAliases(aliases map[string]string, operations []acao.Operacao) {
	for _, operation := range operations {
		switch operation.Kind {
		case string(acao.CreateTable):
			aliases[operation.AliasName] = operation.Table
		case string(acao.RenameTable):
			if _, exists := aliases[operation.Table]; exists {
				aliases[operation.Table] = operation.NewName
				continue
			}
			for alias, physical := range aliases {
				if physical == operation.Table {
					aliases[alias] = operation.NewName
				}
			}
		}
	}
}

func executeOperation(ctx context.Context, db *sql.DB, dialect, schema string, operation acao.Operacao, created map[string]bool, cache *schemaCache) error {
	switch operation.Kind {
	case "create_table":
		exists, err := cache.hasTable(ctx, db, operation.Table)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		query, err := createTableSQL(dialect, schema, operation.Table, operation.Columns)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar tabela %s: %w", operation.Table, err)
		}
		cache.noteTable(operation.Table, operation.Columns)
		created[operation.Table] = true
	case "add_column":
		if operation.Column == nil {
			return fmt.Errorf("operação add_column inválida")
		}
		exists, err := cache.hasColumn(ctx, db, operation.Table, operation.Column.Name)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		query := fmt.Sprintf("ALTER TABLE %s ADD %s", qualified(dialect, schema, operation.Table), columnDefinition(dialect, *operation.Column, false))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("adicionar %s.%s: %w", operation.Table, operation.Column.Name, err)
		}
		cache.noteColumn(operation.Table, *operation.Column)
	case "alter_column":
		if operation.Column == nil {
			return fmt.Errorf("operação alter_column inválida")
		}
		var currentNullable *bool
		if dialect == "oracle" {
			nullable, err := oracleColumnNullable(ctx, db, schema, operation.Table, operation.Column.Name)
			if err != nil {
				return err
			}
			currentNullable = &nullable
		}
		for _, query := range alterColumnSQL(dialect, schema, operation.Table, *operation.Column, currentNullable) {
			if _, err := db.ExecContext(ctx, query); err != nil {
				return fmt.Errorf("alterar %s.%s: %w", operation.Table, operation.Column.Name, err)
			}
		}
	case "drop_column":
		if operation.Column == nil {
			return fmt.Errorf("operação drop_column inválida")
		}
		query := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", qualified(dialect, schema, operation.Table), quote(dialect, operation.Column.Name))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remover %s.%s: %w", operation.Table, operation.Column.Name, err)
		}
	case "drop_table":
		if _, err := db.ExecContext(ctx, dropTableSQL(dialect, schema, operation.Table)); err != nil {
			return fmt.Errorf("remover tabela %s: %w", operation.Table, err)
		}
	case "add_foreign_key":
		if operation.ForeignKey == nil {
			return fmt.Errorf("operação add_foreign_key inválida")
		}
		fk := operation.ForeignKey
		name := fk.ConstraintName
		if name == "" {
			name = foreignKeyName(operation.Table, fk.Column)
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
		query := fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)%s",
			qualified(dialect, schema, operation.Table), quote(dialect, name), strings.Join(quotedColumns(dialect, columns), ", "),
			qualified(dialect, schema, fk.ReferenceTable), strings.Join(quotedColumns(dialect, references), ", "), onDelete,
		)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar relacionamento %s.%s: %w", operation.Table, fk.Column, err)
		}
	case "drop_foreign_key":
		if operation.ForeignKey == nil {
			return fmt.Errorf("operação drop_foreign_key inválida")
		}
		name := operation.ForeignKey.ConstraintName
		if name == "" {
			name = foreignKeyName(operation.Table, operation.ForeignKey.Column)
		}
		query := dropForeignKeySQL(dialect, schema, operation.Table, name)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remover relacionamento %s.%s: %w", operation.Table, operation.ForeignKey.Column, err)
		}
	case "create_index":
		unique := ""
		if operation.Unique {
			unique = "UNIQUE "
		}
		columns, err := indexColumnsSQL(ctx, db, dialect, schema, operation.Table, operation.IndexColumns, cache)
		if err != nil {
			return fmt.Errorf("planejar índice %s: %w", operation.Name, err)
		}
		query := fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", unique, quote(dialect, operation.Name), qualified(dialect, schema, operation.Table), strings.Join(columns, ", "))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar índice %s: %w", operation.Name, err)
		}
	case "drop_index":
		query := fmt.Sprintf("DROP INDEX %s", quote(dialect, operation.Name))
		if dialect == "mysql" {
			query = fmt.Sprintf("DROP INDEX %s ON %s", quote(dialect, operation.Name), qualified(dialect, schema, operation.Table))
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remover índice %s: %w", operation.Name, err)
		}
	case "create_view":
		exists, err := viewExists(ctx, db, dialect, schema, operation.Name)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("criar view %s: a view já existe; use AlterView", operation.Name)
		}
		query, err := selectedViewSQL(operation, dialect)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE VIEW %s AS %s", qualified(dialect, schema, operation.Name), query)); err != nil {
			return fmt.Errorf("criar view %s usando %s.sql: %w; revise o SQL ou crie %s.sql para este dialeto", operation.Name, selectedViewSource(operation, dialect), err, dialect)
		}
	case "alter_view":
		exists, err := viewExists(ctx, db, dialect, schema, operation.Name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("alterar view %s: a view não existe; use CreateView", operation.Name)
		}
		query, err := selectedViewSQL(operation, dialect)
		if err != nil {
			return err
		}
		verb := "CREATE OR REPLACE VIEW"
		if dialect == "sqlserver" {
			verb = "CREATE OR ALTER VIEW"
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf("%s %s AS %s", verb, qualified(dialect, schema, operation.Name), query)); err != nil {
			return fmt.Errorf("alterar view %s usando %s.sql: %w; revise o SQL ou crie %s.sql para este dialeto", operation.Name, selectedViewSource(operation, dialect), err, dialect)
		}
	case "drop_view":
		exists, err := viewExists(ctx, db, dialect, schema, operation.Name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("remover view %s: a view não existe", operation.Name)
		}
		if _, err := db.ExecContext(ctx, "DROP VIEW "+qualified(dialect, schema, operation.Name)); err != nil {
			return fmt.Errorf("remover view %s: %w", operation.Name, err)
		}
	case "create_sequence":
		if dialect == "mysql" {
			return fmt.Errorf("sequences não são suportadas pelo MySQL")
		}
		if _, err := db.ExecContext(ctx, "CREATE SEQUENCE "+qualified(dialect, schema, operation.Name)); err != nil {
			return fmt.Errorf("criar sequence %s: %w", operation.Name, err)
		}
	case "drop_sequence":
		if dialect == "mysql" {
			return fmt.Errorf("sequences não são suportadas pelo MySQL")
		}
		if _, err := db.ExecContext(ctx, "DROP SEQUENCE "+qualified(dialect, schema, operation.Name)); err != nil {
			return fmt.Errorf("remover sequence %s: %w", operation.Name, err)
		}
	case "rename_table":
		query := fmt.Sprintf("ALTER TABLE %s RENAME TO %s", qualified(dialect, schema, operation.Table), quote(dialect, operation.NewName))
		if dialect == "sqlserver" {
			query = fmt.Sprintf("EXEC sp_rename N'%s', N'%s'", objectName(schema, operation.Table), operation.NewName)
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("renomear tabela %s: %w", operation.Table, err)
		}
	case "rename_column":
		if operation.Column == nil {
			return fmt.Errorf("operação rename_column inválida")
		}
		query := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", qualified(dialect, schema, operation.Table), quote(dialect, operation.Column.Name), quote(dialect, operation.NewName))
		if dialect == "sqlserver" {
			query = fmt.Sprintf("EXEC sp_rename N'%s.%s', N'%s', N'COLUMN'", objectName(schema, operation.Table), operation.Column.Name, operation.NewName)
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("renomear coluna %s.%s: %w", operation.Table, operation.Column.Name, err)
		}
	case "add_primary_key", "add_unique":
		columns := quotedColumns(dialect, operation.IndexColumns)
		constraintType := "PRIMARY KEY"
		if operation.Kind == string(acao.AddUnique) {
			constraintType = "UNIQUE"
		}
		// Oracle, Postgres e MySQL promovem as colunas da PK para NOT NULL
		// sozinhos; o SQL Server recusa a constraint e só devolve
		// "Could not create constraint or index".
		if dialect == "sqlserver" && constraintType == "PRIMARY KEY" {
			for _, column := range operation.IndexColumns {
				if err := sqlServerRequireNotNull(ctx, db, schema, operation.Table, column); err != nil {
					return fmt.Errorf("preparar %s.%s para a chave primária: %w", operation.Table, column, err)
				}
			}
		}
		query := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s (%s)", qualified(dialect, schema, operation.Table), quote(dialect, operation.Name), constraintType, strings.Join(columns, ", "))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar constraint %s: %w", operation.Name, err)
		}
	case "add_check":
		query := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s)", qualified(dialect, schema, operation.Table), quote(dialect, operation.Name), checkExpressionSQL(dialect, operation.SQL))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("criar check %s: %w", operation.Name, err)
		}
	case "drop_constraint":
		query := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", qualified(dialect, schema, operation.Table), quote(dialect, operation.Name))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("remover constraint %s: %w", operation.Name, err)
		}
	case "seed_rows":
		return executeSeedRows(ctx, db, dialect, schema, operation)
	case "raw_sql":
		if operation.Dialect != "" && operation.Dialect != "all" && !strings.EqualFold(operation.Dialect, dialect) {
			return nil
		}
		// SQL bruto pode criar tabelas e colunas fora do DSL, então o retrato
		// do schema deixa de valer.
		cache.invalidate()
		statements := splitRawSQL(operation.SQL)
		for index, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				if len(statements) > 1 {
					return fmt.Errorf("executar SQL específico de %s (statement %d de %d): %w",
						operation.Dialect, index+1, len(statements), err)
				}
				return fmt.Errorf("executar SQL específico de %s: %w", operation.Dialect, err)
			}
		}
	default:
		return fmt.Errorf("operação desconhecida: %s", operation.Kind)
	}
	return nil
}

// splitRawSQL quebra um bloco de SQL bruto nos statements que os drivers
// aceitam individualmente. Uma linha contendo apenas "/" é a diretiva do
// SQL*Plus que encerra e executa o bloco PL/SQL anterior — ela não é SQL e
// não pode ser enviada ao banco, mas marca a fronteira entre statements.
func splitRawSQL(statement string) []string {
	var result []string
	var current []string
	flush := func() {
		chunk := normalizeStatement(strings.Join(current, "\n"))
		if chunk != "" {
			result = append(result, chunk)
		}
		current = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(statement, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "/" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return result
}

// normalizeStatement remove o ";" final quando ele é um terminador de cliente.
// Dentro de um bloco PL/SQL anônimo o ponto e vírgula faz parte da sintaxe e
// precisa ser preservado.
func normalizeStatement(statement string) string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return ""
	}
	if strings.HasSuffix(statement, ";") && !endsPLSQLBlock(statement) {
		return strings.TrimSpace(strings.TrimSuffix(statement, ";"))
	}
	return statement
}

func endsPLSQLBlock(statement string) bool {
	upper := strings.ToUpper(strings.TrimRight(statement, " \t\r\n;"))
	return strings.HasSuffix(upper, "END")
}

// placeholder devolve o marcador de parâmetro do dialeto na posição informada.
func placeholder(dialect string, position int) string {
	switch dialect {
	case "postgres":
		return fmt.Sprintf("$%d", position)
	case "oracle":
		return fmt.Sprintf(":%d", position)
	case "sqlserver":
		return fmt.Sprintf("@p%d", position)
	default:
		return "?"
	}
}

// executeSeedRows aplica o seed fixo declarado em func Seeder().
//
// A regra de dono é o que impede perda de dado: numa aplicação nova, uma linha
// que já existe na chave declarada NÃO foi criada por este seed — pode ser um
// registro que a aplicação gerou. Sobrescrever seria apagar dado real em
// silêncio, então diverge = erro. Idêntica = no-op, o que mantém a operação
// reexecutável sem efeito colateral.
func executeSeedRows(ctx context.Context, db *sql.DB, dialect, schema string, operation acao.Operacao) error {
	if len(operation.Rows) == 0 {
		return nil
	}
	target := qualified(dialect, schema, operation.Table)

	transaction, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciar transação do seed de %s: %w", operation.Table, err)
	}
	defer transaction.Rollback()

	// SET IDENTITY_INSERT é por sessão: precisa da mesma conexão dos inserts,
	// por isso tudo acontece dentro da transação.
	identityInsert := dialect == "sqlserver" && operation.IdentityColumn != "" &&
		seedWritesColumn(operation.Rows, operation.IdentityColumn)
	if identityInsert {
		if _, err := transaction.ExecContext(ctx, "SET IDENTITY_INSERT "+target+" ON"); err != nil {
			return fmt.Errorf("habilitar IDENTITY_INSERT em %s: %w", operation.Table, err)
		}
	}

	inserted, unchanged := 0, 0
	for index, row := range operation.Rows {
		columns := sortedColumns(row)
		existing, found, err := seedExistingRow(ctx, transaction, dialect, target, operation.KeyColumns, columns, row)
		if err != nil {
			return fmt.Errorf("consultar %s (linha %d): %w", operation.Table, index+1, err)
		}
		if found {
			if difference := seedDifference(row, existing, columns); difference != "" {
				return cliui.NewUserError(
					fmt.Sprintf("seed de %s: a linha %s já existe no banco com outro conteúdo (%s).",
						operation.Table, seedKeyLabel(operation.KeyColumns, row), difference),
					"Essa linha não foi criada por este seed — pode ter vindo da aplicação. "+
						"Sobrescrever apagaria dado real. Ajuste o ID no Seeder() ou remova a linha do banco antes de reaplicar.",
				)
			}
			unchanged++
			continue
		}
		if err := seedInsert(ctx, transaction, dialect, target, columns, row); err != nil {
			return fmt.Errorf("inserir em %s (linha %d): %w", operation.Table, index+1, err)
		}
		inserted++
	}

	if identityInsert {
		if _, err := transaction.ExecContext(ctx, "SET IDENTITY_INSERT "+target+" OFF"); err != nil {
			return fmt.Errorf("desabilitar IDENTITY_INSERT em %s: %w", operation.Table, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("confirmar seed de %s: %w", operation.Table, err)
	}

	// Fora da transação: no Oracle e no MySQL o resync é DDL e faria commit
	// implícito no meio do lote.
	if operation.IdentityColumn != "" {
		if err := resyncIdentity(ctx, db, dialect, schema, operation.Table, operation.IdentityColumn); err != nil {
			return fmt.Errorf("ressincronizar a sequência de %s.%s: %w", operation.Table, operation.IdentityColumn, err)
		}
	}
	fmt.Printf("  %s %s: %d inserida(s), %d já presente(s)\n",
		cliui.Muted("·"), operation.Table, inserted, unchanged)
	return nil
}

func sortedColumns(row acao.Linha) []string {
	columns := make([]string, 0, len(row))
	for column := range row {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

func seedWritesColumn(rows []acao.Linha, column string) bool {
	for _, row := range rows {
		if _, present := row[column]; present {
			return true
		}
	}
	return false
}

func seedKeyLabel(keys []string, row acao.Linha) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, row[key]))
	}
	return strings.Join(parts, ", ")
}

func seedExistingRow(ctx context.Context, transaction *sql.Tx, dialect, target string,
	keys, columns []string, row acao.Linha) (map[string]any, bool, error) {

	predicate := make([]string, 0, len(keys))
	arguments := make([]any, 0, len(keys))
	for _, key := range keys {
		predicate = append(predicate, fmt.Sprintf("%s = %s", quote(dialect, key), placeholder(dialect, len(arguments)+1)))
		arguments = append(arguments, row[key])
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s",
		strings.Join(quotedColumns(dialect, columns), ", "), target, strings.Join(predicate, " AND "))

	values := make([]any, len(columns))
	scan := make([]any, len(columns))
	for index := range values {
		scan[index] = &values[index]
	}
	err := transaction.QueryRowContext(ctx, query, arguments...).Scan(scan...)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	result := make(map[string]any, len(columns))
	for index, column := range columns {
		result[column] = values[index]
	}
	return result, true, nil
}

// seedDifference descreve a primeira divergência entre o declarado e o banco,
// ou "" quando a linha já está exatamente como o seed pede.
func seedDifference(row acao.Linha, existing map[string]any, columns []string) string {
	for _, column := range columns {
		declared := normalizeSeedValue(row[column])
		stored := normalizeSeedValue(existing[column])
		if declared != stored {
			return fmt.Sprintf("%s: banco tem %q, seed declara %q", column, stored, declared)
		}
	}
	return ""
}

// normalizeSeedValue reduz valores a texto comparável. Os drivers devolvem o
// mesmo número como int64, float64, string ou []byte dependendo do banco.
func normalizeSeedValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "<nil>"
	case []byte:
		return string(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return normalizeSeedValue(float64(typed))
	case int:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		if typed {
			return "1"
		}
		return "0"
	case string:
		return strings.TrimRight(typed, " ")
	default:
		return fmt.Sprint(typed)
	}
}

func seedInsert(ctx context.Context, transaction *sql.Tx, dialect, target string,
	columns []string, row acao.Linha) error {

	markers := make([]string, len(columns))
	arguments := make([]any, len(columns))
	for index, column := range columns {
		markers[index] = placeholder(dialect, index+1)
		arguments[index] = row[column]
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		target, strings.Join(quotedColumns(dialect, columns), ", "), strings.Join(markers, ", "))
	_, err := transaction.ExecContext(ctx, query, arguments...)
	return err
}

// resyncIdentity coloca o gerador de identidade acima do maior valor existente,
// sem nunca andar para trás. A trava importa: se a sequência recuar, o banco
// reemite IDs que já foram entregues e as FKs passam a apontar para outra
// linha, sem erro nenhum.
func resyncIdentity(ctx context.Context, db *sql.DB, dialect, schema, table, column string) error {
	target := qualified(dialect, schema, table)
	quoted := quote(dialect, column)

	var maxUsed int64
	if err := db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COALESCE(MAX(%s), 0) FROM %s", quoted, target)).Scan(&maxUsed); err != nil {
		return err
	}
	current, err := currentIdentityNext(ctx, db, dialect, schema, table, column)
	if err != nil {
		return err
	}
	next := maxUsed + 1
	if current > next {
		next = current
	}

	switch dialect {
	case "postgres":
		_, err := db.ExecContext(ctx,
			"SELECT setval(pg_get_serial_sequence($1, $2), $3, false)",
			objectName(schemaOr(schema, "public"), table), column, next)
		return err
	case "mysql":
		_, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s AUTO_INCREMENT = %d", target, next))
		return err
	case "sqlserver":
		// RESEED grava o último valor usado, então o próximo entregue é next.
		_, err := db.ExecContext(ctx,
			fmt.Sprintf("DBCC CHECKIDENT('%s', RESEED, %d)", objectName(schemaOr(schema, "dbo"), table), next-1))
		return err
	default:
		_, err := db.ExecContext(ctx,
			fmt.Sprintf("ALTER TABLE %s MODIFY (%s GENERATED BY DEFAULT AS IDENTITY (START WITH %d))", target, quoted, next))
		return err
	}
}

// currentIdentityNext devolve o próximo valor que o gerador entregaria hoje, ou
// 0 quando o dialeto não expõe essa informação.
func currentIdentityNext(ctx context.Context, db *sql.DB, dialect, schema, table, column string) (int64, error) {
	var next sql.NullInt64
	switch dialect {
	case "postgres":
		// pg_sequences.last_value é NULL enquanto a sequência nunca foi usada;
		// nesse caso o próximo valor entregue é start_value.
		err := db.QueryRowContext(ctx, `
			SELECT COALESCE(last_value + 1, start_value)
			  FROM pg_sequences
			 WHERE schemaname || '.' || sequencename = pg_get_serial_sequence($1, $2)`,
			objectName(schemaOr(schema, "public"), table), column).Scan(&next)
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return next.Int64, err
	case "mysql":
		err := db.QueryRowContext(ctx, `
			SELECT AUTO_INCREMENT FROM information_schema.TABLES
			 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table).Scan(&next)
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return next.Int64, err
	case "sqlserver":
		err := db.QueryRowContext(ctx, "SELECT CAST(IDENT_CURRENT(@p1) AS BIGINT) + 1",
			objectName(schemaOr(schema, "dbo"), table)).Scan(&next)
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return next.Int64, err
	default:
		err := db.QueryRowContext(ctx, `
			SELECT s.last_number FROM user_sequences s
			  JOIN user_tab_identity_cols i ON i.sequence_name = s.sequence_name
			 WHERE i.table_name = :1 AND i.column_name = :2`,
			strings.ToUpper(table), strings.ToUpper(column)).Scan(&next)
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return next.Int64, err
	}
}

// sqlServerRequireNotNull converte a coluna para NOT NULL preservando o tipo
// atual, lido do catálogo. É no-op quando a coluna já é NOT NULL.
func sqlServerRequireNotNull(ctx context.Context, db *sql.DB, schema, table, column string) error {
	var dataType string
	var maxLength, precision, scale sql.NullInt64
	var isNullable string
	err := db.QueryRowContext(ctx, `
		SELECT DATA_TYPE, CHARACTER_MAXIMUM_LENGTH, NUMERIC_PRECISION, NUMERIC_SCALE, IS_NULLABLE
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = @p1 AND TABLE_NAME = @p2 AND COLUMN_NAME = @p3`,
		schemaOr(schema, "dbo"), table, column).Scan(&dataType, &maxLength, &precision, &scale, &isNullable)
	if err == sql.ErrNoRows {
		return fmt.Errorf("coluna não encontrada")
	}
	if err != nil {
		return err
	}
	if strings.EqualFold(isNullable, "NO") {
		return nil
	}

	rendered := dataType
	switch strings.ToLower(dataType) {
	case "varchar", "nvarchar", "char", "nchar", "varbinary", "binary":
		size := "MAX"
		if maxLength.Valid && maxLength.Int64 > 0 {
			size = fmt.Sprintf("%d", maxLength.Int64)
		}
		rendered = fmt.Sprintf("%s(%s)", dataType, size)
	case "decimal", "numeric":
		if precision.Valid {
			rendered = fmt.Sprintf("%s(%d,%d)", dataType, precision.Int64, scale.Int64)
		}
	}
	query := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s NOT NULL",
		qualified("sqlserver", schema, table), quote("sqlserver", column), rendered)
	_, err = db.ExecContext(ctx, query)
	return err
}

func selectedViewSQL(operation acao.Operacao, dialect string) (string, error) {
	if query := strings.TrimSpace(operation.ViewSQL[dialect]); query != "" {
		return query, nil
	}
	if query := strings.TrimSpace(operation.ViewSQL["common"]); query != "" {
		return query, nil
	}
	return "", fmt.Errorf("view %s não possui SQL para %s nem common.sql", operation.Name, dialect)
}

func selectedViewSource(operation acao.Operacao, dialect string) string {
	if strings.TrimSpace(operation.ViewSQL[dialect]) != "" {
		return dialect
	}
	return "common"
}

func indexColumnsSQL(ctx context.Context, db *sql.DB, dialect, schema, table string, columns []string, cache *schemaCache) ([]string, error) {
	result := quotedColumns(dialect, columns)
	if dialect != "mysql" || len(columns) == 0 {
		return result, nil
	}

	// InnoDB permits 3072 bytes per index. Four bytes per utf8mb4 character
	// gives a safe portable character budget, shared across composite columns.
	characterBudget := 768 / len(columns)
	if characterBudget < 1 {
		characterBudget = 1
	}
	for index, column := range columns {
		meta, found, err := cache.column(ctx, db, table, column)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("coluna %s.%s não existe", table, column)
		}
		isText := strings.Contains(meta.DataType, "char") || strings.Contains(meta.DataType, "text")
		if isText && (meta.Length == 0 || meta.Length > int64(characterBudget)) {
			result[index] = fmt.Sprintf("%s(%d)", quote(dialect, column), characterBudget)
		}
	}
	return result, nil
}

func quotedColumns(dialect string, columns []string) []string {
	result := make([]string, len(columns))
	for index, column := range columns {
		result[index] = quote(dialect, column)
	}
	return result
}

// checkExpressionSQL keeps migration CHECK expressions portable. Authors use
// plain logical column names; identifiers are quoted only for the target DB.
func checkExpressionSQL(dialect, expression string) string {
	keywords := map[string]bool{
		"AND": true, "OR": true, "NOT": true, "NULL": true, "IS": true,
		"IN": true, "BETWEEN": true, "LIKE": true, "TRUE": true, "FALSE": true,
		"CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
		"CURRENT_DATE": true, "CURRENT_TIMESTAMP": true, "CURRENT_USER": true,
	}

	var result strings.Builder
	for index := 0; index < len(expression); {
		if expression[index] == '\'' {
			start := index
			index++
			for index < len(expression) {
				if expression[index] != '\'' {
					index++
					continue
				}
				index++
				if index < len(expression) && expression[index] == '\'' {
					index++
					continue
				}
				break
			}
			result.WriteString(expression[start:index])
			continue
		}

		if isSQLIdentifierStart(expression[index]) {
			start := index
			index++
			for index < len(expression) && isSQLIdentifierPart(expression[index]) {
				index++
			}
			token := expression[start:index]
			lookahead := index
			for lookahead < len(expression) && strings.ContainsRune(" \t\r\n", rune(expression[lookahead])) {
				lookahead++
			}
			if keywords[strings.ToUpper(token)] || (lookahead < len(expression) && expression[lookahead] == '(') {
				result.WriteString(token)
			} else {
				result.WriteString(quote(dialect, token))
			}
			continue
		}

		result.WriteByte(expression[index])
		index++
	}
	return result.String()
}

func isSQLIdentifierStart(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isSQLIdentifierPart(value byte) bool {
	return isSQLIdentifierStart(value) || value >= '0' && value <= '9'
}

func foreignKeyName(table, column string) string {
	return shortenIdentifier("fk_"+table+"_"+column, 30)
}

// shortenIdentifier respeita o limite histórico de 30 caracteres do Oracle sem
// perder unicidade: truncar às cegas fazia duas FKs de prefixo longo em comum
// colidirem (ORA-02264). O sufixo é derivado do nome completo, então continua
// determinístico entre execuções.
func shortenIdentifier(name string, limit int) string {
	if len(name) <= limit {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := "_" + hex.EncodeToString(sum[:])[:6]
	return name[:limit-len(suffix)] + suffix
}

func dropForeignKeySQL(dialect, schema, table, name string) string {
	target, constraint := qualified(dialect, schema, table), quote(dialect, name)
	switch dialect {
	case "mysql":
		return fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s", target, constraint)
	case "postgres":
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s", target, constraint)
	default:
		return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", target, constraint)
	}
}

func ensureHistory(ctx context.Context, db *sql.DB, dialect, schema, table string) error {
	if dialect == "oracle" {
		if err := repairLegacyOracleHistory(ctx, db, schema, table); err != nil {
			return err
		}
	}
	target := qualified(dialect, schema, table)
	var query string
	switch dialect {
	case "postgres":
		query = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY, migration VARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, batch INTEGER NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`, target)
	case "mysql":
		query = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, migration VARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, batch INT NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`, target)
	case "sqlserver":
		query = fmt.Sprintf(`IF OBJECT_ID(N'%s', N'U') IS NULL CREATE TABLE %s (
			id BIGINT IDENTITY(1,1) PRIMARY KEY, migration NVARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, batch INT NOT NULL,
			applied_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME())`,
			objectName(schema, table), target)
	case "oracle":
		query = fmt.Sprintf(`BEGIN EXECUTE IMMEDIATE 'CREATE TABLE %s (
			id NUMBER(19) GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			migration VARCHAR2(255) NOT NULL UNIQUE, checksum VARCHAR2(64) NOT NULL,
			batch NUMBER(10) NOT NULL, applied_at TIMESTAMP DEFAULT SYSTIMESTAMP NOT NULL)';
			EXCEPTION WHEN OTHERS THEN IF SQLCODE != -955 THEN RAISE; END IF; END;`, target)
	default:
		return fmt.Errorf("dialeto não suportado: %s", dialect)
	}
	_, err := db.ExecContext(ctx, query)
	return err
}

func loadHistory(ctx context.Context, db *sql.DB, dialect, schema, table string) (map[string]string, int, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT migration, checksum, batch FROM %s ORDER BY id", qualified(dialect, schema, table)))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	history, maxBatch := map[string]string{}, 0
	for rows.Next() {
		var migration, checksum string
		var batch int
		if err := rows.Scan(&migration, &checksum, &batch); err != nil {
			return nil, 0, err
		}
		history[migration] = checksum
		if batch > maxBatch {
			maxBatch = batch
		}
	}
	return history, maxBatch, rows.Err()
}

func insertHistory(ctx context.Context, db *sql.DB, dialect, schema, table string, file migrationFile, batch int) error {
	target := qualified(dialect, schema, table)
	query := map[string]string{
		"postgres":  fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES ($1, $2, $3)", target),
		"oracle":    fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES (:1, :2, :3)", target),
		"mysql":     fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES (?, ?, ?)", target),
		"sqlserver": fmt.Sprintf("INSERT INTO %s (migration, checksum, batch) VALUES (@p1, @p2, @p3)", target),
	}[dialect]
	_, err := db.ExecContext(ctx, query, file.ID, file.Checksum, batch)
	return err
}

func createTableSQL(dialect, schema, table string, columns []acao.ColunaDefinicao) (string, error) {
	if len(columns) == 0 {
		return "", fmt.Errorf("tabela %s não possui colunas", table)
	}
	// Com mais de uma coluna marcada como PrimaryKey a chave é composta: o
	// PRIMARY KEY inline em cada coluna faria o banco recusar a tabela
	// (ORA-02260), então ela vira uma constraint no nível da tabela.
	var primaryKey []string
	for _, column := range columns {
		if column.PrimaryKey {
			primaryKey = append(primaryKey, column.Name)
		}
	}
	composite := len(primaryKey) > 1

	definitions := make([]string, 0, len(columns)+1)
	for _, column := range columns {
		if composite {
			column.PrimaryKey = false
		}
		definitions = append(definitions, columnDefinition(dialect, column, true))
		if dialect == "mysql" && column.Index && !column.PrimaryKey && !column.Unique {
			definitions = append(definitions, fmt.Sprintf("INDEX %s (%s)",
				quote(dialect, generatedIndexName(table, column.Name)), quote(dialect, column.Name)))
		}
	}
	if composite {
		definitions = append(definitions, fmt.Sprintf("CONSTRAINT %s PRIMARY KEY (%s)",
			quote(dialect, shortenIdentifier("pk_"+table, 30)), strings.Join(quotedColumns(dialect, primaryKey), ", ")))
	}
	// O MySQL exige que a coluna AUTO_INCREMENT seja chave; os outros dialetos
	// aceitam uma identidade que não participa da PK. Uma UNIQUE KEY própria
	// mantém o mesmo schema válido nos quatro bancos.
	if dialect == "mysql" {
		for _, column := range columns {
			if !column.AutoIncrement || column.PrimaryKey || column.Unique {
				continue
			}
			definitions = append(definitions, fmt.Sprintf("UNIQUE KEY %s (%s)",
				quote(dialect, shortenIdentifier("uk_"+table+"_"+column.Name, 64)), quote(dialect, column.Name)))
		}
	}
	return fmt.Sprintf("CREATE TABLE %s (%s)", qualified(dialect, schema, table), strings.Join(definitions, ", ")), nil
}

func generatedIndexName(table, column string) string {
	return shortenIdentifier("idx_"+table+"_"+column, 30)
}

func alterColumnSQL(dialect, schema, table string, column acao.ColunaDefinicao, currentNullable *bool) []string {
	target, name := qualified(dialect, schema, table), quote(dialect, column.Name)
	definition := columnDefinition(dialect, column, false)
	dataType := columnTypeSQL(dialect, column)
	switch dialect {
	case "postgres":
		nullability := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", target, name)
		if column.Nullable {
			nullability = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", target, name)
		}
		return []string{
			fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", target, name, dataType),
			nullability,
		}
	case "oracle":
		queries := []string{fmt.Sprintf("ALTER TABLE %s MODIFY (%s %s)", target, name, dataType)}
		if currentNullable != nil && *currentNullable != column.Nullable {
			nullability := "NOT NULL"
			if column.Nullable {
				nullability = "NULL"
			}
			queries = append(queries, fmt.Sprintf("ALTER TABLE %s MODIFY (%s %s)", target, name, nullability))
		}
		return queries
	case "mysql":
		return []string{fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s", target, definition)}
	default:
		return []string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s", target, definition)}
	}
}

func dropTableSQL(dialect, schema, table string) string {
	target := qualified(dialect, schema, table)
	switch dialect {
	case "postgres":
		return "DROP TABLE " + target + " CASCADE"
	case "oracle":
		return "DROP TABLE " + target + " CASCADE CONSTRAINTS PURGE"
	default:
		return "DROP TABLE " + target
	}
}

func columnDefinition(dialect string, column acao.ColunaDefinicao, allowIdentity bool) string {
	name := quote(dialect, column.Name)
	if column.AutoIncrement && allowIdentity {
		identity := map[string]string{
			"postgres": "BIGSERIAL", "oracle": "NUMBER(19) GENERATED BY DEFAULT AS IDENTITY",
			"mysql": "BIGINT AUTO_INCREMENT", "sqlserver": "BIGINT IDENTITY(1,1)",
		}[dialect]
		definition := name + " " + identity
		// DEFAULT precisa vir antes das constraints inline: o Oracle rejeita
		// "PRIMARY KEY DEFAULT x" com ORA-03076.
		if column.Default != "" {
			definition += " DEFAULT " + columnDefaultSQL(dialect, column)
		}
		if column.PrimaryKey {
			definition += " PRIMARY KEY"
		}
		if column.Unique && !column.PrimaryKey {
			definition += " UNIQUE"
		}
		return definition
	}
	// Ordem exigida pelo Oracle e aceita pelos demais dialetos:
	// tipo, DEFAULT, nulidade e só então as constraints inline.
	definition := name + " " + columnTypeSQL(dialect, column)
	if column.Default != "" {
		definition += " DEFAULT " + columnDefaultSQL(dialect, column)
	}
	if column.Nullable {
		definition += " NULL"
	} else {
		definition += " NOT NULL"
	}
	if column.PrimaryKey {
		definition += " PRIMARY KEY"
	} else if column.Unique {
		definition += " UNIQUE"
	}
	return definition
}

// columnTypeSQL devolve apenas o tipo físico da coluna no dialeto informado.
func columnTypeSQL(dialect string, column acao.ColunaDefinicao) string {
	stringLength := column.Length
	if stringLength == 0 {
		stringLength = 255
	}
	precision, scale := column.Precision, column.Scale
	if precision == 0 {
		precision, scale = 19, 4
	}
	return map[string]map[string]string{
		"int":       {"postgres": "INTEGER", "oracle": "NUMBER(10)", "mysql": "INT", "sqlserver": "INT"},
		"integer":   {"postgres": "BIGINT", "oracle": "NUMBER(19)", "mysql": "BIGINT", "sqlserver": "BIGINT"},
		"string":    {"postgres": fmt.Sprintf("VARCHAR(%d)", stringLength), "oracle": fmt.Sprintf("VARCHAR2(%d)", stringLength), "mysql": fmt.Sprintf("VARCHAR(%d)", stringLength), "sqlserver": fmt.Sprintf("NVARCHAR(%d)", stringLength)},
		"char":      {"postgres": fmt.Sprintf("CHAR(%d)", stringLength), "oracle": fmt.Sprintf("CHAR(%d)", stringLength), "mysql": fmt.Sprintf("CHAR(%d)", stringLength), "sqlserver": fmt.Sprintf("NCHAR(%d)", stringLength)},
		"text":      {"postgres": "TEXT", "oracle": "CLOB", "mysql": "LONGTEXT", "sqlserver": "NVARCHAR(MAX)"},
		"boolean":   {"postgres": "BOOLEAN", "oracle": "NUMBER(1)", "mysql": "BOOLEAN", "sqlserver": "BIT"},
		"decimal":   {"postgres": fmt.Sprintf("DECIMAL(%d,%d)", precision, scale), "oracle": fmt.Sprintf("NUMBER(%d,%d)", precision, scale), "mysql": fmt.Sprintf("DECIMAL(%d,%d)", precision, scale), "sqlserver": fmt.Sprintf("DECIMAL(%d,%d)", precision, scale)},
		"date":      {"postgres": "DATE", "oracle": "DATE", "mysql": "DATE", "sqlserver": "DATE"},
		"datetime":  {"postgres": "TIMESTAMPTZ", "oracle": "TIMESTAMP", "mysql": "DATETIME", "sqlserver": "DATETIME2"},
		"timestamp": {"postgres": "TIMESTAMP", "oracle": "TIMESTAMP", "mysql": "DATETIME", "sqlserver": "DATETIME2"},
		"binary":    {"postgres": "BYTEA", "oracle": "BLOB", "mysql": "LONGBLOB", "sqlserver": "VARBINARY(MAX)"},
	}[column.Type][dialect]
}

func columnDefaultSQL(dialect string, column acao.ColunaDefinicao) string {
	if !column.DefaultRaw {
		return defaultSQL(dialect, column.Default)
	}
	lower := strings.ToLower(strings.TrimSpace(column.Default))
	switch lower {
	case "current_user", "user":
		if dialect == "oracle" {
			return "USER"
		}
		if dialect == "mysql" {
			return "(CURRENT_USER())"
		}
		return "CURRENT_USER"
	case "current_timestamp", "sysdate", "systimestamp":
		if dialect == "oracle" {
			return "SYSTIMESTAMP"
		}
		return "CURRENT_TIMESTAMP"
	default:
		return column.Default
	}
}

var numericLiteral = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

func defaultSQL(dialect, value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "true" {
		if dialect == "oracle" {
			return "1"
		}
		return "TRUE"
	}
	if lower == "false" {
		if dialect == "oracle" {
			return "0"
		}
		return "FALSE"
	}
	if lower == "null" || lower == "current_timestamp" || numericLiteral.MatchString(lower) {
		return strings.ToUpper(lower)
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// schemaCache guarda um retrato das tabelas e colunas existentes. Sem ele cada
// create_table e cada add_column faziam uma consulta ao catálogo — mais de 400
// idas ao banco em um corpus de 267 migrations, e o information_schema do MySQL
// é lento o bastante para isso dominar o tempo total.
//
// O retrato é atualizado em memória conforme o próprio runner cria objetos, e
// invalidado depois de SQL bruto, que pode criar coisas que o DSL não enxerga.
type columnMeta struct {
	DataType string
	Length   int64
}

type schemaCache struct {
	dialect string
	schema  string
	tables  map[string]map[string]columnMeta
	loaded  bool
}

func newSchemaCache(dialect, schema string) *schemaCache {
	return &schemaCache{dialect: dialect, schema: schema, tables: map[string]map[string]columnMeta{}}
}

func (c *schemaCache) invalidate() { c.loaded = false }

func (c *schemaCache) load(ctx context.Context, db *sql.DB) error {
	if c.loaded {
		return nil
	}
	var query string
	var arguments []any
	switch c.dialect {
	case "postgres":
		query = "SELECT table_name, column_name, data_type, COALESCE(character_maximum_length, 0) FROM information_schema.columns WHERE table_schema=$1"
		arguments = []any{schemaOr(c.schema, "public")}
	case "mysql":
		query = "SELECT table_name, column_name, data_type, COALESCE(character_maximum_length, 0) FROM information_schema.columns WHERE table_schema=DATABASE()"
	case "sqlserver":
		query = "SELECT table_name, column_name, data_type, COALESCE(character_maximum_length, 0) FROM information_schema.columns WHERE table_schema=@p1"
		arguments = []any{schemaOr(c.schema, "dbo")}
	default:
		query = "SELECT table_name, column_name, data_type, NVL(char_length, 0) FROM all_tab_columns WHERE owner=:1"
		arguments = []any{strings.ToUpper(c.schema)}
	}
	rows, err := db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return err
	}
	defer rows.Close()
	tables := map[string]map[string]columnMeta{}
	for rows.Next() {
		var table, column, dataType string
		var length int64
		if err := rows.Scan(&table, &column, &dataType, &length); err != nil {
			return err
		}
		key := strings.ToLower(table)
		if tables[key] == nil {
			tables[key] = map[string]columnMeta{}
		}
		tables[key][strings.ToLower(column)] = columnMeta{DataType: strings.ToLower(dataType), Length: length}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	c.tables, c.loaded = tables, true
	return nil
}

func (c *schemaCache) hasTable(ctx context.Context, db *sql.DB, table string) (bool, error) {
	if err := c.load(ctx, db); err != nil {
		return false, err
	}
	_, exists := c.tables[strings.ToLower(table)]
	return exists, nil
}

func (c *schemaCache) hasColumn(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	if err := c.load(ctx, db); err != nil {
		return false, err
	}
	_, exists := c.tables[strings.ToLower(table)][strings.ToLower(column)]
	return exists, nil
}

func (c *schemaCache) column(ctx context.Context, db *sql.DB, table, name string) (columnMeta, bool, error) {
	if err := c.load(ctx, db); err != nil {
		return columnMeta{}, false, err
	}
	meta, exists := c.tables[strings.ToLower(table)][strings.ToLower(name)]
	return meta, exists, nil
}

func (c *schemaCache) noteTable(table string, columns []acao.ColunaDefinicao) {
	for _, column := range columns {
		c.noteColumn(table, column)
	}
}

// noteColumn registra a coluna recém-criada com o tipo já traduzido para o
// vocabulário do catálogo, para que o retrato em memória e o lido do banco
// sejam intercambiáveis.
func (c *schemaCache) noteColumn(table string, column acao.ColunaDefinicao) {
	dataType := map[string]string{
		"string": "varchar", "char": "char", "text": "text",
		"int": "integer", "integer": "bigint", "binary": "blob",
	}[column.Type]
	if dataType == "" {
		dataType = column.Type
	}
	length := int64(column.Length)
	if (column.Type == "string" || column.Type == "char") && length == 0 {
		length = 255
	}
	c.noteColumnMeta(table, column.Name, columnMeta{DataType: dataType, Length: length})
}

func (c *schemaCache) noteColumnMeta(table, column string, meta columnMeta) {
	key := strings.ToLower(table)
	if c.tables[key] == nil {
		c.tables[key] = map[string]columnMeta{}
	}
	c.tables[key][strings.ToLower(column)] = meta
}

func tableExists(ctx context.Context, db *sql.DB, dialect, schema, table string) (bool, error) {
	var count int
	switch dialect {
	case "postgres":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name=$2", schemaOr(schema, "public"), table).Scan(&count)
		return count > 0, err
	case "mysql":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", table).Scan(&count)
		return count > 0, err
	case "sqlserver":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=@p1 AND table_name=@p2", schemaOr(schema, "dbo"), table).Scan(&count)
		return count > 0, err
	default:
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM all_tables WHERE owner=:1 AND table_name=:2", strings.ToUpper(schema), strings.ToUpper(table)).Scan(&count)
		return count > 0, err
	}
}

func viewExists(ctx context.Context, db *sql.DB, dialect, schema, view string) (bool, error) {
	var count int
	switch dialect {
	case "postgres":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.views WHERE table_schema=$1 AND table_name=$2", schemaOr(schema, "public"), view).Scan(&count)
		return count > 0, err
	case "mysql":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.views WHERE table_schema=DATABASE() AND table_name=?", view).Scan(&count)
		return count > 0, err
	case "sqlserver":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.views WHERE table_schema=@p1 AND table_name=@p2", schemaOr(schema, "dbo"), view).Scan(&count)
		return count > 0, err
	default:
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM all_views WHERE owner=:1 AND view_name=:2", strings.ToUpper(schema), strings.ToUpper(view)).Scan(&count)
		return count > 0, err
	}
}

func columnExists(ctx context.Context, db *sql.DB, dialect, schema, table, column string) (bool, error) {
	var count int
	switch dialect {
	case "postgres":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND column_name=$3", schemaOr(schema, "public"), table, column).Scan(&count)
		return count > 0, err
	case "mysql":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?", table, column).Scan(&count)
		return count > 0, err
	case "sqlserver":
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=@p1 AND table_name=@p2 AND column_name=@p3", schemaOr(schema, "dbo"), table, column).Scan(&count)
		return count > 0, err
	default:
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM all_tab_columns WHERE owner=:1 AND table_name=:2 AND column_name=:3", strings.ToUpper(schema), strings.ToUpper(table), strings.ToUpper(column)).Scan(&count)
		return count > 0, err
	}
}

func oracleColumnNullable(ctx context.Context, db *sql.DB, schema, table, column string) (bool, error) {
	var nullable string
	err := db.QueryRowContext(ctx,
		"SELECT nullable FROM all_tab_columns WHERE owner=:1 AND table_name=:2 AND column_name=:3",
		strings.ToUpper(schema), strings.ToUpper(table), strings.ToUpper(column),
	).Scan(&nullable)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(nullable, "Y"), nil
}

func qualified(dialect, schema, table string) string {
	if strings.TrimSpace(schema) == "" || dialect == "mysql" {
		return quote(dialect, table)
	}
	return quote(dialect, schema) + "." + quote(dialect, table)
}

func quote(dialect, value string) string {
	switch dialect {
	case "mysql":
		return "`" + strings.ReplaceAll(value, "`", "``") + "`"
	case "sqlserver":
		return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
	case "oracle":
		return `"` + strings.ReplaceAll(strings.ToUpper(value), `"`, `""`) + `"`
	default:
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
}

func repairLegacyOracleHistory(ctx context.Context, db *sql.DB, schema, table string) error {
	owner := strings.ToUpper(schema)
	var current, legacy int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM all_tables WHERE owner=:1 AND table_name=:2",
		owner, strings.ToUpper(table),
	).Scan(&current); err != nil {
		return err
	}
	if current > 0 {
		return nil
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM all_tables WHERE owner=:1 AND table_name=:2",
		owner, table,
	).Scan(&legacy); err != nil {
		return err
	}
	if legacy == 0 {
		return nil
	}
	legacyTarget := `"` + strings.ReplaceAll(owner, `"`, `""`) + `"."` + strings.ReplaceAll(table, `"`, `""`) + `"`
	_, err := db.ExecContext(ctx, fmt.Sprintf(
		"ALTER TABLE %s RENAME TO %s",
		legacyTarget,
		quote("oracle", table),
	))
	return err
}

func historyLocation(connection config.ConnConfig, table string) string {
	dialect := strings.ToLower(connection.Dialect)
	schema := connection.Schema
	host, database := "", ""
	if dialect == "mysql" {
		value := connection.BuildURL()
		if start := strings.Index(value, "@tcp("); start >= 0 {
			remainder := value[start+5:]
			if end := strings.Index(remainder, ")"); end >= 0 {
				host = remainder[:end]
				after := strings.TrimPrefix(remainder[end+1:], "/")
				database = strings.SplitN(after, "?", 2)[0]
			}
		}
	} else if parsed, err := url.Parse(connection.BuildURL()); err == nil {
		host = parsed.Host
		database = strings.TrimPrefix(parsed.Path, "/")
		if dialect == "sqlserver" {
			database = parsed.Query().Get("database")
		}
	}
	location := host
	if database != "" {
		location += "/" + database
	}
	if schema != "" {
		location += "." + schema
	}
	if location != "" {
		location += "."
	}
	return location + table
}

func objectName(schema, table string) string {
	return schemaOr(schema, "dbo") + "." + table
}

func schemaOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func viewPhysicalName(alias string, root string, state config.ConfigState) string {
	pRoot := projectRoot(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if pRoot == "" {
		pRoot = root
	}
	viewFile := filepath.Join(pRoot, "internal", "gokit", "core", "migration", "view", "dsl.gen.go")
	if data, err := os.ReadFile(viewFile); err == nil {
		re := regexp.MustCompile(`(?m)^\s*var\s+` + regexp.QuoteMeta(alias) + `\s*=\s*migrate\.(?:RegisteredView|View)\("([^"]+)"\)`)
		if m := re.FindStringSubmatch(string(data)); len(m) > 1 {
			return m[1]
		}
	}
	name := strings.ToLower(alias)
	if strings.HasPrefix(name, "vw") && !strings.HasPrefix(name, "vw_") && len(name) > 2 {
		name = "vw_" + name[2:]
	}
	return name
}

func CreateScaffoldMigration(root string, state config.ConfigState, name string, method string, targetAlias string) (string, error) {
	migratePath := filepath.FromSlash(state.Config.Output.Migrate)
	migrateRoot := migratePath
	if !filepath.IsAbs(migratePath) {
		migrateRoot = filepath.Join(root, migratePath)
	}
	// Cada método tem sua subpasta. A ordem de execução continua vindo do
	// timestamp no nome do arquivo — a pasta é só organização.
	methodFolder := strings.ToLower(strings.TrimSpace(method))
	if methodFolder == "" {
		methodFolder = "todo"
	}
	outputDir := filepath.Join(migrateRoot, methodFolder)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("criar pasta de migrations: %w", err)
	}

	pRoot := projectRoot(outputDir)
	if pRoot == "" {
		pRoot = root
	}

	moduleName, err := GetModuleName(pRoot)
	if err != nil {
		moduleName = "prevcontas/backend" // Fallback seguro
	}

	// Sanitiza o nome em snake_case para o arquivo
	name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "_"))
	if name == "" {
		name = "migration"
	}

	timestamp := time.Now().Format("2006_01_02_150405")
	filename := fmt.Sprintf("%s_%s.go", timestamp, name)
	filePath := filepath.Join(outputDir, filename)

	// Cria arquivo SQL para views caso seja uma operação de view
	if strings.ToLower(method) == "create_view" || strings.ToLower(method) == "alter_view" {
		physicalViewName := name
		if targetAlias != "" {
			physicalViewName = viewPhysicalName(targetAlias, root, state)
		}
		if !strings.HasPrefix(physicalViewName, "vw_") {
			physicalViewName = "vw_" + physicalViewName
		}
		viewFolder := filepath.Join(outputDir, "views", physicalViewName, timestamp)
		if err := os.MkdirAll(viewFolder, 0755); err != nil {
			return "", fmt.Errorf("criar pasta de views: %w", err)
		}
		sqlFile := filepath.Join(viewFolder, "common.sql")
		if _, err := os.Stat(sqlFile); os.IsNotExist(err) {
			if err := os.WriteFile(sqlFile, []byte("SELECT 1 AS id FROM dual\n"), 0644); err != nil {
				return "", fmt.Errorf("escrever arquivo SQL da view: %w", err)
			}
		}
	}

	aliasRef := targetAlias
	if aliasRef == "" {
		aliasRef = tableIdentifier(name)
	}
	if len(aliasRef) > 0 {
		aliasRef = strings.ToUpper(aliasRef[:1]) + aliasRef[1:]
	}

	lowerAlias := targetAlias
	if lowerAlias == "" {
		lowerAlias = name
	}
	if len(lowerAlias) > 0 {
		lowerAlias = strings.ToLower(lowerAlias[:1]) + lowerAlias[1:]
	}

	viewRef := targetAlias
	if viewRef == "" {
		viewRef = viewIdentifier(name)
	}
	if len(viewRef) > 0 {
		viewRef = strings.ToUpper(viewRef[:1]) + viewRef[1:]
	}

	var operationBody string
	switch strings.ToLower(method) {
	case "create_table":
		operationBody = `migrate.CreateTable("` + name + `",
		migrate.Col("id").Integer().PrimaryKey().AutoIncrement(),
		migrate.Col("created_at").Timestamp().DefaultExpr("CURRENT_TIMESTAMP"),
	).Alias("` + lowerAlias + `")`
	case "drop_table":
		operationBody = `migrate.DropTable(alias.` + aliasRef + `)`
	case "add_column":
		operationBody = `migrate.AddColumn(alias.` + aliasRef + `,
		migrate.Col("nova_coluna").Varchar(255).Nullable(),
	)`
	case "alter_column":
		operationBody = `migrate.AlterColumn(alias.` + aliasRef + `,
		migrate.Col("coluna").Varchar(255).Nullable(),
	)`
	case "drop_column":
		operationBody = `migrate.DropColumn(alias.` + aliasRef + `,
		migrate.Col("coluna"),
	)`
	case "add_foreign_key":
		operationBody = `migrate.AddForeignKey(alias.` + aliasRef + `,
		migrate.Col("coluna_id").References("tabela_estrangeira", "id").OnDeleteCascade(),
	)`
	case "drop_foreign_key":
		operationBody = `migrate.DropForeignKey(alias.` + aliasRef + `,
		migrate.Col("coluna_id").References("tabela_estrangeira", "id"),
	)`
	case "create_index":
		operationBody = `migrate.CreateIndex(alias.` + aliasRef + `, "idx_` + name + `_coluna", "coluna")`
	case "drop_index":
		operationBody = `migrate.DropIndex(alias.` + aliasRef + `, "idx_` + name + `_coluna")`
	case "create_view":
		operationBody = `migrate.CreateView(view.` + viewRef + `)`
	case "alter_view":
		operationBody = `migrate.AlterView(view.` + viewRef + `)`
	case "drop_view":
		operationBody = `migrate.DropView(view.` + viewRef + `)`
	case "create_sequence":
		operationBody = `migrate.CreateSequence("sq_` + name + `")`
	case "drop_sequence":
		operationBody = `migrate.DropSequence("sq_` + name + `")`
	case "rename_table":
		operationBody = `migrate.RenameTable(alias.` + aliasRef + `, "novo_nome_tabela")`
	case "rename_column":
		operationBody = `migrate.RenameColumn(alias.` + aliasRef + `, "nome_antigo", "nome_novo")`
	case "add_primary_key":
		operationBody = `migrate.AddPrimaryKey(alias.` + aliasRef + `, "pk_` + name + `", "id")`
	case "add_unique":
		operationBody = `migrate.AddUnique(alias.` + aliasRef + `, "uk_` + name + `_coluna", "coluna")`
	case "add_check":
		operationBody = `migrate.AddCheck(alias.` + aliasRef + `, "chk_` + name + `_coluna", "coluna > 0")`
	case "drop_constraint":
		operationBody = `migrate.DropConstraint(alias.` + aliasRef + `, "constraint_nome")`
	case "raw_sql":
		operationBody = `migrate.SQL("common", "SELECT 1")`
	case "todo":
		operationBody = `migrate.TODO()`
	default:
		return "", fmt.Errorf("tipo de operação de migração desconhecido: %s", method)
	}

	importsBlock := `import migrate "gokit/migration"`
	if method != "todo" && method != "raw_sql" {
		importsBlock = `import (
	alias "` + moduleName + `/internal/gokit/core/migration/alias"
	view "` + moduleName + `/internal/gokit/core/migration/view"
	migrate "gokit/migration"
)`
	}

	content := `package migrations

` + importsBlock + `

func Migration() migrate.Definition {
	return migrate.Define(
		` + operationBody + `,
	)
}
`

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("escrever arquivo de migration: %w", err)
	}

	return filepath.ToSlash(filepath.Join(methodFolder, filename)), nil
}

func tableIdentifier(name string) string {
	parts := strings.Split(name, "_")
	for i := 0; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func viewIdentifier(name string) string {
	name = strings.ToLower(name)
	if !strings.HasPrefix(name, "vw") {
		name = "vw_" + name
	}
	parts := strings.Split(name, "_")
	for i := 0; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func projectRoot(startDir string) string {
	current := startDir
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

func LoadCatalogTablesAndViews(root string, state config.ConfigState) ([]string, []string, error) {
	migratePath := filepath.FromSlash(state.Config.Output.Migrate)
	outputDir := migratePath
	if !filepath.IsAbs(migratePath) {
		outputDir = filepath.Join(root, migratePath)
	}

	pRoot := projectRoot(outputDir)
	if pRoot == "" {
		pRoot = root
	}

	// Atualiza o catálogo antes de carregar
	_ = migrationgo.RefreshCatalog(pRoot, outputDir)

	var tables []string
	dslFile := filepath.Join(pRoot, "internal", "gokit", "core", "migration", "alias", "dsl.gen.go")
	if data, err := os.ReadFile(dslFile); err == nil {
		re := regexp.MustCompile(`(?m)^\s*var\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*migrate\.Table`)
		matches := re.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			tables = append(tables, m[1])
		}
	}

	var views []string
	viewFile := filepath.Join(pRoot, "internal", "gokit", "core", "migration", "view", "dsl.gen.go")
	if data, err := os.ReadFile(viewFile); err == nil {
		re := regexp.MustCompile(`(?m)^\s*var\s+([A-Za-z_][A-Za-z0-9_]*)\s*=`)
		matches := re.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			views = append(views, m[1])
		}
	}

	sort.Strings(tables)
	sort.Strings(views)
	return tables, views, nil
}

func GetModuleName(projectRoot string) (string, error) {
	goModPath := filepath.Join(projectRoot, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`(?m)^module\s+([^\s\n\r]+)`)
	matches := re.FindStringSubmatch(string(data))
	if len(matches) > 1 {
		return matches[1], nil
	}
	return "", fmt.Errorf("nome do módulo não encontrado em go.mod")
}
