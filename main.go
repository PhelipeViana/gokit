package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/PhelipeViana/gokit/internal/aviso"
	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/gomodule"
	"github.com/PhelipeViana/gokit/internal/i18n"
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
	aviso.DefinirVersao(Version)

	// Se houver argumentos de linha de comando, roda em modo CLI
	if len(os.Args) > 1 {
		state := config.RunConfigChecks()
		// A proteção entra assim que o state existe: antes dele não há configuração
		// para saber para onde notificar, e depois dele todo o caminho do comando fica
		// coberto.
		defer protegerContraPanic(state, strings.Join(os.Args[1:], " "))
		avisarFalhasDaCriacao(state)
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
			falhou(state, os.Args[1]+" "+os.Args[2], err)
			os.Exit(0)
		}
		if os.Args[1] == "factory" {
			if len(os.Args) < 3 {
				fmt.Println("Uso: gokit factory [run|validate|create] [tabela...]")
				fmt.Println("\n  run       popula as tabelas com dados fake")
				fmt.Println("  validate  confere as factories sem tocar no banco")
				fmt.Println("  create    gera internal/gokit/factory/factories.go a partir da migration")
				fmt.Println("\n  gokit factory run              todas as factories ativas")
				fmt.Println("  gokit factory run cidades      só cidades e as tabelas de que ela depende")
				fmt.Println("  gokit factory create           gera as factories que faltam")
				os.Exit(1)
			}
			var err error
			switch os.Args[2] {
			case "run":
				// --force repovoa tabela que já tem dados. Fora dele, tabela com
				// linha é pulada: a factory começa apagando, e dado que ela não
				// produziu não é dela para apagar.
				forcar := false
				var tabelas []string
				for _, argumento := range os.Args[3:] {
					if argumento == "--force" {
						forcar = true
						continue
					}
					tabelas = append(tabelas, argumento)
				}
				err = migraterun.FactoryRun(".", state, tabelas, forcar)
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
			falhou(state, os.Args[1]+" "+os.Args[2], err)
			os.Exit(0)
		}
		if os.Args[1] == "migrate" {
			if len(os.Args) < 3 {
				fmt.Println("Uso: gokit migrate [run|rollback|validate|scan|import|baseline|create]")
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
			case "scan":
				detalhar := false
				for _, argumento := range os.Args[3:] {
					if argumento == "--detail" || argumento == "--detalhe" {
						detalhar = true
					}
				}
				err = migraterun.MigrateScan(".", state, detalhar)
			case "baseline":
				// Marcar sem executar é escrever no histórico: exige confirmação
				// explícita, como o rollback.
				confirmar := false
				for _, argumento := range os.Args[3:] {
					if argumento == "--confirm" {
						confirmar = true
					}
				}
				err = migraterun.MigrateBaseline(".", state, confirmar)
			case "import":
				// Sem --confirm o import só mostra. Migration é histórico
				// versionado: gerar arquivo não se desfaz sozinho.
				confirmar := false
				for _, argumento := range os.Args[3:] {
					if argumento == "--confirm" {
						confirmar = true
					}
				}
				err = migraterun.MigrateImport(".", state, confirmar)
			case "create":
				if len(os.Args) < 4 {
					fmt.Println("Uso: gokit migrate create <nome> [metodo]")
					fmt.Println("\nMétodos válidos:")
					fmt.Println("  create_table, drop_table, add_column, alter_column, drop_column,")
					fmt.Println("  add_foreign_key, drop_foreign_key, create_index, drop_index,")
					fmt.Println("  create_sequence, drop_sequence,")
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
			falhou(state, os.Args[1]+" "+os.Args[2], err)
			os.Exit(0)
		}
		// gokit mode — mostra ou troca o modo de consumo do gokit.
		//
		// Sem argumento apenas informa, porque trocar de modo reescreve go.mod e
		// go.work: é o tipo de comando que não deve fazer nada por engano.
		if os.Args[1] == "mode" || os.Args[1] == "modo" {
			if len(os.Args) < 3 {
				atual := config.ModoDoProjeto(".", state.Config)
				fmt.Printf(i18n.T("cli_mode_current"), atual)
				fmt.Println()
				if conflito := gomodule.ConferirModo(".", atual); conflito != "" {
					fmt.Printf(i18n.T("cli_mode_conflito"), conflito)
					fmt.Println()
				}
				fmt.Println(i18n.T("cli_mode_usage"))
				os.Exit(0)
			}
			resultado, err := config.DefinirModo(".", os.Args[2])
			if err != nil {
				fmt.Printf("%s\n", err)
				os.Exit(1)
			}
			fmt.Printf(i18n.T("cli_mode_changed"), resultado.Mode)
			fmt.Println()
			if resultado.Mode == gomodule.ModoDev {
				fmt.Printf(i18n.T("cli_mode_dev_detail"), resultado.GoKitLocal)
			} else {
				fmt.Printf(i18n.T("cli_mode_prod_detail"), resultado.GoKitModule, resultado.GoKitVersion)
			}
			fmt.Println()
			// Avisar depois de trocar é o momento em que o aviso serve: acabamos de
			// remover o go.work do projeto, e se ainda há um acima dele o prod não
			// é o que parece.
			if conflito := gomodule.ConferirModo(".", resultado.Mode); conflito != "" {
				fmt.Printf(i18n.T("cli_mode_conflito"), conflito)
				fmt.Println()
			}
			os.Exit(0)
		}
		// gokit special — mapeia view, function e procedure do banco ativo.
		//
		// Comando próprio, e não um subcomando de migrate, porque a responsabilidade é
		// outra: migrate DECLARA e aplica; special só OBSERVA e registra. Os três juntos
		// num comando só porque o ciclo é o mesmo — descobrir, registrar, gerar acessador
		// — e separá-los triplicaria esse ciclo.
		if os.Args[1] == "special" {
			// Sem --confirm apenas mostra, pela mesma razão do import: o registro é
			// versionado, e escrever arquivo não se desfaz sozinho.
			confirmar := false
			for _, argumento := range os.Args[2:] {
				if argumento == "--confirm" {
					confirmar = true
				}
			}
			falhou(state, "special", migraterun.SpecialMap(".", state, confirmar))
			os.Exit(0)
		}
		// gokit orm — regera só o mapeamento da ORM a partir das migrations.
		//
		// O reload completo faz isso, mas passa por Docker, migrations e seeds. Ao
		// mexer no schema, regerar o mapa sozinho é o que se quer noventa por cento
		// das vezes, e não depende de banco: a fonte é o corpus em AST.
		if os.Args[1] == "orm" {
			// Catálogos antes das entidades, pela mesma sequência que o menu usa: a
			// entidade cita `core.Table.X`, e gerar entidade contra catálogo velho
			// produz arquivo que não compila por um nome que ninguém escreveu.
			total, semAtalho, err := migraterun.AtualizarEntidades(".", state)
			if err != nil {
				fmt.Printf("Erro: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf(i18n.T("rel_orm_done"), total)
			fmt.Println()
			// FK que não virou atalho de navegação. A entidade existe e é consultável;
			// só o .With(...) daquela relação não sai. Dizer isso agora evita a
			// pergunta "por que esta relação não existe?" com o arquivo na mão.
			if len(semAtalho) > 0 {
				fmt.Printf(i18n.T("gen_orm_rel_skipped"), len(semAtalho))
				fmt.Println()
				for _, linha := range semAtalho {
					fmt.Printf("  · %s\n", linha)
				}
				central(state).Notificar(aviso.Aviso{
					Nivel: aviso.NivelInfo,
					Acao:  "relação sem atalho de navegação",
					Corpo: fmt.Sprintf("%d chave(s) estrangeira(s) não viraram atalho na ORM porque a coluna "+
						"referenciada não é numérica. As entidades existem; só o .With() daquela relação não.", len(semAtalho)),
					Contexto: map[string]string{"comando": "orm"},
					Bruto:    strings.Join(semAtalho, "\n"),
				})
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

	// A TUI também precisa de cobertura: quem usa o menu tem o mesmo motor por baixo,
	// e panic ali morria do mesmo jeito.
	estadoDaTUI := config.RunConfigChecks()
	defer protegerContraPanic(estadoDaTUI, "menu")
	avisarFalhasDaCriacao(estadoDaTUI)

	err := tui.Start(Version, CommitHash)
	if err != nil {
		fmt.Printf("Ocorreu um erro no aplicativo: %v\n", err)
		central(estadoDaTUI).Notificar(aviso.Aviso{
			Nivel:    aviso.NivelDanger,
			Acao:     "erro no menu",
			Emoji:    ":rotating_light:",
			Corpo:    "A interface do gokit terminou com erro. Não é erro de uso: a TUI não deveria falhar.",
			Contexto: map[string]string{"comando": "menu"},
			Bruto:    err.Error(),
		})
		os.Exit(1)
	}
}

// avisarFalhasDaCriacao reporta o que a preparação do projeto não conseguiu fazer:
// arquivo do scaffold que não foi escrito, .env mapeado que não abriu.
//
// É warning, e não erro: o comando roda de qualquer forma. Mas cada uma dessas falhas
// só aparece MAIS TARDE e disfarçada — o .env que não abriu vira "não conectou ao
// banco", o arquivo que faltou vira "esse recurso não funciona". Antes disto os dois
// erros eram descartados, então nem o usuário nem quem mantém o gokit ficava sabendo.
func avisarFalhasDaCriacao(state config.ConfigState) {
	if len(state.SetupWarnings) == 0 {
		return
	}
	for _, falha := range state.SetupWarnings {
		fmt.Printf("Aviso: %s\n", falha)
	}
	central(state).Notificar(aviso.Aviso{
		Nivel: aviso.NivelWarning,
		Acao:  "preparação do projeto incompleta",
		Emoji: ":construction:",
		Corpo: fmt.Sprintf("%d etapa(s) da preparação do projeto não concluíram. O comando roda, mas o que "+
			"está listado abaixo vai faltar — normalmente é permissão de escrita na pasta.", len(state.SetupWarnings)),
		Contexto: map[string]string{"configuração": state.ConfigPath},
		Bruto:    strings.Join(state.SetupWarnings, "\n"),
	})
}

// falhou trata o fim de qualquer comando: mostra o erro e, quando ele NÃO é um erro
// mapeado, avisa quem cuida do gokit.
//
// A fronteira é o cliui.UserError: ele existe porque alguém já encontrou aquele caso,
// entendeu e escreveu a solução — então o usuário tem o que fazer e não há nada a
// reportar. Erro cru é o oposto: ou é caso de banco que o motor não previu, ou é
// defeito dele. Enquanto esse não chega a quem mantém, cada usuário topa com ele
// sozinho e o motor não melhora.
func falhou(state config.ConfigState, comando string, err error) {
	if err == nil {
		return
	}
	fmt.Printf("Erro: %v\n", err)

	// O gokit CLASSIFICA e emite sempre; quem recebe filtra por nível. A supressão não
	// pode morar aqui: se este ponto decidir não emitir, nenhum projeto consegue pedir
	// aquele aviso depois — e há quem queira acompanhar os próprios erros de uso.
	//
	// A fronteira entre os dois níveis é o cliui.UserError. Ele existe quando alguém já
	// encontrou o caso, entendeu e escreveu a solução: é erro de USO. Erro cru é o
	// oposto, e é o mais grave, porque ninguém previu.
	var mapeado cliui.UserError
	if errors.As(err, &mapeado) {
		central(state).Notificar(aviso.Aviso{
			Nivel: aviso.NivelError,
			Acao:  "erro de uso",
			Emoji: ":warning:",
			Corpo: fmt.Sprintf("O comando `%s` foi interrompido por um erro previsto, e a solução já está "+
				"na mensagem — não há nada a corrigir no motor.", comando),
			Contexto: map[string]string{"comando": comando, "solução": mapeado.Solution},
			Bruto:    mapeado.Message,
		})
		os.Exit(1)
	}

	central(state).Notificar(aviso.Aviso{
		Nivel: aviso.NivelDanger,
		Acao:  "erro não mapeado",
		Emoji: ":rotating_light:",
		Corpo: fmt.Sprintf("O comando `%s` falhou com um erro que o motor não mapeia: não há mensagem de solução, "+
			"então é caso de banco que ninguém previu ou defeito do gokit.", comando),
		Contexto: map[string]string{"comando": comando},
		Bruto:    err.Error(),
	})
	os.Exit(1)
}

// central monta o notificador do projeto. A montagem mora no pacote aviso para não
// divergir entre os vários lugares que notificam.
func central(state config.ConfigState) aviso.Central {
	return aviso.DoEstado(state)
}

// protegerContraPanic transforma qualquer panic em aviso de nível danger e sai com
// erro, em vez de despejar o stack do Go na cara do usuário.
//
// É o buraco de cobertura mais grave que existia: panic é, por definição, defeito do
// motor — ninguém previu — e sem isto ele morria sem deixar rastro em lugar nenhum. O
// stack vai no relatório colável porque é ele que diz a linha onde quebrou, e sem essa
// linha o relatório não resolve nada.
//
// O panic é RE-lançado depois de notificar quando não há canal configurado: engolir a
// falha deixaria o usuário sem o stack e sem o aviso, que é pior que os dois.
func protegerContraPanic(state config.ConfigState, comando string) {
	recuperado := recover()
	if recuperado == nil {
		return
	}

	pilha := string(debug.Stack())
	central := central(state)
	central.Notificar(avisoDePanic(comando, recuperado, pilha))

	fmt.Fprintf(os.Stderr, "\nErro interno do gokit: %v\n", recuperado)
	if !central.Ativa() {
		// Sem canal, o stack é a única informação que existe sobre a falha.
		fmt.Fprintf(os.Stderr, "\n%s\n", pilha)
	}
	os.Exit(2)
}

// avisoDePanic monta o aviso. É separado da recuperação para poder ser conferido em
// teste: a recuperação chama os.Exit, e teste que sai do processo não afirma nada.
func avisoDePanic(comando string, recuperado any, pilha string) aviso.Aviso {
	return aviso.Aviso{
		Nivel: aviso.NivelDanger,
		Acao:  "panic no motor",
		Emoji: ":skull:",
		Corpo: fmt.Sprintf("O comando `%s` quebrou com panic. Panic é sempre defeito do motor, "+
			"nunca erro de uso: o gokit deveria ter tratado essa condição.", comando),
		Contexto: map[string]string{"comando": comando, "panic": fmt.Sprint(recuperado)},
		Bruto:    pilha,
	}
}
