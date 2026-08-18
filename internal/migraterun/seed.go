package migraterun

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/aviso"
	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// Os seeds vivem fora das migrations, em uma pasta por tabela:
//
//	database/seeds/<tabela>/<timestamp>_seeder.go
//
// O primeiro arquivo de cada pasta é o seed fixo — roda junto com o
// `migrate run`, logo depois das migrations, quando a tabela acabou de nascer.
// Os seguintes são correções e acréscimos, aplicados por `seed run`.
//
// Estarem fora da migration é o que torna a correção possível: mexer numa
// migration já aplicada dispara o drift de checksum, e com razão. Um seeder
// novo tem timestamp próprio e histórico próprio.

var seedFileName = regexp.MustCompile(`^(\d{4}_\d{2}_\d{2}_\d{6})_.*\.go$`)

type seedFile struct {
	Table     string // nome físico, vindo da pasta
	ID        string // <tabela>/<timestamp>, único no histórico
	Stamp     string
	Path      string
	Checksum  string
	Rows      []acao.Linha
	Keys      []string
	Identity  string
	Numericas map[string]bool // colunas de tipo numérico, para a comparação não ser textual
	First     bool            // primeiro da pasta: é o seed fixo
}

func seedRoot(root string, state config.ConfigState) string {
	folder := state.Config.Output.Seed
	if folder == "" {
		folder = "database/seeds"
	}
	return filepath.Join(root, filepath.FromSlash(folder))
}

// tableShapes reconstrói a forma final de cada tabela percorrendo o corpus na
// ordem. O CreateTable sozinho não basta: uma coluna acrescentada depois por
// AddColumn não apareceria, e o seed a trataria como desconhecida — sem
// coerção de tipo, uma data iria como texto e o Oracle recusaria (ORA-01843).
func tableShapes(root string, state config.ConfigState) (map[string]acao.Operacao, error) {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if err != nil {
		return nil, describeLoadError(err)
	}

	shapes := map[string]acao.Operacao{}
	for _, file := range files {
		for _, operation := range file.Plan.Operations {
			chave := strings.ToLower(operation.Table)
			// Depois do Expandir a coluna viaja no campo singular, uma
			// operação por coluna; Columns só sobrevive no CreateTable.
			colunas := operation.Columns
			if operation.Column != nil {
				colunas = []acao.ColunaDefinicao{*operation.Column}
			}

			switch operation.Kind {
			case string(acao.CreateTable):
				shapes[chave] = operation

			case string(acao.AddColumn), string(acao.AlterColumn):
				forma, existe := shapes[chave]
				if !existe {
					continue
				}
				for _, column := range colunas {
					forma.Columns = substituiColuna(forma.Columns, column)
				}
				shapes[chave] = forma

			case string(acao.DropColumn):
				forma, existe := shapes[chave]
				if !existe {
					continue
				}
				for _, column := range colunas {
					forma.Columns = removeColuna(forma.Columns, column.Name)
				}
				shapes[chave] = forma

			case string(acao.RenameColumn):
				forma, existe := shapes[chave]
				if !existe || operation.Column == nil {
					continue
				}
				for posicao := range forma.Columns {
					if strings.EqualFold(forma.Columns[posicao].Name, operation.Column.Name) {
						forma.Columns[posicao].Name = operation.NewName
						break
					}
				}
				shapes[chave] = forma

			case string(acao.RenameTable):
				forma, existe := shapes[chave]
				if !existe {
					continue
				}
				delete(shapes, chave)
				forma.Table = operation.NewName
				shapes[strings.ToLower(operation.NewName)] = forma

			case string(acao.DropTable):
				delete(shapes, chave)
			}
		}
	}
	return shapes, nil
}

// substituiColuna acrescenta a coluna ou atualiza a que já existe, preservando
// a posição original — a ordem das colunas é o que dá estabilidade ao INSERT.
func substituiColuna(colunas []acao.ColunaDefinicao, nova acao.ColunaDefinicao) []acao.ColunaDefinicao {
	for posicao := range colunas {
		if strings.EqualFold(colunas[posicao].Name, nova.Name) {
			colunas[posicao] = nova
			return colunas
		}
	}
	return append(colunas, nova)
}

