package tui

import (
	"fmt"
	"os"
	"strings"

	"gokit/internal/config"
	"gokit/internal/migraterun"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var Version string

type menuState int

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
	stateMigrationValidating
	stateConfigScreen
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
	return fmt.Sprintf("... (%d linha(s) acima omitidas)\n%s", hidden, strings.Join(lines[hidden:], "\n"))
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
func Start(version string) error {
	Version = version

	// Carrega dados iniciais de configuração para expor o status no menu
	initialConfig := config.RunConfigChecks()

	m := model{
		state:   stateMainMenu,
		cursor:  0,
		choices: []string{"Configuração", "Migration Options", "Sair (Exit)"},
		migrationsChoices: []string{
			"Criar nova Migration",
			"Validar Migrations (não toca no banco)",
			"Executar Migrations pendentes",
			"Reverter última Migration (Rollback)",
			"Voltar ao menu principal",
		},
		configData: initialConfig,
		methodsChoices: []string{
			"create_table", "drop_table", "add_column", "alter_column", "drop_column",
			"add_foreign_key", "drop_foreign_key", "create_index", "drop_index",
			"create_view", "alter_view", "drop_view", "create_sequence", "drop_sequence",
			"rename_table", "rename_column", "add_primary_key", "add_unique", "add_check",
			"drop_constraint", "raw_sql", "todo",
		},
	}

	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

// Init inicializa o modelo do Bubble Tea
func (m model) Init() tea.Cmd {
	return nil
}

// Update gerencia as interações do Bubble Tea
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
						m.migrationError = fmt.Errorf("nenhuma view disponível no catálogo. Crie uma view primeiro")
						m.state = stateMigrationCreating
						return m, tea.ClearScreen
					}
					m.state = stateMigrationSelectView
					m.viewCursor = 0
					return m, nil
				default:
					if len(tables) == 0 {
						m.migrationError = fmt.Errorf("nenhuma tabela disponível no catálogo. Crie uma tabela usando CreateTable primeiro")
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
			}

		case "enter":
			switch m.state {
			case stateMainMenu:
				switch m.cursor {
				case 0:
					m.state = stateConfigScreen
					m.configData = config.RunConfigChecks()
					return m, tea.ClearScreen
				case 1:
					m.state = stateMigrationsMenu
					m.cursor = 0
					return m, tea.ClearScreen
				case 2:
					return m, tea.Quit
				}
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
					m.state = stateMigrationRollingBack
					m.migrationOutput, m.migrationError = captureOutput(func() error {
						return migraterun.Rollback(".", m.configData)
					})
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
	borderCol := "#50FA7B"
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("✔ "+successTitle) + "\n"
	if m.migrationError != nil {
		borderCol = "#FF5555"
		title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render("✖ "+failureTitle) + "\n"
	}

	var body strings.Builder
	body.WriteString(title)
	if output := strings.TrimSpace(m.migrationOutput); output != "" {
		body.WriteString("\n" + lastLines(output, 24) + "\n")
	}
	if m.migrationError != nil {
		body.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(m.migrationError.Error()) + "\n")
	}
	body.WriteString("\nPressione " + lipgloss.NewStyle().Bold(true).Render("[Enter]") + " para voltar.")

	return actionBoxStyle.Copy().BorderForeground(lipgloss.Color(borderCol)).Render(body.String()) + "\n"
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
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render("🚨 PRODUÇÃO (production)")
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("💻 LOCAL (development)")
}

