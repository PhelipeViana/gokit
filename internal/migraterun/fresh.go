package migraterun

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/config"
)

func ensureDevelopmentResetAllowed(state config.ConfigState) error {
	if state.ActiveEnv != "development" {
		return fmt.Errorf("fresh reload bloqueado: APP_ENV precisa ser development")
	}
	if state.Config == nil {
		return fmt.Errorf("configuração ausente")
	}
	connection, ok := state.Config.Connections[state.ActiveClient]
	if !ok {
		return fmt.Errorf("conexão ativa %q não encontrada", state.ActiveClient)
	}
	if !isLocalHost(connectionHost(connection)) {
		return fmt.Errorf("fresh reload bloqueado para host não local: %s", connectionHost(connection))
	}
	return nil
}

func resetDevelopmentDatabase(state config.ConfigState, files []migrationFile) error {
	if err := ensureDevelopmentResetAllowed(state); err != nil {
		return err
	}
	connection := state.Config.Connections[state.ActiveClient]
	history := state.Config.Migrate.Table
	if history == "" {
		history = "migrations_gokit"
	}
	if err := resetConnection(connection, history, files); err != nil {
		return err
	}

	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	seedHistory := seedHistoryTable(state)
	exists, err := tableExists(ctx, db, dialect, connection.Schema, seedHistory)
	if err != nil {
		return err
	}
	if exists {
		if _, err := db.ExecContext(ctx, dropTableSQL(dialect, connection.Schema, seedHistory)); err != nil {
			return fmt.Errorf("remover histórico de seeders %s: %w", seedHistory, err)
		}
	}
	return nil
}

func RunFreshReload(root string, state config.ConfigState) error {
	if state.Config == nil {
		return fmt.Errorf("configuração ausente")
	}
	files, err := loadPlans(filepathJoin(root, state.Config.Output.Migrate))
	if err != nil {
		return describeLoadError(err)
	}
	if err := resetDevelopmentDatabase(state, files); err != nil {
		return err
	}
	return RunReload(state)
}

func RunDevelopmentRollback(root string, state config.ConfigState, confirmDelete, confirmFresh bool) error {
	if err := ensureDevelopmentResetAllowed(state); err != nil {
		return err
	}
	plan, err := PlanDevelopmentRollback(root)
	if err != nil {
		return err
	}
	fmt.Printf("Geração mais recente: %s\n", plan.Target)
	for _, path := range plan.Files {
		fmt.Printf("  - %s\n", path)
	}
	for _, path := range plan.Blocked {
		fmt.Printf("  ! %s\n", path)
	}
	if len(plan.Files) == 0 {
		return fmt.Errorf("nenhum arquivo removível encontrado")
	}
	if !confirmDelete || !confirmFresh {
		fmt.Println("\nPreview concluído. Para executar use --confirm-delete --confirm-fresh.")
		return nil
	}

	filesBeforeRemoval, err := loadPlans(filepathJoin(root, state.Config.Output.Migrate))
	if err != nil {
		return describeLoadError(err)
	}
	if err := resetDevelopmentDatabase(state, filesBeforeRemoval); err != nil {
		return err
	}
	if err := removeRollbackFiles(root, plan); err != nil {
		return err
	}
	return RunReload(state)
}

func filepathJoin(root, configured string) string {
	if configured == "" {
		return root
	}
	if filepath.IsAbs(configured) {
		return configured
	}
	return filepath.Join(root, filepath.FromSlash(configured))
}
