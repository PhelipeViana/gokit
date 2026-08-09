package main

import (
	"fmt"
	"os"

	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/migraterun"
	"github.com/PhelipeViana/gokit/internal/tui"
	"github.com/PhelipeViana/gokit/internal/updater"
)

// CommitHash e Version são injetados em tempo de compilação via -ldflags.
var (
	CommitHash = "development"
	Version    = "development"
)

func main() {
	// Remove binários antigos remanescentes de atualizações anteriores (.old)
	updater.CleanOldExecutables()

	// Se houver argumentos de linha de comando, roda em modo CLI
	if len(os.Args) > 1 {
		state := config.RunConfigChecks()
		if state.ConfigFileError != nil {
			fmt.Printf("Erro de configuração: %v\n", state.ConfigFileError)
			os.Exit(1)
		}

		if state.ActiveEnv == "production" {
			isAllowed := len(os.Args) >= 3 && os.Args[1] == "migrate" && (os.Args[2] == "run" || os.Args[2] == "up")
			if !isAllowed {
				fmt.Println("Erro: Apenas o comando 'gokit migrate run' é permitido em ambiente de produção.")
				os.Exit(1)
			}
		}

		if os.Args[1] == "seed" {
			if len(os.Args) < 3 {
				fmt.Println("Uso: gokit seed [run|validate|create <tabela>]")
				fmt.Println("\n  run       aplica os seeders pendentes")
				fmt.Println("  validate  confere os seeders sem tocar no banco")
				fmt.Println("  create    cria database/seeds/<tabela>/<timestamp>_seeder.go")
				os.Exit(1)
			}
			var err error
			switch os.Args[2] {
			case "run":
				err = migraterun.SeedRun(".", state, false)
			case "validate", "check":
				err = migraterun.SeedValidate(".", state)
			case "create":
				if len(os.Args) < 4 {
					fmt.Println("Uso: gokit seed create <tabela>")
					os.Exit(1)
				}
				var path string
				var total int
				path, total, err = migraterun.CreateSeedFile(".", state, os.Args[3])
				if err == nil {
					if total == 0 {
						fmt.Printf("Esqueleto de seeder criado em %s\n", path)
						fmt.Println("Preencha os valores e rode: gokit seed validate")
					} else {
						fmt.Printf("Seeder com %d linha(s) criado em %s\n", total, path)
					}
				}
			default:
				fmt.Printf("Comando de seed desconhecido: %s\n", os.Args[2])
				os.Exit(1)
			}
			if err != nil {
				fmt.Printf("Erro: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
		if os.Args[1] == "factory" {
			if len(os.Args) < 3 {
				fmt.Println("Uso: gokit factory [run|validate|create] [tabela...]")
				fmt.Println("\n  run       popula as tabelas com dados fake")
				fmt.Println("  validate  confere as factories sem tocar no banco")
				fmt.Println("  create    gera database/factories/<tabela>_factory.go a partir da migration")
				fmt.Println("\n  gokit factory run              todas as factories ativas")
				fmt.Println("  gokit factory run cidades      só cidades e as tabelas de que ela depende")
				fmt.Println("  gokit factory create           gera as factories que faltam")
				os.Exit(1)
			}
			var err error
			switch os.Args[2] {
			case "run":
				err = migraterun.FactoryRun(".", state, os.Args[3:])
			case "validate", "check":
				err = migraterun.FactoryValidate(".", state)
			case "create":
				tabela := ""
				if len(os.Args) >= 4 {
					tabela = os.Args[3]
				}
				err = migraterun.FactoryCreate(".", state, tabela)
			default:
				fmt.Printf("Comando de factory desconhecido: %s\n", os.Args[2])
				os.Exit(1)
			}
			if err != nil {
				fmt.Printf("Erro: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
		if os.Args[1] == "migrate" {
			if len(os.Args) < 3 {
				fmt.Println("Uso: gokit migrate [run|rollback|validate|create]")
				os.Exit(1)
			}
			var err error
			switch os.Args[2] {
			case "run", "up":
				err = migraterun.Run(".", state)
			case "rollback", "down":
				confirmDelete, confirmFresh := false, false
				for _, argument := range os.Args[3:] {
					if argument == "--confirm-delete" {
						confirmDelete = true
					}
					if argument == "--confirm-fresh" {
						confirmFresh = true
					}
				}
				err = migraterun.RunDevelopmentRollback(".", state, confirmDelete, confirmFresh)
			case "validate", "check":
				err = migraterun.ValidateReport(".", state)
			case "create":
				if len(os.Args) < 4 {
					fmt.Println("Uso: gokit migrate create <nome> [metodo]")
					fmt.Println("\nMétodos válidos:")
					fmt.Println("  create_table, drop_table, add_column, alter_column, drop_column,")
					fmt.Println("  add_foreign_key, drop_foreign_key, create_index, drop_index,")
					fmt.Println("  create_view, alter_view, drop_view, create_sequence, drop_sequence,")
					fmt.Println("  rename_table, rename_column, add_primary_key, add_unique, add_check,")
					fmt.Println("  drop_constraint, raw_sql, todo")
					os.Exit(1)
				}
				nome := os.Args[3]
				metodo := "todo"
				if len(os.Args) >= 5 {
					metodo = os.Args[4]
				}
				var filename string
				filename, err = migraterun.CreateScaffoldMigration(".", state, nome, metodo, "")
				if err == nil {
					fmt.Printf("Migration criada com sucesso: %s\n", filename)
				}
			default:
				fmt.Printf("Comando de migração desconhecido: %s\n", os.Args[2])
				os.Exit(1)
			}
			if err != nil {
				fmt.Printf("Erro: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
		if os.Args[1] == "doctor" || os.Args[1] == "check" {
			cliui.PrintTitle("GoKit · Doctor")
			report := migraterun.RunDoctor(state)

			dialect := state.ActiveDialect
			fmt.Printf("→ %s (%s) [ATIVO]\n", dialect, state.ActiveClient)
			fmt.Printf("  Histórico: %s\n", state.ActiveURL)

			if report.ConnSuccess {
				fmt.Println("  ✓ Conectividade física: OK")
				if report.VersionOK {
					fmt.Printf("  ✓ Versão do Banco: %s (Compatível)\n", report.Version)
				} else {
					fmt.Printf("  ✗ Versão do Banco: %s (%s)\n", report.Version, report.VersionWarning)
				}
				if report.DDLSuccess {
					fmt.Println("  ✓ Permissão de DDL: OK (CREATE/DROP executados)")
				} else {
					fmt.Printf("  ✗ Permissão de DDL: Falha (%v)\n", report.DDLError)
				}
			} else {
				fmt.Printf("  ✗ Conectividade física: Falha (Erro: %v)\n", report.ConnError)
			}
			os.Exit(0)
		}
		if os.Args[1] == "rollback" {
			confirmDelete, confirmFresh := false, false
			for _, argument := range os.Args[2:] {
				switch argument {
				case "--confirm-delete":
					confirmDelete = true
				case "--confirm-fresh":
					confirmFresh = true
				}
			}
			if err := migraterun.RunDevelopmentRollback(".", state, confirmDelete, confirmFresh); err != nil {
				fmt.Printf("Erro: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
		if os.Args[1] == "reload" {
			var err error
			if len(os.Args) >= 3 && os.Args[2] == "--fresh" {
				err = migraterun.RunFreshReload(".", state)
			} else {
				err = migraterun.RunReload(state)
			}
			if err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	// Inicia a interface TUI do aplicativo
	err := tui.Start(Version, CommitHash)
	if err != nil {
		fmt.Printf("Ocorreu um erro no aplicativo: %v\n", err)
		os.Exit(1)
	}
}
