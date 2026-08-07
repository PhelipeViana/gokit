package main

import (
	"fmt"
	"os"
	"time"

	"gokit/internal/config"
	"gokit/internal/migraterun"
	"gokit/internal/tui"
	"gokit/internal/updater"
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
		if os.Args[1] == "seed" {
			if len(os.Args) < 3 {
				fmt.Println("Uso: gokit seed [run|validate|create <tabela>]")
				fmt.Println("\n  run       aplica os seeders pendentes")
				fmt.Println("  validate  confere os seeders sem tocar no banco")
				fmt.Println("  create    cria database/seeds/<tabela>/<timestamp>_seeder.go")
				os.Exit(1)
			}
			state := config.RunConfigChecks()
			if state.ConfigFileError != nil {
				fmt.Printf("Erro de configuração: %v\n", state.ConfigFileError)
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
			state := config.RunConfigChecks()
			if state.ConfigFileError != nil {
				fmt.Printf("Erro de configuração: %v\n", state.ConfigFileError)
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
			state := config.RunConfigChecks()
			if state.ConfigFileError != nil {
				fmt.Printf("Erro de configuração: %v\n", state.ConfigFileError)
				os.Exit(1)
			}
			var err error
			switch os.Args[2] {
			case "run", "up":
				err = migraterun.Run(".", state)
			case "rollback", "down":
				err = migraterun.Rollback(".", state)
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
	}

	// Checagem rápida e silenciosa de versão no GitHub
	updateAvailable, _, remoteSHA := updater.RunSilentUpdateCheck(CommitHash)

	// Se houver nova versão disponível, atualiza e reinicia automaticamente em background
	if updateAvailable {
		fmt.Printf("\n\033[1m\033[36m⚡ Nova versão detectada (%s). Atualizando gokit automaticamente...\033[0m\n", remoteSHA[:7])
		err := updater.RunSelfUpdate()
		if err != nil {
			fmt.Printf("\033[31m✖ Erro ao atualizar automaticamente: %v. Continuando com a versão atual...\033[0m\n", err)
			time.Sleep(2 * time.Second)
		} else {
			fmt.Println("\033[32m✔ Atualizado com sucesso! Reiniciando...\033[0m")
			time.Sleep(500 * time.Millisecond)

			// Reinicia o processo atual
			err = updater.RestartProcess()
			if err != nil {
				fmt.Printf("\033[31m✖ Falha ao reiniciar: %v. Por favor, execute novamente.\033[0m\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	// Inicia a interface TUI do aplicativo
	err := tui.Start(Version)
	if err != nil {
		fmt.Printf("Ocorreu um erro no aplicativo: %v\n", err)
		os.Exit(1)
	}
}
