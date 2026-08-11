package migraterun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
)

// ReloadStep represents a single task in the reload pipeline
type ReloadStep struct {
	Name        string
	Description string
	Status      string // "pending", "running", "success", "failed"
	Error       error
	Message     string
	Suggestion  string
}

// ReloadPipeline holds the state of the three reload groups
type ReloadPipeline struct {
	Group1 []ReloadStep
	Group2 []ReloadStep
	Group3 []ReloadStep
}

// RunReload CLI execution reporter
func RunReload(state config.ConfigState) error {
	pipeline := InitReloadPipeline()

	fmt.Println(i18n.T("rel_group_env"))
	for i := range pipeline.Group1 {
		step := &pipeline.Group1[i]
		fmt.Printf("  → %s... ", step.Description)
		err := executeStep(step, state)
		if err != nil {
			fmt.Printf("\033[31m✗ FALHA\033[0m\n")
			fmt.Printf(i18n.T("rel_error_line"), step.Message)
			if step.Suggestion != "" {
				fmt.Printf(i18n.T("rel_hint_line"), step.Suggestion)
			}
			return i18n.Errf("rel_aborted_g1", err)
		}
		fmt.Printf("\033[32m✓ OK\033[0m\n")
	}

	fmt.Println(i18n.T("rel_group_exec"))
	for i := range pipeline.Group2 {
		step := &pipeline.Group2[i]
		fmt.Printf("  → %s... ", step.Description)
		err := executeStep(step, state)
		if err != nil {
			fmt.Printf("\033[31m✗ FALHA\033[0m\n")
			fmt.Printf(i18n.T("rel_error_line"), step.Message)
			if step.Suggestion != "" {
				fmt.Printf(i18n.T("rel_hint_line"), step.Suggestion)
			}
			return i18n.Errf("rel_aborted_g2", err)
		}
		fmt.Printf("\033[32m✓ OK\033[0m (%s)\n", step.Message)
	}

	fmt.Println(i18n.T("rel_group_gen"))
	for i := range pipeline.Group3 {
		step := &pipeline.Group3[i]
		fmt.Printf("  → %s... ", step.Description)
		err := executeStep(step, state)
		if err != nil {
			fmt.Printf("\033[31m✗ FALHA\033[0m\n")
			fmt.Printf(i18n.T("rel_error_line"), step.Message)
			if step.Suggestion != "" {
				fmt.Printf(i18n.T("rel_hint_line"), step.Suggestion)
			}
			return i18n.Errf("rel_aborted_g3", err)
		}
		fmt.Printf("\033[32m✓ OK\033[0m\n")
	}

	fmt.Println(i18n.T("rel_success"))
	fmt.Println()
	return nil
}

// InitReloadPipeline initializes the steps of the pipeline
func InitReloadPipeline() ReloadPipeline {
	return ReloadPipeline{
		Group1: []ReloadStep{
			{Name: "check_docker", Description: i18n.T("rel_step_docker")},
			{Name: "check_go", Description: i18n.T("rel_step_toolchain")},
			{Name: "check_config", Description: i18n.T("rel_step_config")},
			{Name: "align_directories", Description: i18n.T("rel_step_dirs")},
			{Name: "normalize_declarations", Description: i18n.T("rel_step_names")},
			{Name: "go_tidy", Description: i18n.T("rel_step_tidy")},
			{Name: "start_database", Description: i18n.T("rel_step_compose")},
			{Name: "check_conn", Description: i18n.T("rel_step_ping")},
		},
		Group2: []ReloadStep{
			{Name: "run_migrations", Description: i18n.T("rel_step_migrations")},
			{Name: "run_seeds", Description: i18n.T("rel_step_seeders")},
			{Name: "run_factories", Description: i18n.T("rel_step_factories")},
		},
		Group3: []ReloadStep{
			{Name: "generate_orm", Description: i18n.T("rel_step_orm")},
			{Name: "refresh_catalog", Description: i18n.T("rel_step_dsl")},
			{Name: "generate_docs", Description: i18n.T("rel_step_docs")},
			{Name: "generate_editors", Description: i18n.T("rel_step_editors")},
		},
	}
}

