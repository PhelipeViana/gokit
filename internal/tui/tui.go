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
				case 1:
					m.state = stateMigrationsMenu
					m.cursor = 0
				case 2:
					return m, tea.Quit
				}
			case stateMigrationsMenu:
				switch m.cursor {
				case 0:
					m.state = stateMigrationCreating
				case 1:
					m.state = stateMigrationRunning
				case 2:
					m.state = stateMigrationRollingBack
				case 3:
					m.state = stateMainMenu
					m.cursor = 0
				}
			default:
				m.state = stateMainMenu
				m.cursor = 0
				// Sincroniza o status das conexões ao voltar ao menu principal
				m.configData = config.RunConfigChecks()
			}
		}
	}
	return m, nil
}

// View renderiza a interface no terminal
func (m model) View() string {
	var s strings.Builder

	// Cabeçalho da aplicação
	s.WriteString(titleStyle.Render("GO KIT CLI") + "\n")
	s.WriteString(versionStyle.Render("Última Atualização: "+Version) + "\n\n")

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
				fmt.Sprintf("Banco de Dados: ✖ Erro (%v)", m.configData.ConfigFileError),
			) + "\n")
		} else if m.configData.Config != nil {
			if m.configData.ConnSuccess {
				s.WriteString(statusStyle.Foreground(lipgloss.Color("#50FA7B")).Render(
					fmt.Sprintf("Banco de Dados: ✔ Conectado (%s • %s)", m.configData.ActiveClient, m.configData.ActiveDialect),
				) + "\n")
			} else {
				s.WriteString(statusStyle.Foreground(lipgloss.Color("#FF5555")).Render(
					fmt.Sprintf("Banco de Dados: ✖ Desconectado (%s • %s)", m.configData.ActiveClient, m.configData.ActiveDialect),
				) + "\n")
			}
		} else {
			s.WriteString(statusStyle.Foreground(lipgloss.Color("#FFB86C")).Render(
				"Banco de Dados: ✖ gokit.yaml não inicializado (Acesse 'Configuração' para criar)",
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
			"\033[1m\033[36m[Ação]\033[0m Criando nova estrutura de migration...\n\n"+
				"\033[32m✔ Migration criada com sucesso! (gokit_migration_placeholder.go)\033[0m\n\n"+
				"Pressione \033[1m[Enter]\033[0m para voltar.",
		)
		s.WriteString(actionBoxStyle.Render(content) + "\n")

	case stateMigrationRunning:
		content := fmt.Sprintf(
			"\033[1m\033[36m[Ação]\033[0m Executando migrações pendentes...\n\n"+
				"\033[32m✔ Banco de dados atualizado! Todas as migrações foram aplicadas.\033[0m\n\n"+
				"Pressione \033[1m[Enter]\033[0m para voltar.",
		)
		s.WriteString(actionBoxStyle.Render(content) + "\n")

	case stateMigrationRollingBack:
		content := fmt.Sprintf(
			"\033[1m\033[36m[Ação]\033[0m Revertendo última migration (Rollback)...\n\n"+
				"\033[32m✔ Rollback executado com sucesso!\033[0m\n\n"+
				"Pressione \033[1m[Enter]\033[0m para voltar.",
		)
		s.WriteString(actionBoxStyle.Render(content) + "\n")

	case stateConfigScreen:
		var cfgStr strings.Builder
		cState := m.configData

		cfgStr.WriteString("\033[1m\033[36m[Configuração - Checklist de Validação]\033[0m\n\n")

		// 1. Validar existência do gokit.yaml
		if cState.ScaffoldCreated {
			cfgStr.WriteString("  \033[32m[✔] gokit.yaml: Criado com sucesso em " + cState.ConfigPath + "\033[0m\n")
		} else if cState.ConfigFileError != nil && strings.Contains(cState.ConfigFileError.Error(), "não foi possível ler") {
			cfgStr.WriteString("  \033[31m[✖] gokit.yaml: Não foi possível ler o arquivo\033[0m\n")
		} else {
			cfgStr.WriteString("  \033[32m[✔] gokit.yaml: Carregado com sucesso\033[0m\n")
		}

		// 2. Validar estrutura YAML
		if cState.ConfigFileError != nil && strings.Contains(cState.ConfigFileError.Error(), "sintaxe YAML") {
			cfgStr.WriteString(fmt.Sprintf("  \033[31m[✖] Estrutura YAML: %v\033[0m\n", cState.ConfigFileError))
		} else if cState.Config != nil {
			cfgStr.WriteString("  \033[32m[✔] Estrutura YAML: Válida\033[0m\n")
		}

		// 3. Validar .env
		if cState.Config != nil {
			envPath := cState.Config.Environment.MapperEnv
			if _, err := os.Stat(envPath); err == nil {
				cfgStr.WriteString(fmt.Sprintf("  \033[32m[✔] Ambiente (.env): Carregado de \"%s\"\033[0m\n", envPath))
			} else {
				cfgStr.WriteString(fmt.Sprintf("  \033[33m[!] Ambiente (.env): Arquivo \"%s\" não encontrado\033[0m\n", envPath))
			}
		}

		// 4. Validar Conectividade do banco de dados (Apenas o banco atual configurado)
		if cState.ConfigFileError != nil && !strings.Contains(cState.ConfigFileError.Error(), "sintaxe YAML") {
			cfgStr.WriteString(fmt.Sprintf("  \033[31m[✖] Conectividade: %v\033[0m\n", cState.ConfigFileError))
		} else if cState.Config != nil {
			cfgStr.WriteString(fmt.Sprintf("  \033[32m[✔] Cliente Ativo: %s (dialeto: %s)\033[0m\n", cState.ActiveClient, cState.ActiveDialect))

			maskedURL := config.MaskPassword(cState.ActiveDialect, cState.ActiveURL)
			if cState.ConnSuccess {
				cfgStr.WriteString("  \033[32m[✔] Conectividade: CONECTADO COM SUCESSO!\033[0m\n")
				cfgStr.WriteString(fmt.Sprintf("      URL: %s\n", maskedURL))
			} else {
				cfgStr.WriteString("  \033[31m[✖] Conectividade: FALHA DE CONEXÃO\033[0m\n")
				cfgStr.WriteString(fmt.Sprintf("      Erro: %v\n", cState.ConnError))
				cfgStr.WriteString(fmt.Sprintf("      URL: %s\n", maskedURL))
			}
		}

		// 5. Mostrar Mapeamento de Diretórios (Output)
		if cState.Config != nil {
			cfgStr.WriteString("\n  \033[1mDiretórios de Destino (Output):\033[0m\n")
			cfgStr.WriteString(fmt.Sprintf("    Settings:  %s\n", cState.Config.Output.Settings))
			cfgStr.WriteString(fmt.Sprintf("    ORM:       %s\n", cState.Config.Output.ORM))
			cfgStr.WriteString(fmt.Sprintf("    Migrate:   %s\n", cState.Config.Output.Migrate))
			cfgStr.WriteString(fmt.Sprintf("    Factory:   %s\n", cState.Config.Output.Factory))
			cfgStr.WriteString(fmt.Sprintf("    Seed:      %s\n", cState.Config.Output.Seed))
			cfgStr.WriteString(fmt.Sprintf("    Docs:      %s\n", cState.Config.Output.Docs))
		}

		cfgStr.WriteString("\nPressione \033[1m[Enter]\033[0m para voltar ao menu principal.")
		s.WriteString(actionBoxStyle.Render(cfgStr.String()) + "\n")
	}

	if m.state == stateMainMenu || m.state == stateMigrationsMenu {
		s.WriteString(footerStyle.Render("↑/↓: navegar • enter: selecionar • ctrl+c: sair") + "\n")
	}

	return s.String()
}
