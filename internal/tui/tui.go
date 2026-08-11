package tui

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migraterun"
	"github.com/PhelipeViana/gokit/internal/updater"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var Version string

type menuState int

type statusTickMsg struct{}
type spinnerTickMsg struct{}

type reloadFinishedMsg struct {
	output string
	err    error
}

const (
	stateMainMenu menuState = iota
	stateMigrationsMenu
	stateMigrationSelectMethod
	stateMigrationInputName
	stateMigrationSelectTable
	stateMigrationSelectView
	stateMigrationCreating
	stateMigrationRunning
	stateMigrationRollingBack
	stateMigrationRollbackConfirmDelete
	stateMigrationRollbackConfirmFresh
	stateMigrationValidating
	stateSeedMenu
	stateSeedSelectTable
	stateSeedCreating
	stateFactoryMenu
	stateFactorySelectTable
	stateFactoryRunning
	stateConfigScreen
	stateReloadRunning
	stateUpdateConfirm
	stateUpdateRunning
)

// captureOutput executa a ação desviando os.Stdout para um buffer. O runner
// escreve o relatório com fmt.Printf, e sem isso a TUI limpava a tela por cima
// e só sobrava a mensagem genérica de sucesso — inútil para teste manual.
func captureOutput(action func() error) (string, error) {
	original := os.Stdout
	reader, writer, pipeErr := os.Pipe()
	if pipeErr != nil {
		return "", action()
	}
	os.Stdout = writer

	captured := make(chan string, 1)
	go func() {
		var builder strings.Builder
		buffer := make([]byte, 4096)
		for {
			read, err := reader.Read(buffer)
			if read > 0 {
				builder.Write(buffer[:read])
			}
			if err != nil {
				break
			}
		}
		captured <- builder.String()
	}()

	err := action()

	writer.Close()
	os.Stdout = original
	return <-captured, err
}

// lastLines mantém a caixa de saída legível quando o relatório é longo.
func lastLines(text string, limit int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) <= limit {
		return strings.Join(lines, "\n")
	}
	hidden := len(lines) - limit
	return i18n.Tf("tui_lines_omitted", hidden, strings.Join(lines[hidden:], "\n"))
}

type model struct {
	state               menuState
	cursor              int
	choices             []string
	migrationsChoices   []string
	configData          config.ConfigState
	migrationError      error
	migrationOutput     string
	migrationNameInput  string
	methodsChoices      []string
	methodCursor        int
	availableTables     []string
	availableViews      []string
	tableCursor         int
	viewCursor          int
	selectedTableOrView string
	seedChoices         []string
	seedTables          []string
	seedCursor          int
	factoryChoices      []string
	factoryTables       []string
	factoryCursor       int
	doctorReport        migraterun.DoctorReport
	updateStatus        updater.Status
	updateMenuIndex     int
	exitMenuIndex       int
	statusSignature     string
	terminalHeight      int
	actionRunning       bool
	spinnerFrame        int
}

// Estilos Lip Gloss inspirados na estética Charm Bracelet
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00F0FF")). // Ciano Brilhante
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00F0FF")).
			Padding(0, 3).
			MarginLeft(1).
			MarginTop(1)

	versionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500")). // Laranja
			MarginLeft(2)

	selectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FF007F")). // Hot Pink / Magenta
				MarginLeft(2)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8F8F2")). // Branco suave
			MarginLeft(4)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4")). // Roxo/Cinza escuro
			MarginLeft(2).
			MarginTop(1)

	actionBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#50FA7B")). // Verde
			Padding(1, 3).
			MarginLeft(2).
			MarginTop(1)
)

// Start inicia o loop do aplicativo interativo Bubble Tea
func Start(version, commitHash string) error {
	Version = version

	// Carrega dados iniciais de configuração para expor o status no menu
	initialConfig := config.RunConfigChecks()

	updateStatus := updater.Check(commitHash)
	var choices []string
	if initialConfig.ActiveEnv == "production" {
		choices = []string{
			i18n.T("menu_prod_update"),
		}
	} else {
		choices = []string{
			i18n.T("menu_reload"),
			i18n.T("menu_migrations"),
			i18n.T("menu_seeds"),
			i18n.T("menu_factories"),
			i18n.T("menu_config"),
		}
	}
	updateIndex := -1
	if updateStatus.Available {
		updateIndex = len(choices)
		choices = append(choices, fmt.Sprintf("⚡ Atualizar GoKit %s → %s", updateStatus.Local, updateStatus.Remote))
	}
	exitIndex := len(choices)
	choices = append(choices, i18n.T("menu_exit"))

	m := model{
		state:   stateMainMenu,
		cursor:  0,
		choices: choices,
		factoryChoices: []string{
			i18n.T("fact_create"),
			i18n.T("fact_validate"),
			i18n.T("fact_run_all"),
			i18n.T("fact_run_one"),
			i18n.T("mig_back"),
		},
		seedChoices: []string{
			i18n.T("seed_create"),
			i18n.T("seed_validate"),
			i18n.T("seed_run"),
			i18n.T("mig_back"),
		},
		migrationsChoices: []string{
			i18n.T("mig_create"),
			i18n.T("mig_validate"),
			i18n.T("mig_run"),
			i18n.T("mig_rollback"),
			i18n.T("mig_back"),
		},
		configData:      initialConfig,
		updateStatus:    updateStatus,
		updateMenuIndex: updateIndex,
		exitMenuIndex:   exitIndex,
		statusSignature: environmentSignature(initialConfig),
		terminalHeight:  24,
		methodsChoices: []string{
			"create_table", "drop_table", "add_column", "alter_column", "drop_column",
			"add_foreign_key", "drop_foreign_key", "create_index", "drop_index",
			"create_view", "alter_view", "drop_view", "create_sequence", "drop_sequence",
			"rename_table", "rename_column", "add_primary_key", "add_unique", "add_check",
			"drop_constraint", "raw_sql", "todo",
		},
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Init inicializa o modelo do Bubble Tea
func (m model) Init() tea.Cmd {
	return statusTick()
}

func statusTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return statusTickMsg{} })
}

func spinnerTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

func runReloadCmd(state config.ConfigState) tea.Cmd {
	return func() tea.Msg {
		output, err := captureOutput(func() error { return migraterun.RunReload(state) })
		return reloadFinishedMsg{output: output, err: err}
	}
}

func environmentSignature(state config.ConfigState) string {
	paths := []string{"gokit.json"}
	if state.ConfigPath != "" {
		paths[0] = state.ConfigPath
	}
	if state.Config != nil && state.Config.Environment.MapperEnv != "" {
		paths = append(paths, state.Config.Environment.MapperEnv)
	}
	hash := sha256.New()
	for _, path := range paths {
		hash.Write([]byte(path))
		content, err := os.ReadFile(path)
		if err != nil {
			hash.Write([]byte(err.Error()))
			continue
		}
		hash.Write(content)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// Update gerencia as interações do Bubble Tea
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.terminalHeight = msg.Height
		return m, nil
	case spinnerTickMsg:
		if !m.actionRunning {
			return m, nil
		}
		m.spinnerFrame++
		return m, spinnerTick()
	case reloadFinishedMsg:
		m.actionRunning = false
		m.migrationOutput = msg.output
		m.migrationError = msg.err
		m.configData = config.RunConfigChecks()
		m.statusSignature = environmentSignature(m.configData)
		return m, tea.ClearScreen
	case statusTickMsg:
		signature := environmentSignature(m.configData)
		if signature != m.statusSignature {
			m.configData = config.RunConfigChecks()
			m.statusSignature = environmentSignature(m.configData)
			if m.state == stateConfigScreen {
				m.doctorReport = migraterun.RunDoctor(m.configData)
			}
		}
		return m, statusTick()
	case tea.KeyMsg:
		if m.state == stateUpdateConfirm {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "n":
				m.state = stateMainMenu
				return m, tea.ClearScreen
			case "enter", "y":
				m.state = stateUpdateRunning
				m.migrationOutput, m.migrationError = captureOutput(func() error {
					fmt.Printf(i18n.T("tui_downloading"), m.updateStatus.Remote)
					if err := updater.RunSelfUpdate(); err != nil {
						return err
					}
					fmt.Println(i18n.T("tui_update_installed"))
					return updater.RestartProcess()
				})
				if m.migrationError == nil {
					return m, tea.Quit
				}
				return m, tea.ClearScreen
			}
			return m, nil
		}
		if m.state == stateMigrationRollbackConfirmDelete || m.state == stateMigrationRollbackConfirmFresh {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "n":
				m.state = stateMigrationsMenu
				m.cursor = 0
				return m, tea.ClearScreen
			case "enter", "y":
				if m.state == stateMigrationRollbackConfirmDelete {
					m.state = stateMigrationRollbackConfirmFresh
					return m, tea.ClearScreen
				}
				m.state = stateMigrationRollingBack
				m.migrationOutput, m.migrationError = captureOutput(func() error {
					return migraterun.RunDevelopmentRollback(".", m.configData, true, true)
				})
				return m, tea.ClearScreen
			}
			return m, nil
		}
		if m.state == stateMigrationInputName {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "enter":
				if strings.TrimSpace(m.migrationNameInput) != "" {
					method := m.methodsChoices[m.methodCursor]
					if method == "create_table" || method == "create_view" || method == "create_sequence" || method == "raw_sql" || method == "todo" {
						name, err := migraterun.CreateScaffoldMigration(".", m.configData, m.migrationNameInput, method, "")
						m.migrationError = err
						m.migrationOutput = name
						m.state = stateMigrationCreating
						return m, tea.ClearScreen
					} else {
						descName := strings.TrimSpace(m.migrationNameInput)
						target := m.selectedTableOrView
						fileNameDesc := ""
						switch method {
						case "add_column":
							fileNameDesc = "add_" + descName + "_to_" + target
						case "alter_column":
							fileNameDesc = "alter_" + descName + "_in_" + target
						case "drop_column":
							fileNameDesc = "drop_" + descName + "_from_" + target
						case "add_foreign_key":
							fileNameDesc = "add_fk_" + descName + "_to_" + target
						case "drop_foreign_key":
							fileNameDesc = "drop_fk_" + descName + "_from_" + target
						case "create_index":
							fileNameDesc = "create_index_" + descName + "_on_" + target
						case "drop_index":
							fileNameDesc = "drop_index_" + descName + "_on_" + target
						case "rename_column":
							fileNameDesc = "rename_col_in_" + target
						case "add_unique":
							fileNameDesc = "add_uk_" + descName + "_to_" + target
						case "drop_constraint":
							fileNameDesc = "drop_constraint_" + descName + "_from_" + target
						default:
							fileNameDesc = method + "_" + descName + "_" + target
						}

						name, err := migraterun.CreateScaffoldMigration(".", m.configData, fileNameDesc, method, target)
						m.migrationError = err
						m.migrationOutput = name
						m.state = stateMigrationCreating
						return m, tea.ClearScreen
					}
				}
				return m, nil
			case "backspace":
				if len(m.migrationNameInput) > 0 {
					m.migrationNameInput = m.migrationNameInput[:len(m.migrationNameInput)-1]
				}
				return m, nil
			case "esc":
				method := m.methodsChoices[m.methodCursor]
				if method == "create_table" || method == "create_view" || method == "create_sequence" || method == "raw_sql" || method == "todo" {
					m.state = stateMigrationSelectMethod
				} else {
					m.state = stateMigrationSelectTable
				}
				return m, nil
			default:
				kStr := msg.String()
				if len(kStr) == 1 && (kStr[0] >= 'a' && kStr[0] <= 'z' || kStr[0] >= 'A' && kStr[0] <= 'Z' || kStr[0] >= '0' && kStr[0] <= '9' || kStr[0] == '_' || kStr[0] == '-' || kStr[0] == ' ') {
					if len(m.migrationNameInput) < 50 {
						if kStr[0] == ' ' {
							m.migrationNameInput += "_"
						} else {
							m.migrationNameInput += kStr
						}
					}
				}
				return m, nil
			}
		}

		if m.state == stateMigrationSelectMethod {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				m.methodCursor--
				if m.methodCursor < 0 {
					m.methodCursor = len(m.methodsChoices) - 1
				}
				return m, nil
			case "down", "j":
				m.methodCursor++
				if m.methodCursor >= len(m.methodsChoices) {
					m.methodCursor = 0
				}
				return m, nil
			case "enter":
				method := m.methodsChoices[m.methodCursor]
				tables, views, err := migraterun.LoadCatalogTablesAndViews(".", m.configData)
				if err != nil {
					m.migrationError = err
					m.state = stateMigrationCreating
					return m, tea.ClearScreen
				}
				m.availableTables = tables
				m.availableViews = views

				switch method {
				case "create_table", "create_view", "create_sequence", "raw_sql", "todo":
					m.state = stateMigrationInputName
					m.migrationNameInput = ""
					m.selectedTableOrView = ""
					return m, nil
				case "alter_view", "drop_view":
					if len(views) == 0 {
						m.migrationError = i18n.Errf("tui_no_view_available")
						m.state = stateMigrationCreating
						return m, tea.ClearScreen
					}
					m.state = stateMigrationSelectView
					m.viewCursor = 0
					return m, nil
				default:
					if len(tables) == 0 {
						m.migrationError = i18n.Errf("tui_no_table_available")
						m.state = stateMigrationCreating
						return m, tea.ClearScreen
					}
					m.state = stateMigrationSelectTable
					m.tableCursor = 0
					return m, nil
				}
			case "esc":
				m.state = stateMigrationsMenu
				m.cursor = 0
				return m, nil
			}
		}

		if m.state == stateMigrationSelectTable {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				m.tableCursor--
				if m.tableCursor < 0 {
					m.tableCursor = len(m.availableTables) - 1
				}
				return m, nil
			case "down", "j":
				m.tableCursor++
				if m.tableCursor >= len(m.availableTables) {
					m.tableCursor = 0
				}
				return m, nil
			case "enter":
				chosenTable := m.availableTables[m.tableCursor]
				method := m.methodsChoices[m.methodCursor]
				if method == "drop_table" || method == "rename_table" || method == "add_primary_key" || method == "add_check" {
					fileNameDesc := ""
					switch method {
					case "drop_table":
						fileNameDesc = "drop_" + chosenTable
					case "rename_table":
						fileNameDesc = "rename_" + chosenTable
					case "add_primary_key":
						fileNameDesc = "add_pk_to_" + chosenTable
					case "add_check":
						fileNameDesc = "add_check_to_" + chosenTable
					}
					name, err := migraterun.CreateScaffoldMigration(".", m.configData, fileNameDesc, method, chosenTable)
					m.migrationError = err
					m.migrationOutput = name
					m.state = stateMigrationCreating
					return m, tea.ClearScreen
				} else {
					m.state = stateMigrationInputName
					m.migrationNameInput = ""
					m.selectedTableOrView = chosenTable
					return m, nil
				}
			case "esc":
				m.state = stateMigrationSelectMethod
				return m, nil
			}
		}

		if m.state == stateMigrationSelectView {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				m.viewCursor--
				if m.viewCursor < 0 {
					m.viewCursor = len(m.availableViews) - 1
				}
				return m, nil
			case "down", "j":
				m.viewCursor++
				if m.viewCursor >= len(m.availableViews) {
					m.viewCursor = 0
				}
				return m, nil
			case "enter":
				chosenView := m.availableViews[m.viewCursor]
				method := m.methodsChoices[m.methodCursor]
				fileNameDesc := ""
				switch method {
				case "alter_view":
					fileNameDesc = "alter_" + chosenView
				case "drop_view":
					fileNameDesc = "drop_" + chosenView
				}
				name, err := migraterun.CreateScaffoldMigration(".", m.configData, fileNameDesc, method, chosenView)
				m.migrationError = err
				m.migrationOutput = name
				m.state = stateMigrationCreating
				return m, tea.ClearScreen
			case "esc":
				m.state = stateMigrationSelectMethod
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "up", "k":
			if m.state == stateMainMenu {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.choices) - 1
				}
			} else if m.state == stateMigrationsMenu {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.migrationsChoices) - 1
				}
			} else if m.state == stateSeedMenu {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.seedChoices) - 1
				}
			} else if m.state == stateSeedSelectTable {
				m.seedCursor--
				if m.seedCursor < 0 {
					m.seedCursor = len(m.seedTables) - 1
				}
			} else if m.state == stateFactoryMenu {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = len(m.factoryChoices) - 1
				}
			} else if m.state == stateFactorySelectTable {
				m.factoryCursor--
				if m.factoryCursor < 0 {
					m.factoryCursor = len(m.factoryTables) - 1
				}
			}

		case "down", "j":
			if m.state == stateMainMenu {
				m.cursor++
				if m.cursor >= len(m.choices) {
					m.cursor = 0
				}
			} else if m.state == stateMigrationsMenu {
				m.cursor++
				if m.cursor >= len(m.migrationsChoices) {
					m.cursor = 0
				}
			} else if m.state == stateSeedMenu {
				m.cursor++
				if m.cursor >= len(m.seedChoices) {
					m.cursor = 0
				}
			} else if m.state == stateSeedSelectTable {
				m.seedCursor++
				if m.seedCursor >= len(m.seedTables) {
					m.seedCursor = 0
				}
			} else if m.state == stateFactoryMenu {
				m.cursor++
				if m.cursor >= len(m.factoryChoices) {
					m.cursor = 0
				}
			} else if m.state == stateFactorySelectTable {
				m.factoryCursor++
				if m.factoryCursor >= len(m.factoryTables) {
					m.factoryCursor = 0
				}
			}

		case "enter":
			switch m.state {
			case stateMainMenu:
				if m.cursor == m.updateMenuIndex && m.updateMenuIndex >= 0 {
					m.state = stateUpdateConfirm
					return m, tea.ClearScreen
				}
				if m.cursor == m.exitMenuIndex {
					return m, tea.Quit
				}
				if m.configData.ActiveEnv == "production" {
					switch m.cursor {
					case 0:
						m.state = stateMigrationRunning
						m.migrationOutput, m.migrationError = captureOutput(func() error {
							return migraterun.Run(".", m.configData)
						})
						return m, tea.ClearScreen
					}
				} else {
					switch m.cursor {
					case 0:
						m.state = stateReloadRunning
						m.actionRunning = true
						m.spinnerFrame = 0
						m.migrationOutput, m.migrationError = "", nil
						return m, tea.Batch(tea.ClearScreen, runReloadCmd(m.configData), spinnerTick())
					case 1:
						m.state = stateMigrationsMenu
						m.cursor = 0
						return m, tea.ClearScreen
					case 2:
						m.state = stateSeedMenu
						m.cursor = 0
						return m, tea.ClearScreen
					case 3:
						m.state = stateFactoryMenu
						m.cursor = 0
						return m, tea.ClearScreen
					case 4:
						m.state = stateConfigScreen
						m.configData = config.RunConfigChecks()
						m.doctorReport = migraterun.RunDoctor(m.configData)
						return m, tea.ClearScreen
					}
				}
			case stateSeedMenu:
				switch m.cursor {
				case 0:
					tables, err := migraterun.SeedableTables(".", m.configData)
					if err != nil {
						m.state = stateSeedCreating
						m.migrationOutput, m.migrationError = "", err
						return m, tea.ClearScreen
					}
					m.seedTables, m.seedCursor = tables, 0
					m.state = stateSeedSelectTable
					return m, tea.ClearScreen
				case 1:
					m.state = stateSeedCreating
					m.migrationOutput, m.migrationError = captureOutput(func() error {
						return migraterun.SeedValidate(".", m.configData)
					})
					return m, tea.ClearScreen
				case 2:
					m.state = stateSeedCreating
					m.migrationOutput, m.migrationError = captureOutput(func() error {
						return migraterun.SeedRun(".", m.configData, false)
					})
					return m, tea.ClearScreen
				case 3:
					m.state = stateMainMenu
					m.cursor = 0
					return m, tea.ClearScreen
				}
			case stateFactoryMenu:
				switch m.cursor {
				case 0:
					m.state = stateFactoryRunning
					m.migrationOutput, m.migrationError = captureOutput(func() error {
						return migraterun.FactoryCreate(".", m.configData, "")
					})
					return m, tea.ClearScreen
				case 1:
					m.state = stateFactoryRunning
					m.migrationOutput, m.migrationError = captureOutput(func() error {
						return migraterun.FactoryValidate(".", m.configData)
					})
					return m, tea.ClearScreen
				case 2:
					m.state = stateFactoryRunning
					m.migrationOutput, m.migrationError = captureOutput(func() error {
						return migraterun.FactoryRun(".", m.configData, nil)
					})
					return m, tea.ClearScreen
				case 3:
					tables, err := migraterun.FactoryTables(".", m.configData)
					if err != nil {
						m.state = stateFactoryRunning
						m.migrationOutput, m.migrationError = "", err
						return m, tea.ClearScreen
					}
					m.factoryTables, m.factoryCursor = tables, 0
					m.state = stateFactorySelectTable
					return m, tea.ClearScreen
				case 4:
					m.state = stateMainMenu
					m.cursor = 0
					return m, tea.ClearScreen
				}
			case stateFactorySelectTable:
				if len(m.factoryTables) == 0 {
					m.state = stateFactoryMenu
					m.cursor = 0
					return m, tea.ClearScreen
				}
				table := m.factoryTables[m.factoryCursor]
				m.state = stateFactoryRunning
				m.migrationOutput, m.migrationError = captureOutput(func() error {
					return migraterun.FactoryRun(".", m.configData, []string{table})
				})
				return m, tea.ClearScreen
			case stateSeedSelectTable:
				if len(m.seedTables) == 0 {
					m.state = stateSeedMenu
					m.cursor = 0
					return m, tea.ClearScreen
				}
				table := m.seedTables[m.seedCursor]
				m.state = stateSeedCreating
				m.migrationOutput, m.migrationError = captureOutput(func() error {
					path, total, err := migraterun.CreateSeedFile(".", m.configData, table)
					if err != nil {
						return err
					}
					fmt.Printf("Tabela:  %s\n", table)
					fmt.Printf(i18n.T("tui_source"), m.configData.ActiveClient, m.configData.ActiveDialect)
					fmt.Printf(i18n.T("tui_file"), path)
					if total == 0 {
						fmt.Println("\n" + i18n.T("tui_skeleton_created"))
					} else {
						fmt.Printf(i18n.T("tui_rows"), total)
					}
					fmt.Println(i18n.T("tui_seed_next_steps"))
					return nil
				})
				return m, tea.ClearScreen
			case stateMigrationsMenu:
				switch m.cursor {
				case 0:
					m.state = stateMigrationSelectMethod
					m.methodCursor = 0
					return m, nil
				case 1:
					m.state = stateMigrationValidating
					m.migrationOutput, m.migrationError = captureOutput(func() error {
						return migraterun.ValidateReport(".", m.configData)
					})
					return m, tea.ClearScreen
				case 2:
					m.state = stateMigrationRunning
					m.migrationOutput, m.migrationError = captureOutput(func() error {
						return migraterun.Run(".", m.configData)
					})
					return m, tea.ClearScreen
				case 3:
					m.state = stateMigrationRollbackConfirmDelete
					return m, tea.ClearScreen
				case 4:
					m.state = stateMainMenu
					m.cursor = 0
					m.configData = config.RunConfigChecks()
					return m, tea.ClearScreen
				}
			default:
				m.state = stateMainMenu
				m.cursor = 0
				m.configData = config.RunConfigChecks()
				return m, tea.ClearScreen
			}
		}
	}
	return m, nil
}