func executeStep(step *ReloadStep, state config.ConfigState) error {
	step.Status = "running"
	var err error

	switch step.Name {
	case "check_go":
		err = checkGoInstallation(step, state)
	case "check_docker":
		err = checkDockerState(step)
	case "check_config":
		err = checkConfigStructure(step, state)
	case "check_conn":
		err = checkConnection(step, state)
	case "start_database":
		err = startActiveDatabase(step, state)
	case "go_tidy":
		err = runGoModTidy(step, state)
	case "run_migrations":
		err = executePendingMigrations(step, state)
	case "run_seeds":
		err = executePendingSeeds(step, state)
	case "run_factories":
		err = executeFactories(step, state)
	case "refresh_catalog":
		err = refreshCoreCatalog(step, state)
	case "generate_orm":
		err = generateORM(step, state)
	case "generate_docs":
		err = generateDocumentation(step, state)
	case "generate_editors":
		err = generateEditorSettings(step)
	case "align_directories":
		err = alignDirectories(step, state)
	case "normalize_declarations":
		var changed int
		changed, err = normalizeLegacyDeclarations(".", state)
		step.Message = i18n.Tf("rel_names_normalized", changed)
	}

	if err != nil {
		step.Status = "failed"
		step.Error = err
		return err
	}

	step.Status = "success"
	return nil
}

// ==========================================
// GRUPO 1: FUNCIONAMENTO
// ==========================================

func goCommand(state config.ConfigState, args ...string) *exec.Cmd {
	if state.Config != nil && strings.EqualFold(strings.TrimSpace(state.Config.Go.Execution), "docker") {
		service := strings.TrimSpace(state.Config.Go.DockerService)
		if service == "" {
			service = "toolchain"
		}
		dockerArgs := []string{"compose", "run", "--rm", service, "go"}
		dockerArgs = append(dockerArgs, args...)
		return exec.Command("docker", dockerArgs...)
	}
	return exec.Command("go", args...)
}

func checkGoInstallation(step *ReloadStep, state config.ConfigState) error {
	cmd := goCommand(state, "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		step.Message = i18n.Tf("rel_toolchain_failed", strings.TrimSpace(string(out)))
		step.Suggestion = i18n.T("rel_toolchain_failed_fix")
		return err
	}
	step.Message = strings.TrimSpace(string(out))
	return nil
}

func checkConfigStructure(step *ReloadStep, state config.ConfigState) error {
	if state.ConfigFileError != nil {
		step.Message = i18n.Tf("rel_config_invalid", state.ConfigFileError)
		step.Suggestion = i18n.T("rel_config_invalid_fix")
		return state.ConfigFileError
	}
	if state.Config == nil {
		step.Message = i18n.T("rel_config_empty")
		step.Suggestion = i18n.T("rel_config_empty_fix")
		return errors.New("config nula")
	}
	step.Message = i18n.T("rel_config_ok")
	return nil
}

func startActiveDatabase(step *ReloadStep, state config.ConfigState) error {
	if state.Config == nil {
		return errors.New("config nula")
	}
	if state.Config.Go.DockerAutoStart != nil && !*state.Config.Go.DockerAutoStart {
		step.Message = i18n.T("rel_autostart_off")
		return nil
	}
	if err := config.TestDatabaseConnection(state.ActiveDialect, state.ActiveURL); err == nil {
		step.Message = i18n.T("rel_db_already_up")
		return nil
	}

	composeFile := ""
	for _, candidate := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		if _, err := os.Stat(candidate); err == nil {
			composeFile = candidate
			break
		}
	}
	if composeFile == "" {
		step.Message = i18n.T("rel_compose_missing")
		step.Suggestion = i18n.T("rel_compose_missing_fix")
		return errors.New(i18n.T("rel_compose_missing_err"))
	}

	serviceByDialect := map[string]string{
		"mysql":      "mysql",
		"postgres":   "postgres",
		"postgresql": "postgres",
		"oracle":     "oracle",
		"sqlserver":  "mssql",
		"mssql":      "mssql",
	}
	service := serviceByDialect[strings.ToLower(strings.TrimSpace(state.ActiveDialect))]
	if service == "" {
		step.Message = i18n.Tf("rel_no_service_for_dialect", state.ActiveDialect)
		return i18n.Errf("rel_no_service_for_dialect_err", state.ActiveDialect)
	}

	servicesCmd := exec.Command("docker", "compose", "-f", composeFile, "config", "--services")
	servicesOut, err := servicesCmd.CombinedOutput()
	if err != nil || !containsComposeService(string(servicesOut), service) {
		step.Message = i18n.Tf("rel_service_inactive", service, composeFile)
		step.Suggestion = i18n.Tf("rel_service_inactive_fix", service, state.ActiveDialect)
		if err != nil {
			return err
		}
		return i18n.Errf("rel_service_inactive_err", service)
	}

	upCmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d", service)
	if out, err := upCmd.CombinedOutput(); err != nil {
		step.Message = i18n.Tf("rel_service_start_failed", service, strings.TrimSpace(string(out)))
		step.Suggestion = i18n.Tf("rel_service_start_failed_fix", service)
		return err
	}

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = config.TestDatabaseConnection(state.ActiveDialect, state.ActiveURL)
		if lastErr == nil {
			step.Message = i18n.Tf("rel_service_ready", service)
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	step.Message = i18n.Tf("rel_service_not_ready", service, lastErr)
	step.Suggestion = i18n.Tf("rel_service_not_ready_fix", service)
	return lastErr
}

