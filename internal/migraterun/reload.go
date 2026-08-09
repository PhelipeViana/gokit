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
	"github.com/PhelipeViana/gokit/internal/gomodule"
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

	fmt.Println("\nAmbiente & infraestrutura")
	for i := range pipeline.Group1 {
		step := &pipeline.Group1[i]
		fmt.Printf("  → %s... ", step.Description)
		err := executeStep(step, state)
		if err != nil {
			fmt.Printf("\033[31m✗ FALHA\033[0m\n")
			fmt.Printf("\n  \033[31mErro:\033[0m %s\n", step.Message)
			if step.Suggestion != "" {
				fmt.Printf("  \033[36mSugestão:\033[0m %s\n\n", step.Suggestion)
			}
			return fmt.Errorf("reload abortado no Grupo 1: %w", err)
		}
		fmt.Printf("\033[32m✓ OK\033[0m\n")
	}

	fmt.Println("\nOperações & execução")
	for i := range pipeline.Group2 {
		step := &pipeline.Group2[i]
		fmt.Printf("  → %s... ", step.Description)
		err := executeStep(step, state)
		if err != nil {
			fmt.Printf("\033[31m✗ FALHA\033[0m\n")
			fmt.Printf("\n  \033[31mErro:\033[0m %s\n", step.Message)
			if step.Suggestion != "" {
				fmt.Printf("  \033[36mSugestão:\033[0m %s\n\n", step.Suggestion)
			}
			return fmt.Errorf("reload abortado no Grupo 2: %w", err)
		}
		fmt.Printf("\033[32m✓ OK\033[0m (%s)\n", step.Message)
	}

	fmt.Println("\nGeração & catálogos")
	for i := range pipeline.Group3 {
		step := &pipeline.Group3[i]
		fmt.Printf("  → %s... ", step.Description)
		err := executeStep(step, state)
		if err != nil {
			fmt.Printf("\033[31m✗ FALHA\033[0m\n")
			fmt.Printf("\n  \033[31mErro:\033[0m %s\n", step.Message)
			if step.Suggestion != "" {
				fmt.Printf("  \033[36mSugestão:\033[0m %s\n\n", step.Suggestion)
			}
			return fmt.Errorf("reload abortado no Grupo 3: %w", err)
		}
		fmt.Printf("\033[32m✓ OK\033[0m\n")
	}

	fmt.Println("\n\033[32m✔ Projeto redefinido e alinhado com sucesso!\033[0m")
	fmt.Println()
	return nil
}