// renderActionResult mostra a saída real do comando, não só um "deu certo".
// É o que permite conferir o relatório de validação ou as linhas de seed sem
// sair da TUI.
func (m model) renderActionResult(successTitle, failureTitle string) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("✅ "+successTitle) + "\n"
	if m.migrationError != nil {
		title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render("❌ "+failureTitle) + "\n"
	}

	var body strings.Builder
	body.WriteString(title)
	if output := strings.TrimSpace(m.migrationOutput); output != "" {
		body.WriteString("\n" + lastLines(output, 24) + "\n")
	}
	if m.migrationError != nil {
		var userError cliui.UserError
		if errors.As(m.migrationError, &userError) {
			body.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("❌ "+userError.Message) + "\n")
			if solution := strings.TrimSpace(userError.Solution); solution != "" {
				body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C")).Render(i18n.T("tui_solutions")) + "\n")
				for _, line := range strings.Split(solution, "\n") {
					body.WriteString("   • " + line + "\n")
				}
			}
		} else {
			body.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("❌ "+m.migrationError.Error()) + "\n")
		}
	}
	body.WriteString("\n" + i18n.Tf("tui_back_hint", lipgloss.NewStyle().Bold(true).Render("[Enter]")))

	return body.String() + "\n"
}

func getDialectIcon(dialect string) string {
	d := strings.ToLower(strings.TrimSpace(dialect))
	switch d {
	case "postgres", "postgresql":
		return "🐘 PostgreSQL"
	case "oracle":
		return "🔴 Oracle"
	case "mysql":
		return "🐬 MySQL"
	case "sqlserver", "mssql":
		return "🪟 SQL Server"
	default:
		return "💾 " + dialect
	}
}