func containsComposeService(output, expected string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == expected {
			return true
		}
	}
	return false
}

func checkConnection(step *ReloadStep, state config.ConfigState) error {
	if state.ActiveDialect == "" || state.ActiveURL == "" {
		step.Message = i18n.T("rel_no_active_conn")
		step.Suggestion = i18n.T("rel_no_active_conn_fix")
		return errors.New("sem conexão")
	}
	err := config.TestDatabaseConnection(state.ActiveDialect, state.ActiveURL)
	if err != nil {
		step.Message = i18n.Tf("rel_conn_failed", state.ActiveDialect, err)
		step.Suggestion = i18n.T("rel_conn_failed_fix")
		return err
	}
	step.Message = i18n.T("rel_ping_ok")
	return nil
}

func checkDockerState(step *ReloadStep) error {
	// 1. Checa se o CLI do Docker está instalado no sistema
	if _, err := exec.LookPath("docker"); err != nil {
		step.Message = i18n.T("rel_docker_cli_missing")
		step.Suggestion = i18n.T("rel_docker_cli_missing_fix")
		return err
	}

	// 2. Checa se o Daemon do Docker está rodando ativamente
	infoCmd := exec.Command("docker", "info")
	infoCmd.Stderr = io.Discard
	if err := infoCmd.Run(); err == nil {
		step.Message = i18n.T("rel_docker_ok")
		return nil
	}

	// 3. Auto-Healing: tenta iniciar a engine do Docker automaticamente se inativa
	var started bool
	if runtime.GOOS == "darwin" {
		if _, colimaErr := exec.LookPath("colima"); colimaErr == nil {
			_ = exec.Command("colima", "start").Run()
			started = true
		} else {
			_ = exec.Command("open", "-a", "Docker").Run()
			started = true
		}
	} else if runtime.GOOS == "linux" {
		_ = exec.Command("sudo", "systemctl", "start", "docker").Run()
		started = true
	}

	if started {
		time.Sleep(3 * time.Second)
		infoRetry := exec.Command("docker", "info")
		infoRetry.Stderr = io.Discard
		if err := infoRetry.Run(); err == nil {
			step.Message = i18n.T("rel_docker_autostarted")
			return nil
		}
	}

	step.Message = i18n.T("rel_docker_offline")
	step.Suggestion = i18n.T("rel_docker_offline_fix")
	return errors.New(i18n.T("rel_docker_offline_err"))
}

func runGoModTidy(step *ReloadStep, state config.ConfigState) error {
	goConfig := config.GoConfig{}
	if state.Config != nil {
		goConfig = state.Config.Go
	}
	// AplicarModo em vez de montar as opções aqui: a política de go.mod e
	// go.work é do modo declarado, e ter uma segunda montagem neste arquivo foi
	// justamente o que deixou o replace entrar sem ninguém pedir.
	result, err := config.AplicarModo(".", state.Config)
	if err != nil {
		step.Message = i18n.Tf("rel_gomod_failed", err)
		step.Suggestion = i18n.T("rel_gomod_failed_fix")
		return err
	}

	cmd := goCommand(state, "mod", "tidy")
	out, err := cmd.CombinedOutput()
	if err != nil {
		step.Message = i18n.Tf("rel_tidy_failed", strings.TrimSpace(string(out)))
		step.Suggestion = i18n.T("rel_tidy_failed_fix")
		return err
	}
	execucao := "host"
	if strings.EqualFold(goConfig.Execution, "docker") {
		execucao = "docker"
	}
	// O modo entra na linha de resultado de propósito: é a informação que diz se
	// este projeto está compilando contra o gokit local ou contra a versão
	// publicada, e é o que o usuário precisa ver sem abrir arquivo nenhum.
	step.Message = i18n.Tf("rel_module_aligned_mode", result.Module, result.GoKitModule, result.Mode, execucao)
	return nil
}

// ==========================================
// GRUPO 2: MÉTODO
// ==========================================