// InitReloadPipeline initializes the steps of the pipeline
func InitReloadPipeline() ReloadPipeline {
	return ReloadPipeline{
		Group1: []ReloadStep{
			{Name: "check_docker", Description: "Verificar estado e daemon do Docker"},
			{Name: "check_go", Description: "Verificar o toolchain Golang configurado"},
			{Name: "check_config", Description: "Validar gokit.json e .env"},
			{Name: "align_directories", Description: "Alinhar diretórios físicos com o gokit.json"},
			{Name: "normalize_declarations", Description: "Normalizar nomes dinâmicos de migrations e seeders"},
			{Name: "go_tidy", Description: "Atualizar dependências (go mod tidy)"},
			{Name: "start_database", Description: "Iniciar o banco ativo pelo Docker Compose"},
			{Name: "check_conn", Description: "Ping físico com o Banco de Dados"},
		},
		Group2: []ReloadStep{
			{Name: "run_migrations", Description: "Verificar e executar migrations pendentes"},
			{Name: "run_seeds", Description: "Verificar e executar seeders pendentes"},
			{Name: "run_factories", Description: "Verificar e rodar factories ativas"},
		},
		Group3: []ReloadStep{
			{Name: "refresh_catalog", Description: "Reconstruir catálogo de autocomplete do Core (dsl.gen.go)"},
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
	case "align_directories":
		err = alignDirectories(step, state)
	case "normalize_declarations":
		var changed int
		changed, err = normalizeLegacyDeclarations(".", state)
		step.Message = fmt.Sprintf("%d declaração(ões) normalizada(s)", changed)
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
		step.Message = fmt.Sprintf("Toolchain Go não pôde ser executado: %s", strings.TrimSpace(string(out)))
		step.Suggestion = "Verifique go.execution e go.docker_service no gokit.json ou instale o Go no PATH para execução host."
		return err
	}
	step.Message = strings.TrimSpace(string(out))
	return nil
}

func checkConfigStructure(step *ReloadStep, state config.ConfigState) error {
	if state.ConfigFileError != nil {
		step.Message = fmt.Sprintf("Arquivo gokit.json ou .env inválido: %v", state.ConfigFileError)
		step.Suggestion = "Verifique o formato JSON do internal/gokit/gokit.json ou remova-o para gerar o scaffold de onboarding."
		return state.ConfigFileError
	}
	if state.Config == nil {
		step.Message = "Configuração do GoKit vazia ou nula."
		step.Suggestion = "Execute o GoKit na pasta para criar o gokit.json."
		return errors.New("config nula")
	}
	step.Message = "gokit.json e .env válidos."
	return nil
}

func startActiveDatabase(step *ReloadStep, state config.ConfigState) error {
	if state.Config == nil {
		return errors.New("config nula")
	}
	if state.Config.Go.DockerAutoStart != nil && !*state.Config.Go.DockerAutoStart {
		step.Message = "Inicialização automática desativada no gokit.json."
		return nil
	}
	if err := config.TestDatabaseConnection(state.ActiveDialect, state.ActiveURL); err == nil {
		step.Message = "Banco já estava em execução."
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
		step.Message = "Arquivo Docker Compose não encontrado na raiz do projeto."
		step.Suggestion = "Crie o serviço do banco ou defina go.docker_auto_start como false para usar um banco externo."
		return errors.New("docker compose ausente")
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
		step.Message = fmt.Sprintf("Não há serviço Docker conhecido para o dialeto %q.", state.ActiveDialect)
		return fmt.Errorf("dialeto sem servico docker: %s", state.ActiveDialect)
	}

	servicesCmd := exec.Command("docker", "compose", "-f", composeFile, "config", "--services")
	servicesOut, err := servicesCmd.CombinedOutput()
	if err != nil || !containsComposeService(string(servicesOut), service) {
		step.Message = fmt.Sprintf("O serviço %q não está ativo no %s.", service, composeFile)
		step.Suggestion = fmt.Sprintf("Descomente ou adicione o serviço %q no Docker Compose para o dialeto %s.", service, state.ActiveDialect)
		if err != nil {
			return err
		}
		return fmt.Errorf("servico docker %s ausente", service)
	}

	upCmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d", service)
	if out, err := upCmd.CombinedOutput(); err != nil {
		step.Message = fmt.Sprintf("Falha ao iniciar o serviço %s: %s", service, strings.TrimSpace(string(out)))
		step.Suggestion = fmt.Sprintf("Execute 'docker compose up -d %s' para inspecionar o erro.", service)
		return err
	}

	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = config.TestDatabaseConnection(state.ActiveDialect, state.ActiveURL)
		if lastErr == nil {
			step.Message = fmt.Sprintf("Serviço %s iniciado e banco pronto.", service)
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	step.Message = fmt.Sprintf("O serviço %s iniciou, mas o banco não ficou pronto: %v", service, lastErr)
	step.Suggestion = fmt.Sprintf("Inspecione os logs com 'docker compose logs %s'.", service)
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
		step.Message = "Sem conexão ativa configurada."
		step.Suggestion = "Verifique se a variável DB_DIALECT no .env está preenchida corretamente."
		return errors.New("sem conexão")
	}
	err := config.TestDatabaseConnection(state.ActiveDialect, state.ActiveURL)
	if err != nil {
		step.Message = fmt.Sprintf("Falha ao se conectar ao banco %s: %v", state.ActiveDialect, err)
		step.Suggestion = "Certifique-se de que o container do banco de dados está rodando e a porta está correta no .env."
		return err
	}
	step.Message = "Banco de dados respondendo ao Ping."
	return nil
}

func checkDockerState(step *ReloadStep) error {
	// 1. Checa se o CLI do Docker está instalado no sistema
	if _, err := exec.LookPath("docker"); err != nil {
		step.Message = "CLI do Docker não foi localizado no PATH do sistema."
		step.Suggestion = "Instale o Docker Desktop em https://www.docker.com/products/docker-desktop/ ou ative a CLI do Docker no seu ambiente."
		return err
	}

	// 2. Checa se o Daemon do Docker está rodando ativamente
	infoCmd := exec.Command("docker", "info")
	infoCmd.Stderr = io.Discard
	if err := infoCmd.Run(); err == nil {
		step.Message = "Daemon Docker ativo e comunicando."
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
			step.Message = "Daemon Docker estava inativo, mas foi iniciado automaticamente com sucesso."
			return nil
		}
	}

	step.Message = "Docker Daemon inativo (não foi possível estabelecer comunicação com o socket do Docker)."
	step.Suggestion = "Inicie o aplicativo Docker Desktop ou o serviço do Docker (ex: 'colima start' ou 'systemctl start docker')."
	return errors.New("docker daemon offline")
}