func removeColuna(colunas []acao.ColunaDefinicao, nome string) []acao.ColunaDefinicao {
	restantes := colunas[:0]
	for _, coluna := range colunas {
		if !strings.EqualFold(coluna.Name, nome) {
			restantes = append(restantes, coluna)
		}
	}
	return restantes
}

func loadSeeds(root string, state config.ConfigState) ([]seedFile, error) {
	base := seedRoot(root, state)
	entries, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	shapes, err := tableShapes(root, state)
	if err != nil {
		return nil, err
	}

	failures := &LoadError{}
	var result []seedFile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		table := strings.ToLower(entry.Name())
		shape, known := shapes[table]
		folder := filepath.Join(base, entry.Name())

		files, err := os.ReadDir(folder)
		if err != nil {
			failures.add(entry.Name(), "%v", err)
			continue
		}
		var stamps []string
		byStamp := map[string]string{}
		for _, file := range files {
			match := seedFileName.FindStringSubmatch(file.Name())
			if file.IsDir() || match == nil {
				continue
			}
			stamps = append(stamps, match[1])
			byStamp[match[1]] = filepath.Join(folder, file.Name())
		}
		if len(stamps) == 0 {
			continue
		}
		if !known {
			failures.add(entry.Name(), i18n.T("sed_no_createtable_for"), table)
			continue
		}
		sort.Strings(stamps)

		var keys []string
		identity := ""
		for _, column := range shape.Columns {
			if column.PrimaryKey {
				keys = append(keys, column.Name)
			}
			if column.AutoIncrement {
				identity = column.Name
			}
		}

		// Colunas de tipo NUMÉRICO, para a comparação do seed não ser textual.
		//
		// Os drivers devolvem DECIMAL de formas diferentes: o do Oracle entrega float64, e os
		// dos outros três entregam texto com a escala completa. Um DECIMAL(15,4) com valor
		// 9.001 volta "9.0010" do MySQL, do Postgres e do SQL Server, contra o 9.001 que o
		// seed declara — e a comparação textual acusava divergência num dado que o próprio
		// gokit escreveu. Saber quais colunas são números é o que permite comparar por valor.
		numericas := map[string]bool{}
		for _, column := range shape.Columns {
			switch strings.ToLower(column.Type) {
			case "int", "integer", "decimal":
				numericas[strings.ToLower(column.Name)] = true
			}
		}
		for index, stamp := range stamps {
			path := byStamp[stamp]
			display := filepath.ToSlash(filepath.Join(entry.Name(), filepath.Base(path)))
			rows, err := migrationgo.ParseSeedFile(path, shape.Columns)
			if err != nil {
				failures.add(display, "%v", err)
				continue
			}
			payload, err := json.Marshal(rows)
			if err != nil {
				failures.add(display, "%v", err)
				continue
			}
			sum := sha256.Sum256(payload)
			result = append(result, seedFile{
				Table: shape.Table, ID: table + "/" + stamp, Stamp: stamp, Path: path,
				Checksum: hex.EncodeToString(sum[:]), Rows: rows,
				Keys: keys, Identity: identity, Numericas: numericas, First: index == 0,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	validateSeeds(result, failures)
	if err := failures.orNil(); err != nil {
		return nil, err
	}
	return result, nil
}

// validateSeeds aplica o contrato de ID fixo: só se edita linha cujo ID foi
// escrito explicitamente em algum seeder anterior da mesma tabela. ID gerado
// pelo banco muda de ambiente para ambiente e não é alvo válido de edição.
func validateSeeds(seeds []seedFile, failures *LoadError) {
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].ID < seeds[j].ID })
	fixos := map[string]map[string]bool{}

	for _, seed := range seeds {
		display := filepath.ToSlash(filepath.Join(seed.Table, filepath.Base(seed.Path)))
		// Tabela SEM chave primária é aceita: o seeder é soberano e substitui o conteúdo
		// inteiro. Sem chave não há o que conferir linha a linha — nem ID fixo, nem
		// divergência —, então as checagens abaixo não se aplicam e o arquivo passa como está.
		if len(seed.Keys) == 0 {
			if len(seed.Rows) == 0 {
				failures.add(display, "%s", i18n.T("sed_empty"))
			}
			continue
		}
		if len(seed.Rows) == 0 {
			failures.add(display, "%s", i18n.T("sed_empty"))
			continue
		}
		if fixos[seed.Table] == nil {
			fixos[seed.Table] = map[string]bool{}
		}

		for index, row := range seed.Rows {
			informou := true
			for _, key := range seed.Keys {
				if _, ok := row[key]; !ok {
					informou = false
					break
				}
			}

			if seed.First && !informou {
				failures.add(display, i18n.T("sed_row_without_id"),
					index+1, strings.Join(seed.Keys, ", "))
				continue
			}
			if !informou {
				// Sem chave: é inserção com ID gerado pelo banco. Permitido,
				// mas a linha vira folha — não se edita nem se referencia.
				continue
			}
			assinatura := seedKeyLabel(seed.Keys, row)
			if seed.First || !fixos[seed.Table][assinatura] {
				// ID explícito inédito: está declarando um novo ID fixo.
				fixos[seed.Table][assinatura] = true
				continue
			}
			// ID explícito já conhecido: é edição, e é permitida.
		}
	}
}