func getEnvLabel(env string) string {
	e := strings.ToLower(strings.TrimSpace(env))
	if e == "production" || e == "prod" {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render(i18n.T("tui_env_production"))
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render(i18n.T("tui_env_local"))
}

func (m model) statusColor() lipgloss.Color {
	if m.configData.ConfigFileError != nil || m.configData.Config == nil || !m.configData.ConnSuccess {
		return lipgloss.Color("#FF5555")
	}
	if m.updateStatus.Available {
		return lipgloss.Color("#F1FA8C")
	}
	return lipgloss.Color("#50FA7B")
}

func visibleRange(total, cursor, limit int) (int, int) {
	if total <= limit {
		return 0, total
	}
	start := cursor - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > total {
		start = total - limit
	}
	return start, start + limit
}

func renderScrollableList(items []string, cursor, limit int) string {
	if len(items) == 0 {
		return ""
	}
	start, end := visibleRange(len(items), cursor, limit)
	var output strings.Builder
	if start > 0 {
		output.WriteString(itemStyle.Render(i18n.T("tui_more_above")) + "\n")
	}
	for index := start; index < end; index++ {
		if index == cursor {
			output.WriteString(selectedItemStyle.Render("➔ "+items[index]) + "\n")
		} else {
			output.WriteString(itemStyle.Render("  "+items[index]) + "\n")
		}
	}
	if end < len(items) {
		output.WriteString(itemStyle.Render(i18n.T("tui_more_below")) + "\n")
	}
	return output.String()
}

func (m model) navigationListLimit() int {
	limit := m.terminalHeight - 8 // cabeçalho, título, contador e atalhos
	if limit < 3 {
		limit = 3
	}
	if limit > 12 {
		limit = 12
	}
	return limit
}

func (m model) renderHeader() string {
	color := m.statusColor()
	environment := strings.ToUpper(strings.TrimSpace(m.configData.ActiveEnv))
	if environment == "" {
		environment = "ENV?"
	} else if strings.HasPrefix(environment, "DEV") {
		environment = "DEV"
	} else if strings.HasPrefix(environment, "PROD") {
		environment = "PROD"
	}
	database := i18n.T("tui_db_unset")
	if m.configData.Config != nil {
		if connection, ok := m.configData.Config.Connections[m.configData.ActiveClient]; ok {
			database = getDialectIcon(connection.Dialect)
		}
	}
	connectionStatus := i18n.T("tui_status_ok")
	if !m.configData.ConnSuccess {
		connectionStatus = i18n.T("tui_status_fail")
	}
	line := fmt.Sprintf("%s  │  %s  │  %s", environment, database, connectionStatus)
	if m.updateStatus.Available {
		line += "  │  ⚡ " + m.updateStatus.Remote
	}
	var output strings.Builder
	output.WriteString(lipgloss.NewStyle().Bold(true).Foreground(color).Render(line) + "\n")
	output.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render(strings.Repeat("─", 54)) + "\n")
	return output.String()
}