func runGoModTidy(step *ReloadStep, state config.ConfigState) error {
	goConfig := config.GoConfig{}
	if state.Config != nil {
		goConfig = state.Config.Go
	}
	result, err := gomodule.Ensure(gomodule.Options{
		Root:         ".",
		Module:       goConfig.Module,
		GoKitModule:  goConfig.GoKitModule,
		GoKitVersion: goConfig.GoKitVersion,
		GoKitLocal:   goConfig.GoKitLocal,
	})
	if err != nil {
		step.Message = fmt.Sprintf("Não foi possível alinhar o go.mod: %v", err)
		step.Suggestion = "Revise a seção go do internal/gokit/gokit.json, especialmente module e gokit_local."
		return err
	}

	cmd := goCommand(state, "mod", "tidy")
	out, err := cmd.CombinedOutput()
	if err != nil {
		step.Message = fmt.Sprintf("Falha ao executar 'go mod tidy': %s", strings.TrimSpace(string(out)))
		step.Suggestion = "Verifique a sintaxe dos arquivos .go ou remova caminhos de 'replace' inválidos no seu go.mod."
		return err
	}
	mode := "host"
	if strings.EqualFold(goConfig.Execution, "docker") {
		mode = "docker"
	}
	step.Message = fmt.Sprintf("Módulo %s alinhado com %s via %s.", result.Module, result.GoKitModule, mode)
	return nil
}

// ==========================================
// GRUPO 2: MÉTODO
// ==========================================

func executePendingMigrations(step *ReloadStep, state config.ConfigState) error {
	folder := filepath.Join(".", filepath.FromSlash(state.Config.Output.Migrate))
	files, err := loadPlans(folder)
	if err != nil {
		step.Message = fmt.Sprintf("Erro ao carregar migrations locais: %v", err)
		return err
	}

	db, err := sql.Open(getDriverName(state.ActiveDialect), state.ActiveURL)
	if err != nil {
		step.Message = fmt.Sprintf("Erro ao abrir banco: %v", err)
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
			step.Message = fmt.Sprintf("%d migration(s) pendente(s) não foram aplicadas; veja o diagnóstico e as soluções abaixo.", pendingCount)
			return err
		}
		step.Message = fmt.Sprintf("%d aplicadas", pendingCount)
	} else {
		step.Message = "Nenhuma pendente"
	}
	return nil
}

func executePendingSeeds(step *ReloadStep, state config.ConfigState) error {
	// Checa se há seeders locais
	tables, err := SeedableTables(".", state)
	if err != nil {
		step.Message = "Nenhum seeder pendente"
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
			step.Message = fmt.Sprintf("Erro ao rodar seeders: %v", err)
			return err
		}
		step.Message = fmt.Sprintf("%d aplicados", pendingCount)
	} else {
		step.Message = "Nenhum pendente"
	}
	return nil
}

func executeFactories(step *ReloadStep, state config.ConfigState) error {
	tables, err := FactoryTables(".", state)
	if err != nil || len(tables) == 0 {
		step.Message = "Nenhuma ativa"
		return nil
	}
	// Roda as factories (cria 10 linhas em cada tabela configurada)
	err = FactoryRun(".", state, nil)
	if err != nil {
		step.Message = fmt.Sprintf("Falha ao popular factories: %v", err)
		return err
	}
	step.Message = fmt.Sprintf("%d tabelas populadas", len(tables))
	return nil
}

// ==========================================
// GRUPO 3: METADADOS
// ==========================================

func refreshCoreCatalog(step *ReloadStep, state config.ConfigState) error {
	folder := filepath.Join(".", filepath.FromSlash(state.Config.Output.Migrate))
	err := migrationgo.RefreshCatalog(".", folder)
	if err != nil {
		step.Message = fmt.Sprintf("Falha ao reconstruir catálogo: %v", err)
		return err
	}
	step.Message = "Autocompletes do Core gerados."
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
					step.Message = fmt.Sprintf("Erro ao migrar pasta %s para %s: %v", alternatePath, configuredPath, err)
					return err
				}
				migratedCount++
			}
		}
	}

	if migratedCount > 0 {
		step.Message = fmt.Sprintf("%d diretórios alinhados", migratedCount)
	} else {
		step.Message = "Diretórios já alinhados"
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