func ensureSeedHistory(ctx context.Context, db *sql.DB, dialect, schema, table string) error {
	target := qualified(dialect, schema, table)
	var query string
	switch dialect {
	case "postgres":
		query = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY, seeder VARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, rows_applied INTEGER NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`, target)
	case "mysql":
		query = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGINT AUTO_INCREMENT PRIMARY KEY, seeder VARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, rows_applied INT NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`, target)
	case "sqlserver":
		query = fmt.Sprintf(`IF OBJECT_ID(N'%s', N'U') IS NULL CREATE TABLE %s (
			id BIGINT IDENTITY(1,1) PRIMARY KEY, seeder NVARCHAR(255) NOT NULL UNIQUE,
			checksum VARCHAR(64) NOT NULL, rows_applied INT NOT NULL,
			applied_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME())`,
			objectName(schema, table), target)
	default:
		query = fmt.Sprintf(`BEGIN EXECUTE IMMEDIATE 'CREATE TABLE %s (
			id NUMBER(19) GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			seeder VARCHAR2(255) NOT NULL UNIQUE, checksum VARCHAR2(64) NOT NULL,
			rows_applied NUMBER(10) NOT NULL, applied_at TIMESTAMP DEFAULT SYSTIMESTAMP NOT NULL)';
			EXCEPTION WHEN OTHERS THEN IF SQLCODE != -955 THEN RAISE; END IF; END;`, target)
	}
	_, err := db.ExecContext(ctx, query)
	return err
}

func loadSeedHistory(ctx context.Context, db *sql.DB, dialect, schema, table string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT seeder, checksum FROM %s", qualified(dialect, schema, table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := map[string]string{}
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			return nil, err
		}
		history[name] = checksum
	}
	return history, rows.Err()
}

func seedHistoryTable(state config.ConfigState) string {
	if name := state.Config.Seed.Table; name != "" {
		return name
	}
	return "seeders_gokit"
}

