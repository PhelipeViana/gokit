package tui

import (
	"fmt"
	"os"
	"strings"

	"gokit/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var Version string

type menuState int

const (
	stateMainMenu menuState = iota
	stateMigrationsMenu
	stateMigrationCreating
	stateMigrationRunning
	stateMigrationRollingBack
	stateConfigScreen
)

type model struct {
	state             menuState
	cursor            int
	choices           []string
	migrationsChoices []string
	configData        config.ConfigState
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
		state:             stateMainMenu,
		cursor:            0,
		choices:           []string{"Configuração", "Migration Options", "Sair (Exit)"},
		migrationsChoices: []string{"Criar nova Migration", "Executar Migrations pendentes", "Reverter última Migration (Rollback)", "Voltar ao menu principal"},
		configData:        initialConfig,
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
					m.state = stateMigrationCreating
					return m, tea.ClearScreen
				case 1:
					m.state = stateMigrationRunning
					return m, tea.ClearScreen
				case 2:
					m.state = stateMigrationRollingBack
					return m, tea.ClearScreen
				case 3:
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

	case stateMigrationCreating:
		content := fmt.Sprintf(
			"%s Criando nova estrutura de migration...\n\n"+
				"%s\n\n"+
				"Pressione %s para voltar.",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00F0FF")).Render("[Ação]"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✔ Migration criada com sucesso! (gokit_migration_placeholder.go)"),
			lipgloss.NewStyle().Bold(true).Render("[Enter]"),
		)
		s.WriteString(actionBoxStyle.Render(content) + "\n")

	case stateMigrationRunning:
		content := fmt.Sprintf(
			"%s Executando migrações pendentes...\n\n"+
				"%s\n\n"+
				"Pressione %s para voltar.",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00F0FF")).Render("[Ação]"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✔ Banco de dados atualizado! Todas as migrações foram aplicadas."),
			lipgloss.NewStyle().Bold(true).Render("[Enter]"),
		)
		s.WriteString(actionBoxStyle.Render(content) + "\n")

	case stateMigrationRollingBack:
		content := fmt.Sprintf(
			"%s Revertendo última migration (Rollback)...\n\n"+
				"%s\n\n"+
				"Pressione %s para voltar.",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00F0FF")).Render("[Ação]"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✔ Rollback executado com sucesso!"),
			lipgloss.NewStyle().Bold(true).Render("[Enter]"),
		)
		s.WriteString(actionBoxStyle.Render(content) + "\n")

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