func executePendingMigrations(step *ReloadStep, state config.ConfigState) error {
	folder := filepath.Join(".", filepath.FromSlash(state.Config.Output.Migrate))
	files, err := loadPlans(folder)
	if err != nil {
		step.Message = i18n.Tf("rel_load_migrations_failed", err)
		return err
	}

	db, err := sql.Open(getDriverName(state.ActiveDialect), state.ActiveURL)
	if err != nil {
		step.Message = i18n.Tf("rel_open_db_failed", err)
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	historyTable := state.Config.Migrate.Table
	if historyTable == "" {
		historyTable = "migrations_gokit"
	}

	var pendingCount int
	exists, err := tableExists(ctx, db, state.ActiveDialect, state.Config.Connections[state.ActiveClient].Schema, historyTable)
	if err == nil && exists {
		history, _, err := loadHistory(ctx, db, state.ActiveDialect, state.Config.Connections[state.ActiveClient].Schema, historyTable)
		if err == nil {
			for _, file := range files {
				_, existsID := history[file.ID]
				_, existsName := history[file.Name]
				if !existsID && !existsName {
					pendingCount++
				}
			}
		} else {
			pendingCount = len(files)
		}
	} else {
		pendingCount = len(files)
	}

	if pendingCount > 0 {
		// Roda as migrations automaticamente
		err = Run(".", state)
		if err != nil {
			step.Message = i18n.Tf("rel_migrations_pending", pendingCount)
			return err
		}
		step.Message = i18n.Tf("rel_applied_n", pendingCount)
	} else {
		step.Message = i18n.T("rel_none_pending_f")
	}
	return nil
}

func executePendingSeeds(step *ReloadStep, state config.ConfigState) error {
	// Checa se há seeders locais
	tables, err := SeedableTables(".", state)
	if err != nil {
		step.Message = i18n.T("rel_no_seeder_pending")
		return nil
	}

	db, err := sql.Open(getDriverName(state.ActiveDialect), state.ActiveURL)
	if err != nil {
		return nil
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	seedTable := state.Config.Seed.Table
	if seedTable == "" {
		seedTable = "seeders_gokit"
	}

	var pendingCount int
	exists, err := tableExists(ctx, db, state.ActiveDialect, state.Config.Connections[state.ActiveClient].Schema, seedTable)
	if err == nil && exists {
		// Executa validação de seeds pendentes
		for _, table := range tables {
			query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE table_name = ?", seedTable)
			if state.ActiveDialect == "postgres" || state.ActiveDialect == "postgresql" {
				query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE table_name = $1", seedTable)
			} else if state.ActiveDialect == "oracle" {
				query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE table_name = :1", seedTable)
			} else if state.ActiveDialect == "sqlserver" {
				query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE table_name = @p1", seedTable)
			}
			var count int
			err = db.QueryRowContext(ctx, query, table).Scan(&count)
			if err != nil || count == 0 {
				pendingCount++
			}
		}
	} else {
		pendingCount = len(tables)
	}

	if pendingCount > 0 {
		err = SeedRun(".", state, false)
		if err != nil {
			step.Message = i18n.Tf("rel_seeders_failed", err)
			return err
		}
		step.Message = i18n.Tf("rel_applied_n_m", pendingCount)
	} else {
		step.Message = i18n.T("rel_none_pending_m")
	}
	return nil
}

func executeFactories(step *ReloadStep, state config.ConfigState) error {
	tables, err := FactoryTables(".", state)
	if err != nil || len(tables) == 0 {
		step.Message = i18n.T("rel_none_active")
		return nil
	}
	// Roda as factories (cria 10 linhas em cada tabela configurada)
	err = FactoryRun(".", state, nil)
	if err != nil {
		step.Message = i18n.Tf("rel_factories_failed", err)
		return err
	}
	step.Message = i18n.Tf("rel_tables_populated", len(tables))
	return nil
}

// ==========================================
// GRUPO 3: METADADOS
// ==========================================

func generateORM(step *ReloadStep, state config.ConfigState) error {
	count, err := GenerateORM(".", state)
	if err != nil {
		step.Message = i18n.Tf("rel_orm_failed", err)
		return err
	}
	step.Message = i18n.Tf("rel_orm_done", count)
	return nil
}

func refreshCoreCatalog(step *ReloadStep, state config.ConfigState) error {
	folder := filepath.Join(".", filepath.FromSlash(state.Config.Output.Migrate))
	err := migrationgo.RefreshCatalog(".", folder)
	if err != nil {
		step.Message = i18n.Tf("rel_catalog_failed", err)
		return err
	}
	step.Message = i18n.T("rel_catalog_done")
	return nil
}

func generateDocumentation(step *ReloadStep, state config.ConfigState) error {
	if err := GenerateDocumentation(".", state); err != nil {
		step.Message = i18n.Tf("rel_docs_failed", err)
		return err
	}
	step.Message = i18n.T("rel_docs_done")
	return nil
}

// generateEditorSettings mantém a configuração de editor em dia sem passar por
// cima de escolha do usuário: arquivo existente fica como está, e o
// settings.json só recebe as chaves que faltam.
func generateEditorSettings(step *ReloadStep) error {
	escritos, aviso, err := config.EscreverConfigEditores(".")
	if err != nil {
		step.Message = err.Error()
		return err
	}
	step.Suggestion = aviso
	if len(escritos) == 0 {
		step.Message = i18n.T("edt_already_current")
		return nil
	}
	step.Message = i18n.Tf("edt_written", len(escritos))
	return nil
}

func alignDirectories(step *ReloadStep, state config.ConfigState) error {
	outputs := map[string]string{
		"migrate": state.Config.Output.Migrate,
		"factory": state.Config.Output.Factory,
		"seed":    state.Config.Output.Seed,
		"docs":    state.Config.Output.Docs,
		"orm":     state.Config.Output.ORM,
	}

	// Mapeia possíveis caminhos legados/alternativos para cada alvo
	legacyCandidates := map[string][]string{
		filepath.FromSlash("internal/gokit/migrate"): {
			filepath.FromSlash("database/migrations"),
			filepath.FromSlash("internal/database/migrations"),
		},
		filepath.FromSlash("internal/gokit/factory"): {
			filepath.FromSlash("database/factories"),
			filepath.FromSlash("internal/database/factories"),
		},
		filepath.FromSlash("internal/gokit/seed"): {
			filepath.FromSlash("database/seeds"),
			filepath.FromSlash("internal/database/seeds"),
		},
		filepath.FromSlash("internal/gokit/docs"): {
			filepath.FromSlash("docs"),
			filepath.FromSlash("internal/docs"),
		},
		filepath.FromSlash("database/migrations"): {
			filepath.FromSlash("internal/gokit/migrate"),
			filepath.FromSlash("internal/database/migrations"),
		},
		filepath.FromSlash("database/factories"): {
			filepath.FromSlash("internal/gokit/factory"),
			filepath.FromSlash("internal/database/factories"),
		},
		filepath.FromSlash("database/seeds"): {
			filepath.FromSlash("internal/gokit/seed"),
			filepath.FromSlash("internal/database/seeds"),
		},
		filepath.FromSlash("docs"): {
			filepath.FromSlash("internal/gokit/docs"),
			filepath.FromSlash("internal/docs"),
		},
	}

	migratedCount := 0
	for _, configuredPath := range outputs {
		if configuredPath == "" {
			continue
		}
		configuredPath = filepath.FromSlash(configuredPath)

		candidates, hasCandidates := legacyCandidates[configuredPath]
		if !hasCandidates {
			continue
		}

		for _, alternatePath := range candidates {
			// Se o diretório alternativo existe e tem arquivos, e o configurado está ausente ou vazio, movemos os arquivos!
			if dirExistsAndNotEmpty(alternatePath) && !dirExistsAndNotEmpty(configuredPath) {
				err := moveFiles(alternatePath, configuredPath)
				if err != nil {
					step.Message = i18n.Tf("rel_dir_move_failed", alternatePath, configuredPath, err)
					return err
				}
				migratedCount++
			}
		}
	}

	if migratedCount > 0 {
		step.Message = i18n.Tf("rel_dirs_aligned_n", migratedCount)
	} else {
		step.Message = i18n.T("rel_dirs_already")
	}
	return nil
}

func dirExistsAndNotEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

func moveFiles(src, dst string) error {
	err := os.MkdirAll(dst, 0o755)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = moveFiles(srcPath, dstPath)
			if err != nil {
				return err
			}
		} else {
			// Move o arquivo (Remove primeiro se for read-only)
			_ = os.Remove(dstPath)
			err = os.Rename(srcPath, dstPath)
			if err != nil {
				// Fallback de cópia caso seja partição diferente
				err = copyFile(srcPath, dstPath)
				if err != nil {
					return err
				}
				_ = os.Remove(srcPath)
			}
		}
	}

	// Remove pasta de origem se vazia
	_ = os.Remove(src)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