// SeedRun aplica os seeders pendentes. Com onlyFirst, aplica apenas o seed
// inicial de cada tabela — é o que o `migrate run` chama logo após as
// migrations, para que uma instalação nova fique pronta em um comando só.
func SeedRun(root string, state config.ConfigState, onlyFirst bool) error {
	seeds, err := loadSeeds(root, state)
	if err != nil {
		return describeLoadError(err)
	}
	if onlyFirst {
		var initial []seedFile
		for _, seed := range seeds {
			if seed.First {
				initial = append(initial, seed)
			}
		}
		seeds = initial
	}
	if len(seeds) == 0 {
		if !onlyFirst {
			fmt.Println(cliui.Muted(i18n.T("sed_none_found")))
		}
		return nil
	}

	dialect := state.ActiveDialect
	connection := state.Config.Connections[state.ActiveClient]
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	if driver == "" {
		return i18n.Errf("run_dialect_unsupported", dialect)
	}
	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	historyTable := seedHistoryTable(state)
	if err := ensureSeedHistory(ctx, db, dialect, connection.Schema, historyTable); err != nil {
		return i18n.Errf("sed_create_failed", historyTable, err)
	}
	history, err := loadSeedHistory(ctx, db, dialect, connection.Schema, historyTable)
	if err != nil {
		return err
	}

	applied, skipped := 0, 0
	for _, seed := range seeds {
		if checksum, exists := history[seed.ID]; exists {
			if checksum != seed.Checksum {
				return cliui.NewUserError(
					i18n.Tf("sed_changed", seed.ID),
					i18n.Tf("sed_changed_fix", seed.Table),
				)
			}
			skipped++
			continue
		}
		operation := acao.Operacao{
			Kind: string(acao.SeedRows), Table: seed.Table, Rows: seed.Rows,
			KeyColumns: seed.Keys, IdentityColumn: seed.Identity, Numericas: seed.Numericas,
		}
		if err := executeSeedRows(ctx, db, dialect, connection.Schema, operation); err != nil {
			return fmt.Errorf("%s: %w", seed.ID, err)
		}
		if err := insertSeedHistory(ctx, db, dialect, connection.Schema, historyTable, seed); err != nil {
			return i18n.Errf("sed_register_failed", seed.ID, err)
		}
		applied++
	}
	// Rastro da soberania do seeder. Sai ANTES do resumo, porque é o que o usuário precisa
	// olhar: o resumo diz que deu certo, estas listas dizem o que foi perdido no caminho.
	if sobrescritas := SeedSobrescritas(); len(sobrescritas) > 0 {
		fmt.Println(cliui.Warning(i18n.Tf("run_seed_overwritten", len(sobrescritas))))
		for _, linha := range sobrescritas {
			fmt.Printf("  · %s\n", linha)
		}
		aviso.DoEstado(state).Notificar(aviso.Aviso{
			Nivel: aviso.NivelWarning,
			Acao:  "linha sobrescrita pelo seeder",
			Corpo: fmt.Sprintf("%d linha(s) que já existiam no banco com outro conteúdo foram substituídas "+
				"pelo seeder. O seeder é soberano sobre os dados, então dado da aplicação naquelas "+
				"linhas foi perdido.", len(sobrescritas)),
			Contexto: map[string]string{"comando": "seed run"},
			Bruto:    strings.Join(sobrescritas, "\n"),
		})
	}
	if substituidas := SeedSubstituicoes(); len(substituidas) > 0 {
		fmt.Println(cliui.Warning(i18n.Tf("run_seed_replaced_group", len(substituidas))))
		for _, linha := range substituidas {
			fmt.Printf("  · %s\n", linha)
		}
		aviso.DoEstado(state).Notificar(aviso.Aviso{
			Nivel: aviso.NivelWarning,
			Acao:  "conteúdo de tabela substituído pelo seeder",
			Corpo: fmt.Sprintf("%d tabela(s) sem chave primária tiveram o conteúdo apagado e reinserido pelo "+
				"seeder. Sem chave não há como casar linha declarada com existente, então o seeder "+
				"é dono do conteúdo.", len(substituidas)),
			Contexto: map[string]string{"comando": "seed run"},
			Bruto:    strings.Join(substituidas, "\n"),
		})
	}

	if applied > 0 || !onlyFirst {
		fmt.Printf(i18n.T("sed_applied_summary"), cliui.Success("✓ OK"), applied, skipped)
	}
	return nil
}