func doctorCheck(ok bool, success, failure string) string {
	if ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✅ " + success)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("❌ " + failure)
}

func (m model) renderDoctorList() string {
	state, report := m.configData, m.doctorReport
	var output strings.Builder
	output.WriteString(lipgloss.NewStyle().Bold(true).Render(i18n.T("tui_doctor_checklist")) + "\n\n")
	output.WriteString(doctorCheck(state.ConfigFileError == nil && state.Config != nil,
		i18n.T("tui_cfg_loaded"), i18n.Tf("tui_cfg_invalid", state.ConfigFileError)) + "\n")

	envOK := false
	envPath := ".env"
	if state.Config != nil {
		envPath = state.Config.Environment.MapperEnv
		_, err := os.Stat(envPath)
		envOK = err == nil && len(state.EnvWarnings) == 0
	}
	envFailure := i18n.T("tui_env_invalid") + envPath
	if len(state.EnvWarnings) > 0 {
		envFailure = i18n.T("tui_env_duplicates") + strings.Join(state.EnvWarnings, ", ")
	}
	output.WriteString(doctorCheck(envOK, i18n.T("tui_env_loaded")+envPath, envFailure) + "\n")
	output.WriteString(doctorCheck(report.ConnSuccess,
		i18n.T("tui_db_connected"), i18n.Tf("tui_conn_failed", report.ConnError)) + "\n")
	if report.ConnSuccess {
		output.WriteString(doctorCheck(report.VersionOK,
			i18n.T("tui_version_ok")+report.Version, i18n.T("tui_version_bad")+report.VersionWarning) + "\n")
		output.WriteString(doctorCheck(report.DDLSuccess,
			i18n.T("tui_ddl_ok"), i18n.Tf("tui_ddl_fail", report.DDLError)) + "\n")
	} else {
		output.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render(i18n.T("tui_no_conn_skipped")) + "\n")
	}
	if m.updateStatus.Error != nil {
		output.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F1FA8C")).Render(i18n.T("tui_update_check_failed")) + "\n")
	} else if m.updateStatus.Available {
		output.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F1FA8C")).Render(i18n.T("tui_update_exec_available")+m.updateStatus.Remote) + "\n")
	} else {
		output.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(i18n.T("tui_exec_current")) + "\n")
	}
	output.WriteString("\n" + footerStyle.Render(i18n.T("tui_nav_back_main")))
	return output.String()
}

