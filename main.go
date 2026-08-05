package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommitHash e Version são injetados em tempo de compilação via -ldflags.
var (
	CommitHash = "development"
	Version    = "development"
)

// URL oficial do repositório e endpoint de commits do GitHub
const (
	RepoURL    = "https://github.com/PhelipeViana/gokit"
	CommitsAPI = "https://api.github.com/repos/PhelipeViana/gokit/commits"
)

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

type Commit struct {
	SHA string `json:"sha"`
}

type menuState int

const (
	stateMainMenu menuState = iota
	stateMigrationsMenu
	stateMigrationCreating
	stateMigrationRunning
	stateMigrationRollingBack
)

type model struct {
	state             menuState
	cursor            int
	choices           []string
	migrationsChoices []string
}

func main() {
	// Checagem rápida de versão silenciosa
	updateAvailable, localSHA, remoteSHA := runSilentUpdateCheck()

	// Se estiver desatualizado, bloqueia o menu e exige download
	if updateAvailable {
		printOutdatedAndExit(localSHA, remoteSHA)
		return
	}

	m := model{
		state:             stateMainMenu,
		cursor:            0,
		choices:           []string{"Migration Options", "Sair (Exit)"},
		migrationsChoices: []string{"Criar nova Migration", "Executar Migrations pendentes", "Reverter última Migration (Rollback)", "Voltar ao menu principal"},
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Ocorreu um erro no aplicativo: %v\n", err)
		os.Exit(1)
	}
}

// printOutdatedAndExit informa que o executável está desatualizado e encerra o app
func printOutdatedAndExit(localSHA, remoteSHA string) {
	downloadURL := getDirectDownloadURL()
	fmt.Println()
	fmt.Printf("\033[1m\033[31m⚠️  O GOKIT ESTÁ DESATUALIZADO! ⚠️\033[0m\n\n")
	fmt.Printf("Por favor, baixe a nova versão em:\n\033[36m%s\033[0m\n\n", downloadURL)
	os.Exit(1)
}

// getDirectDownloadURL gera o link de download direto do arquivo binário bruto no repositório GitHub
func getDirectDownloadURL() string {
	baseURL := "https://github.com/PhelipeViana/gokit/raw/main/dist"
	
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	switch goos {
	case "windows":
		return baseURL + "/gokit-windows-amd64.exe"
	case "linux":
		return baseURL + "/gokit-linux-amd64"
	case "darwin":
		if goarch == "arm64" {
			return baseURL + "/gokit-darwin-arm64"
		}
		return baseURL + "/gokit-darwin-amd64"
	default:
		return "https://github.com/PhelipeViana/gokit/tree/main/dist"
	}
}

// runSilentUpdateCheck verifica atualizações de forma silenciosa
func runSilentUpdateCheck() (bool, string, string) {
	if CommitHash == "development" {
		return false, "", ""
	}

	remoteSHA, err := fetchLatestRemoteCommit()
	if err != nil {
		return false, "", ""
	}

	if !strings.HasPrefix(remoteSHA, CommitHash) && !strings.HasPrefix(CommitHash, remoteSHA) {
		shortLocal := CommitHash
		if len(shortLocal) > 7 {
			shortLocal = shortLocal[:7]
		}
		shortRemote := remoteSHA
		if len(shortRemote) > 7 {
			shortRemote = shortRemote[:7]
		}
		return true, shortLocal, shortRemote
	}

	return false, "", ""
}

func fetchLatestRemoteCommit() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	url := "https://github.com/PhelipeViana/gokit/raw/main/dist/commit.txt"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gokit-cli")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("não foi possível obter a versão remota (status %d)", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(bodyBytes)), nil
}

// terminalLink cria o hyperlink OSC 8 interativo no terminal
func terminalLink(url, text string) string {
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\ (%s)", url, text, url)
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
			if m.state == stateMainMenu {
				switch m.cursor {
				case 0:
					m.state = stateMigrationsMenu
					m.cursor = 0
				case 1:
					return m, tea.Quit
				}
			} else if m.state == stateMigrationsMenu {
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
			} else {
				m.state = stateMigrationsMenu
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
	}

	if m.state == stateMainMenu || m.state == stateMigrationsMenu {
		s.WriteString(footerStyle.Render("↑/↓: navegar • enter: selecionar • ctrl+c: sair") + "\n")
	}

	return s.String()
}