func insertSeedHistory(ctx context.Context, db *sql.DB, dialect, schema, table string, seed seedFile) error {
	target := qualified(dialect, schema, table)
	query := map[string]string{
		"postgres":  fmt.Sprintf("INSERT INTO %s (seeder, checksum, rows_applied) VALUES ($1, $2, $3)", target),
		"oracle":    fmt.Sprintf("INSERT INTO %s (seeder, checksum, rows_applied) VALUES (:1, :2, :3)", target),
		"mysql":     fmt.Sprintf("INSERT INTO %s (seeder, checksum, rows_applied) VALUES (?, ?, ?)", target),
		"sqlserver": fmt.Sprintf("INSERT INTO %s (seeder, checksum, rows_applied) VALUES (@p1, @p2, @p3)", target),
	}[dialect]
	_, err := db.ExecContext(ctx, query, seed.ID, seed.Checksum, len(seed.Rows))
	return err
}

// CreateSeedFile cria database/seeds/<tabela>/<timestamp>_seeder.go.
//
// Se for o primeiro seeder da tabela e ela já tiver dados no banco, o arquivo
// nasce com o retrato atual. Caso contrário nasce como esqueleto, com as
// colunas certas para preencher.
func CreateSeedFile(root string, state config.ConfigState, target string) (string, int, error) {
	shapes, err := tableShapes(root, state)
	if err != nil {
		return "", 0, err
	}
	shape, known := shapes[strings.ToLower(target)]
	if !known {
		// Também aceita o alias, que é como as migrations referenciam a tabela.
		for _, candidate := range shapes {
			if strings.EqualFold(candidate.AliasName, target) {
				shape, known = candidate, true
				break
			}
		}
	}
	if !known {
		return "", 0, cliui.NewUserError(
			i18n.Tf("sed_no_createtable", target),
			i18n.T("sed_no_createtable_fix"),
		)
	}

	var columns []string
	hasKey := false
	for _, column := range shape.Columns {
		columns = append(columns, column.Name)
		if column.PrimaryKey {
			hasKey = true
		}
	}
	// Tabela sem chave primária É aceita: o seeder é soberano sobre os dados e substitui o
	// conteúdo inteiro. Antes isto era recusado — sem chave não há como distinguir inserir de
	// editar —, e a recusa deixava 306 das 754 tabelas do corpus legado fora do seeder.
	//
	// O aviso sai na criação, e não só na execução: quem gera o arquivo precisa saber que
	// aquele seeder vai APAGAR o conteúdo da tabela, não somá-lo.
	if !hasKey {
		fmt.Println(cliui.Warning(i18n.Tf("sed_no_pk_sovereign", shape.Table)))
	}

	folder := filepath.Join(seedRoot(root, state), shape.Table)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", 0, err
	}
	// A pasta acabou de ser criada pelo MkdirAll acima, então falha em lê-la é problema
	// de permissão — e não pode ser descartada: sem a lista, `primeiro` fica true e o
	// seeder novo nasce se declarando o primeiro da tabela, com o cabeçalho e os IDs
	// fixos de um seeder inicial, em cima de outros que já existem.
	existentes, err := os.ReadDir(folder)
	if err != nil {
		return "", 0, err
	}
	primeiro := true
	for _, entry := range existentes {
		if seedFileName.MatchString(entry.Name()) {
			primeiro = false
			break
		}
	}

	// Só o seed inicial nasce com o retrato do banco: um seeder de atualização
	// deve conter apenas o que muda, não a tabela inteira de novo.
	var literals []string
	if primeiro {
		literals, err = snapshotRows(state, shape, columns)
		if err != nil {
			return "", 0, err
		}
	}
	if len(literals) == 0 {
		comentario := i18n.T("sed_gen_skeleton")
		if !primeiro {
			comentario = i18n.T("sed_gen_update_note")
		}
		literals = []string{
			"\t\t" + comentario,
			"\t\t{" + strings.Join(seedPlaceholders(shape.Columns), ", ") + "},",
		}
	}

	stamp := time.Now().Format("2006_01_02_150405")
	name := stamp + "_seeder.go"
	if !primeiro {
		name = stamp + "_atualiza_" + shape.Table + ".go"
	}
	path := filepath.Join(folder, name)

	titulo := i18n.Tf("sed_gen_title_initial", shape.Table)
	if !primeiro {
		titulo = i18n.Tf("sed_gen_title_update", shape.Table)
	}
	declarationName := dynamicDeclarationName("Seeder", name)
	content := fmt.Sprintf(`package seeds

import (
	migrate "github.com/PhelipeViana/gokit/migration"
)

// %s
func %s() migrate.Rows {
	return migrate.Rows{
%s
	}
}
`, titulo, declarationName, strings.Join(literals, "\n"))

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", 0, err
	}
	if err := recordGeneratedFile(root, shape.Table, "seeder", path); err != nil {
		return "", 0, i18n.Errf("sed_gen_register_failed", err)
	}
	relative, _ := filepath.Rel(root, path)
	if primeiro && len(literals) > 0 && !strings.Contains(literals[0], "//") {
		return filepath.ToSlash(relative), len(literals), nil
	}
	return filepath.ToSlash(relative), 0, nil
}