// View renderiza a interface no terminal
func (m model) View() string {
	var s strings.Builder

	// Cabeçalho da aplicação - Apenas impresso no menu principal e submenus de navegação
	if m.state == stateMainMenu || m.state == stateMigrationsMenu {
		s.WriteString(titleStyle.Render("GO KIT CLI") + "\n")
		s.WriteString(versionStyle.Render("Última Atualização: "+Version) + "\n\n")
	}

	switch m.state {
	case stateMainMenu:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Menu Principal:") + "\n\n")
		for i, choice := range m.choices {
			if m.cursor == i {
				s.WriteString(selectedItemStyle.Render("➔ "+choice) + "\n")
			} else {
				s.WriteString(itemStyle.Render(choice) + "\n")
			}
		}

		// Exibe o status da conexão atual de forma clara no Menu Principal
		s.WriteString("\n")
		statusStyle := lipgloss.NewStyle().MarginLeft(2)
		if m.configData.ConfigFileError != nil {
			s.WriteString(statusStyle.Foreground(lipgloss.Color("#FF5555")).Render(
				fmt.Sprintf("Ambiente:       %s\n  Banco de Dados: ✖ Erro de Configuração (%v)",
					getEnvLabel(m.configData.ActiveEnv), m.configData.ConfigFileError),
			) + "\n")
		} else if m.configData.Config != nil {
			envLabel := getEnvLabel(m.configData.ActiveEnv)
			dialectIcon := getDialectIcon(m.configData.ActiveDialect)
			if m.configData.ConnSuccess {
				s.WriteString(statusStyle.Render(
					fmt.Sprintf("Ambiente:       %s\n  Banco de Dados: %s (%s)\n  Status:         %s",
						envLabel,
						dialectIcon,
						m.configData.ActiveClient,
						lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("✔ Conectado"),
					),
				) + "\n")
			} else {
				s.WriteString(statusStyle.Render(
					fmt.Sprintf("Ambiente:       %s\n  Banco de Dados: %s (%s)\n  Status:         %s",
						envLabel,
						dialectIcon,
						m.configData.ActiveClient,
						lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render("✖ Desconectado"),
					),
				) + "\n")
			}
		} else {
			s.WriteString(statusStyle.Foreground(lipgloss.Color("#FFB86C")).Render(
				"Banco de Dados: ✖ gokit.json não inicializado (Acesse 'Configuração' para criar)",
			) + "\n")
		}

	case stateMigrationsMenu:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Opções de Migração:") + "\n\n")
		for i, choice := range m.migrationsChoices {
			if m.cursor == i {
				s.WriteString(selectedItemStyle.Render("➔ "+choice) + "\n")
			} else {
				s.WriteString(itemStyle.Render(choice) + "\n")
			}
		}

	case stateMigrationInputName:
		method := m.methodsChoices[m.methodCursor]
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Scaffold Migration (%s)", method)) + "\n\n")

		if m.selectedTableOrView != "" {
			s.WriteString(fmt.Sprintf("  Tabela selecionada: %s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("alias."+m.selectedTableOrView)))
			s.WriteString("  Digite o nome descritivo do campo/constraint (ex: email ou chk_users_age):\n")
		} else {
			s.WriteString("  Digite o nome descritivo da nova estrutura (ex: users ou active_users):\n")
		}

		inputBoxStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00F0FF")).
			Padding(0, 1).
			Width(45)

		inputText := m.migrationNameInput
		if inputText == "" {
			inputText = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render("digite aqui...")
		}
		s.WriteString("  " + inputBoxStyle.Render(inputText) + "\n\n")
		s.WriteString("  [Enter] Confirmar e Gerar  ·  [Esc] Voltar\n")

	case stateMigrationSelectMethod:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Selecione o Tipo de Operação:") + "\n\n")

		colSelectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF007F"))
		colItemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F8F8F2"))

		half := (len(m.methodsChoices) + 1) / 2
		for i := 0; i < half; i++ {
			idx1 := i
			col1 := ""
			if m.methodCursor == idx1 {
				col1 = colSelectionStyle.Render(fmt.Sprintf("➔ %-20s", m.methodsChoices[idx1]))
			} else {
				col1 = colItemStyle.Render(fmt.Sprintf("  %-20s", m.methodsChoices[idx1]))
			}

			idx2 := i + half
			col2 := ""
			if idx2 < len(m.methodsChoices) {
				if m.methodCursor == idx2 {
					col2 = colSelectionStyle.Render(fmt.Sprintf("➔ %-20s", m.methodsChoices[idx2]))
				} else {
					col2 = colItemStyle.Render(fmt.Sprintf("  %-20s", m.methodsChoices[idx2]))
				}
			}

			s.WriteString("  " + col1 + "    " + col2 + "\n")
		}
		s.WriteString("\n  [Enter] Avançar  ·  [Esc] Voltar para Opções\n")

	case stateMigrationSelectTable:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Selecione a Tabela no Catálogo:") + "\n\n")

		colSelectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF007F"))
		colItemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F8F8F2"))

		half := (len(m.availableTables) + 1) / 2
		if half == 0 {
			s.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("Nenhuma tabela encontrada no catálogo.") + "\n")
		} else {
			for i := 0; i < half; i++ {
				idx1 := i
				col1 := ""
				if m.tableCursor == idx1 {
					col1 = colSelectionStyle.Render(fmt.Sprintf("➔ %-20s", m.availableTables[idx1]))
				} else {
					col1 = colItemStyle.Render(fmt.Sprintf("  %-20s", m.availableTables[idx1]))
				}

				idx2 := i + half
				col2 := ""
				if idx2 < len(m.availableTables) {
					if m.tableCursor == idx2 {
						col2 = colSelectionStyle.Render(fmt.Sprintf("➔ %-20s", m.availableTables[idx2]))
					} else {
						col2 = colItemStyle.Render(fmt.Sprintf("  %-20s", m.availableTables[idx2]))
					}
				}

				s.WriteString("  " + col1 + "    " + col2 + "\n")
			}
		}
		s.WriteString("\n  [Enter] Avançar  ·  [Esc] Voltar para Ações\n")

	case stateMigrationSelectView:
		s.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Selecione a View no Catálogo:") + "\n\n")

		colSelectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF007F"))
		colItemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F8F8F2"))

		half := (len(m.availableViews) + 1) / 2
		if half == 0 {
			s.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("Nenhuma view encontrada no catálogo.") + "\n")
		} else {
			for i := 0; i < half; i++ {
				idx1 := i
				col1 := ""
				if m.viewCursor == idx1 {
					col1 = colSelectionStyle.Render(fmt.Sprintf("➔ %-20s", m.availableViews[idx1]))
				} else {
					col1 = colItemStyle.Render(fmt.Sprintf("  %-20s", m.availableViews[idx1]))
				}

				idx2 := i + half
				col2 := ""
				if idx2 < len(m.availableViews) {
					if m.viewCursor == idx2 {
						col2 = colSelectionStyle.Render(fmt.Sprintf("➔ %-20s", m.availableViews[idx2]))
					} else {
						col2 = colItemStyle.Render(fmt.Sprintf("  %-20s", m.availableViews[idx2]))
					}
				}

				s.WriteString("  " + col1 + "    " + col2 + "\n")
			}
		}
		s.WriteString("\n  [Enter] Confirmar e Gerar  ·  [Esc] Voltar para Ações\n")

	case stateMigrationCreating:
		var content string
		borderCol := "#50FA7B" // Verde
		if m.migrationError != nil {
			borderCol = "#FF5555" // Vermelho
			content = fmt.Sprintf(
				"%s Erro ao criar migration:\n\n"+
					"%s\n\n"+
					"Pressione %s para voltar.",
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render("[Erro]"),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(m.migrationError.Error()),
				lipgloss.NewStyle().Bold(true).Render("[Enter]"),
			)
		} else {
			content = fmt.Sprintf(
				"%s Criando nova estrutura de migration...\n\n"+
					"%s\n\n"+
					"Pressione %s para voltar.",
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00F0FF")).Render("[Ação]"),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(fmt.Sprintf("✔ Migration criada com sucesso: %s", m.migrationOutput)),
				lipgloss.NewStyle().Bold(true).Render("[Enter]"),
			)
		}
		s.WriteString(actionBoxStyle.Copy().BorderForeground(lipgloss.Color(borderCol)).Render(content) + "\n")

	case stateMigrationRunning:
		s.WriteString(m.renderActionResult("Migrations aplicadas.", "Erro ao executar migrações:"))

	case stateMigrationRollingBack:
		s.WriteString(m.renderActionResult("Rollback executado.", "Erro ao reverter última migration:"))

	case stateMigrationValidating:
		s.WriteString(m.renderActionResult("Corpus válido.", "A pré-validação encontrou problemas:"))

	case stateConfigScreen:
		var cfgStr strings.Builder
		cState := m.configData

		cfgStr.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00F0FF")).Render("[Configuração - Checklist de Validação]") + "\n\n")

		// 1. Validar Conectividade do banco de dados (Sempre no topo e simplificado, apenas diz qual cliente e dialeto)
		if cState.ConfigFileError != nil && !strings.Contains(cState.ConfigFileError.Error(), "sintaxe YAML") {
			cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(
				fmt.Sprintf("  [✖] Conectividade: Erro ao ler conexões (%v)", cState.ConfigFileError),
			) + "\n")
		} else if cState.Config != nil {
			dialectIcon := getDialectIcon(cState.ActiveDialect)
			envLabel := getEnvLabel(cState.ActiveEnv)
			if cState.ConnSuccess {
				cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(
					fmt.Sprintf("  [✔] Conectividade: Conectado ao %s (%s) em %s", dialectIcon, cState.ActiveClient, envLabel),
				) + "\n")
			} else {
				cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(
					fmt.Sprintf("  [✖] Conectividade: Desconectado de %s (%s) em %s - Erro: %v", dialectIcon, cState.ActiveClient, envLabel, cState.ConnError),
				) + "\n")
			}
		}

		// 2. Validar existência do gokit.json
		if cState.ScaffoldCreated {
			cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(
				"  [✔] gokit.json: Criado com sucesso em "+cState.ConfigPath,
			) + "\n")
		} else if cState.ConfigFileError != nil && strings.Contains(cState.ConfigFileError.Error(), "não foi possível ler") {
			cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(
				"  [✖] gokit.json: Não foi possível ler o arquivo",
			) + "\n")
		} else {
			cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(
				"  [✔] gokit.json: Carregado com sucesso",
			) + "\n")
		}

		// 3. Validar estrutura JSON
		if cState.ConfigFileError != nil && strings.Contains(cState.ConfigFileError.Error(), "sintaxe JSON") {
			cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(
				fmt.Sprintf("  [✖] Estrutura JSON: %v", cState.ConfigFileError),
			) + "\n")
		} else if cState.Config != nil {
			cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(
				"  [✔] Estrutura JSON: Válida",
			) + "\n")
		}

		// 4. Validar .env
		if cState.Config != nil {
			envPath := cState.Config.Environment.MapperEnv
			if _, err := os.Stat(envPath); err == nil {
				if len(cState.EnvWarnings) > 0 {
					cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C")).Render(
						fmt.Sprintf("  [!] Ambiente (.env): Carregado, mas contém variáveis duplicadas (sobrescritas): %s", strings.Join(cState.EnvWarnings, ", ")),
					) + "\n")
				} else {
					cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(
						fmt.Sprintf("  [✔] Ambiente (.env): Carregado de \"%s\"", envPath),
					) + "\n")
				}
			} else {
				cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C")).Render(
					fmt.Sprintf("  [!] Ambiente (.env): Arquivo \"%s\" não encontrado", envPath),
				) + "\n")
			}
		}

		// 5. Validar Notificações via Slack
		if cState.Config != nil {
			slackConf := cState.Config.Notifications.Slack
			if slackConf.Enabled {
				if slackConf.WebhookURL != "" && slackConf.WebhookURL != "${SLACK_WEBHOOK_URL}" {
					cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(
						"  [✔] Notificações Slack: Ativas (Webhook configurado)",
					) + "\n")
				} else {
					cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(
						"  [✖] Notificações Slack: Habilitadas, mas sem webhook_url válido",
					) + "\n")
				}
			} else {
				cfgStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4")).Render(
					"  [✔] Notificações Slack: Desativadas",
				) + "\n")
			}
		}

		// 6. Mostrar Mapeamento de Diretórios (Output)
		if cState.Config != nil {
			cfgStr.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render("Diretórios de Destino (Output):") + "\n")
			cfgStr.WriteString(fmt.Sprintf("    Settings:  %s\n", cState.Config.Output.Settings))
			cfgStr.WriteString(fmt.Sprintf("    ORM:       %s\n", cState.Config.Output.ORM))
			cfgStr.WriteString(fmt.Sprintf("    Migrate:   %s\n", cState.Config.Output.Migrate))
			cfgStr.WriteString(fmt.Sprintf("    Factory:   %s\n", cState.Config.Output.Factory))
			cfgStr.WriteString(fmt.Sprintf("    Seed:      %s\n", cState.Config.Output.Seed))
			cfgStr.WriteString(fmt.Sprintf("    Docs:      %s\n", cState.Config.Output.Docs))
		}

		cfgStr.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render("Pressione [Enter] para voltar ao menu principal."))

		boxColor := "#50FA7B" // Verde
		if cState.ConfigFileError != nil || !cState.ConnSuccess {
			boxColor = "#FF5555" // Vermelho
		}
		currentBoxStyle := actionBoxStyle.Copy().BorderForeground(lipgloss.Color(boxColor))
		s.WriteString(currentBoxStyle.Render(cfgStr.String()) + "\n")
	}
	return s.String()
}