// View renderiza a interface no terminal
func (m model) View() string {
	var s strings.Builder
	s.WriteString(m.renderHeader())

	switch m.state {
	case stateMainMenu:
		selectionStyle := lipgloss.NewStyle().Bold(true).Foreground(m.statusColor()).MarginLeft(2)
		for i, choice := range m.choices {
			if m.cursor == i {
				s.WriteString(selectionStyle.Render("➔ "+choice) + "\n")
			} else {
				s.WriteString(itemStyle.Render(choice) + "\n")
			}
		}

	case stateMigrationsMenu:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(i18n.T("tui_mig_options")) + "\n\n")
		for i, choice := range m.migrationsChoices {
			if m.cursor == i {
				s.WriteString(selectedItemStyle.Render("➔ "+choice) + "\n")
			} else {
				s.WriteString(itemStyle.Render(choice) + "\n")
			}
		}

	case stateMigrationInputName:
		method := m.methodsChoices[m.methodCursor]
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf(i18n.T("tui_scaffold_migration"), method)) + "\n\n")

		if m.selectedTableOrView != "" {
			s.WriteString(fmt.Sprintf(i18n.T("tui_selected_table"), lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("alias."+m.selectedTableOrView)))
			s.WriteString(i18n.T("tui_prompt_field_name"))
		} else {
			s.WriteString(i18n.T("tui_prompt_struct_name"))
		}

		inputBoxStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00F0FF")).
			Padding(0, 1).
			Width(45)

		inputText := m.migrationNameInput
		if inputText == "" {
			inputText = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render(i18n.T("tui_input_placeholder"))
		}
		s.WriteString("  " + inputBoxStyle.Render(inputText) + "\n\n")
		s.WriteString(i18n.T("tui_nav_confirm_generate"))

	case stateMigrationSelectMethod:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(i18n.T("tui_pick_operation")) + "\n\n")
		s.WriteString(renderScrollableList(m.methodsChoices, m.methodCursor, m.navigationListLimit()))
		s.WriteString(footerStyle.Render(i18n.Tf("tui_counter", m.methodCursor+1, len(m.methodsChoices))) + "\n")
		s.WriteString(i18n.T("tui_nav_next_options"))

	case stateMigrationSelectTable:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(i18n.T("tui_pick_table")) + "\n\n")

		if len(m.availableTables) == 0 {
			s.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(i18n.T("tui_no_tables")) + "\n")
		} else {
			s.WriteString(renderScrollableList(m.availableTables, m.tableCursor, 12))
			s.WriteString(footerStyle.Render(i18n.Tf("tui_counter", m.tableCursor+1, len(m.availableTables))) + "\n")
		}
		s.WriteString(i18n.T("tui_nav_next_actions"))

	case stateMigrationSelectView:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(i18n.T("tui_pick_view")) + "\n\n")

		if len(m.availableViews) == 0 {
			s.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(i18n.T("tui_no_views")) + "\n")
		} else {
			s.WriteString(renderScrollableList(m.availableViews, m.viewCursor, 12))
			s.WriteString(footerStyle.Render(i18n.Tf("tui_counter", m.viewCursor+1, len(m.availableViews))) + "\n")
		}
		s.WriteString(i18n.T("tui_nav_confirm_actions"))

	case stateMigrationCreating:
		var content string
		borderCol := "#50FA7B" // Verde
		if m.migrationError != nil {
			borderCol = "#FF5555" // Vermelho
			content = fmt.Sprintf(
				i18n.T("tui_mig_create_error")+i18n.T("tui_back_hint"),
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render(i18n.T("tui_tag_error")),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(m.migrationError.Error()),
				lipgloss.NewStyle().Bold(true).Render("[Enter]"),
			)
		} else {
			content = fmt.Sprintf(
				i18n.T("tui_mig_creating")+i18n.T("tui_back_hint"),
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00F0FF")).Render(i18n.T("tui_tag_action")),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(i18n.Tf("tui_mig_created", m.migrationOutput)),
				lipgloss.NewStyle().Bold(true).Render("[Enter]"),
			)
		}
		s.WriteString(actionBoxStyle.Copy().BorderForeground(lipgloss.Color(borderCol)).Render(content) + "\n")

	case stateMigrationRunning:
		s.WriteString(m.renderActionResult(i18n.T("tui_migrations_applied"), i18n.T("tui_migrate_failed")))

	case stateMigrationRollingBack:
		s.WriteString(m.renderActionResult(i18n.T("tui_rollback_done"), i18n.T("tui_rollback_failed")))

	case stateMigrationRollbackConfirmDelete:
		plan, err := migraterun.PlanDevelopmentRollback(".")
		if err != nil {
			s.WriteString(actionBoxStyle.Copy().BorderForeground(lipgloss.Color("#FF5555")).Render(i18n.T("tui_rollback_prepare_failed")+err.Error()+"\n\n[Esc] Voltar") + "\n")
			break
		}
		var files strings.Builder
		for _, path := range plan.Files {
			files.WriteString("  - " + path + "\n")
		}
		s.WriteString(actionBoxStyle.Copy().BorderForeground(lipgloss.Color("#FFB86C")).Render(i18n.T("tui_confirm_delete_files")+files.String()+"\n[Enter/Y] Confirmar  ·  [Esc/N] Cancelar") + "\n")

	case stateMigrationRollbackConfirmFresh:
		s.WriteString(actionBoxStyle.Copy().BorderForeground(lipgloss.Color("#FF5555")).Render(i18n.T("tui_confirm_rebuild_db")) + "\n")

	case stateUpdateConfirm:
		content := fmt.Sprintf(i18n.T("tui_update_available"), m.updateStatus.Local, m.updateStatus.Remote)
		s.WriteString(actionBoxStyle.Copy().BorderForeground(lipgloss.Color("#F1FA8C")).Render(content) + "\n")

	case stateUpdateRunning:
		s.WriteString(m.renderActionResult(i18n.T("tui_update_done"), i18n.T("tui_update_failed")))

	case stateMigrationValidating:
		s.WriteString(m.renderActionResult(i18n.T("tui_corpus_valid"), i18n.T("tui_prevalidation_failed")))

	case stateSeedMenu:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(i18n.T("tui_seed_options")) + "\n\n")
		for i, choice := range m.seedChoices {
			if m.cursor == i {
				s.WriteString(selectedItemStyle.Render("➔ "+choice) + "\n")
			} else {
				s.WriteString(itemStyle.Render(choice) + "\n")
			}
		}
		s.WriteString("\n" + footerStyle.Render(fmt.Sprintf(
			i18n.T("tui_seed_from_db_hint"),
			m.configData.ActiveClient, m.configData.ActiveDialect)) + "\n")

	case stateSeedSelectTable:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(i18n.T("tui_seed_from_db")) + "\n\n")
		if len(m.seedTables) == 0 {
			s.WriteString(i18n.T("tui_no_pk_table"))
			break
		}
		s.WriteString(footerStyle.Render(fmt.Sprintf(i18n.T("tui_reading_from"),
			m.configData.ActiveClient, m.configData.ActiveDialect)) + "\n\n")

		// Janela de 12 itens em volta do cursor: a lista tem centenas de tabelas.
		start := m.seedCursor - 6
		if start < 0 {
			start = 0
		}
		end := start + 12
		if end > len(m.seedTables) {
			end = len(m.seedTables)
			if start = end - 12; start < 0 {
				start = 0
			}
		}
		if start > 0 {
			s.WriteString(itemStyle.Render("↑ ...") + "\n")
		}
		for i := start; i < end; i++ {
			if m.seedCursor == i {
				s.WriteString(selectedItemStyle.Render("➔ "+m.seedTables[i]) + "\n")
			} else {
				s.WriteString(itemStyle.Render(m.seedTables[i]) + "\n")
			}
		}
		if end < len(m.seedTables) {
			s.WriteString(itemStyle.Render("↓ ...") + "\n")
		}
		s.WriteString("\n" + footerStyle.Render(i18n.Tf("tui_footer_seed",
			m.seedCursor+1, len(m.seedTables))) + "\n")

	case stateSeedCreating:
		s.WriteString(m.renderActionResult(i18n.T("tui_done"), i18n.T("tui_seed_failed")))

	case stateFactoryMenu:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(i18n.T("tui_fact_options")) + "\n\n")
		for i, choice := range m.factoryChoices {
			if m.cursor == i {
				s.WriteString(selectedItemStyle.Render("➔ "+choice) + "\n")
			} else {
				s.WriteString(itemStyle.Render(choice) + "\n")
			}
		}
		s.WriteString("\n" + footerStyle.Render(fmt.Sprintf(
			i18n.T("tui_factory_truncates"),
			m.configData.ActiveClient, m.configData.ActiveDialect)) + "\n")

	case stateFactorySelectTable:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(i18n.T("tui_populate_one")) + "\n\n")
		if len(m.factoryTables) == 0 {
			s.WriteString(i18n.T("tui_no_factory"))
			break
		}
		s.WriteString(footerStyle.Render(i18n.T("tui_factory_deps")) + "\n\n")

		// Janela de 12 itens em volta do cursor: a lista tem centenas de tabelas.
		start := m.factoryCursor - 6
		if start < 0 {
			start = 0
		}
		end := start + 12
		if end > len(m.factoryTables) {
			end = len(m.factoryTables)
			if start = end - 12; start < 0 {
				start = 0
			}
		}
		if start > 0 {
			s.WriteString(itemStyle.Render("↑ ...") + "\n")
		}
		for i := start; i < end; i++ {
			if m.factoryCursor == i {
				s.WriteString(selectedItemStyle.Render("➔ "+m.factoryTables[i]) + "\n")
			} else {
				s.WriteString(itemStyle.Render(m.factoryTables[i]) + "\n")
			}
		}
		if end < len(m.factoryTables) {
			s.WriteString(itemStyle.Render("↓ ...") + "\n")
		}
		s.WriteString("\n" + footerStyle.Render(i18n.Tf("tui_footer_factory",
			m.factoryCursor+1, len(m.factoryTables))) + "\n")

	case stateFactoryRunning:
		s.WriteString(m.renderActionResult(i18n.T("tui_done"), i18n.T("tui_factory_failed")))

	case stateReloadRunning:
		if m.actionRunning {
			frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			frame := frames[m.spinnerFrame%len(frames)]
			s.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F1FA8C")).Render(frame+i18n.T("tui_reload_running")) + "\n")
			s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render(i18n.T("tui_reload_wait")) + "\n")
		} else {
			s.WriteString(m.renderActionResult(i18n.T("tui_reload_done"), i18n.T("tui_reload_failed")))
		}

	case stateConfigScreen:
		s.WriteString(m.renderDoctorList() + "\n")
	}
	return s.String()
}