// snapshotRows lê o conteúdo atual da tabela na conexão ativa. Tabela
// inexistente ou vazia devolve nada, e quem chama gera o esqueleto.
func snapshotRows(state config.ConfigState, shape acao.Operacao, columns []string) ([]string, error) {
	dialect := state.ActiveDialect
	connection := state.Config.Connections[state.ActiveClient]
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	if driver == "" {
		return nil, i18n.Errf("run_dialect_unsupported", dialect)
	}
	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	existe, err := tableExists(ctx, db, dialect, connection.Schema, shape.Table)
	if err != nil || !existe {
		return nil, nil
	}

	var keys []string
	for _, column := range shape.Columns {
		if column.PrimaryKey {
			keys = append(keys, column.Name)
		}
	}
	order := ""
	if len(keys) > 0 {
		order = " ORDER BY " + strings.Join(quotedColumns(dialect, keys), ", ")
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s%s",
		strings.Join(quotedColumns(dialect, columns), ", "),
		qualified(dialect, connection.Schema, shape.Table), order))
	if err != nil {
		return nil, fmt.Errorf("ler %s: %w", shape.Table, err)
	}
	defer rows.Close()

	var literals []string
	for rows.Next() {
		values := make([]any, len(columns))
		scan := make([]any, len(columns))
		for i := range values {
			scan[i] = &values[i]
		}
		if err := rows.Scan(scan...); err != nil {
			return nil, err
		}
		parts := make([]string, 0, len(columns))
		for i, column := range columns {
			parts = append(parts, fmt.Sprintf("%q: %s", column, seedGoLiteral(values[i], shape.Columns[i])))
		}
		literals = append(literals, "\t\t{"+strings.Join(parts, ", ")+"},")
	}
	return literals, rows.Err()
}

// SeedValidate confere todos os seeders sem tocar no banco.
func SeedValidate(root string, state config.ConfigState) error {
	seeds, err := loadSeeds(root, state)
	var loadErr *LoadError
	if errors.As(err, &loadErr) {
		fmt.Printf(i18n.T("sed_problems"), cliui.Failure("✗"), len(loadErr.Issues))
		for _, issue := range loadErr.Issues {
			fmt.Printf("  %s %s\n      %s\n", cliui.Failure("•"), issue.File, issue.Detail)
		}
		return cliui.NewUserError(i18n.T("sed_invalid"), i18n.T("sed_invalid_fix"))
	}
	if err != nil {
		return err
	}
	if len(seeds) == 0 {
		fmt.Println(cliui.Muted(i18n.T("sed_none_found")))
		return nil
	}
	rows := 0
	for _, seed := range seeds {
		rows += len(seed.Rows)
	}
	fmt.Printf(i18n.T("sed_validate_summary"), cliui.Success("✓ OK"), len(seeds), rows)
	for _, seed := range seeds {
		marca := "atualização"
		if seed.First {
			marca = "inicial"
		}
		fmt.Printf(i18n.T("sed_validate_line"), cliui.Muted("·"), seed.ID, len(seed.Rows), marca)
	}
	return nil
}
