package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

// URL oficial do repositório
const RepoURL = "https://github.com/PhelipeViana/gokit"

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
	// Remove binários antigos remanescentes de atualizações anteriores (.old)
	cleanOldExecutables()

	// Checagem rápida e silenciosa de versão no GitHub
	updateAvailable, _, remoteSHA := runSilentUpdateCheck()

	// Se houver nova versão disponível, atualiza e reinicia automaticamente em background
	if updateAvailable {
		fmt.Printf("\n\033[1m\033[36m⚡ Nova versão detectada (%s). Atualizando gokit automaticamente...\033[0m\n", remoteSHA[:7])
		err := runSelfUpdate()
		if err != nil {
			fmt.Printf("\033[31m✖ Erro ao atualizar automaticamente: %v. Continuando com a versão atual...\033[0m\n", err)
			time.Sleep(2 * time.Second)
		} else {
			fmt.Println("\033[32m✔ Atualizado com sucesso! Reiniciando...\033[0m")
			time.Sleep(500 * time.Millisecond)

			// Reinicia o processo atual
			err = restartProcess()
			if err != nil {
				fmt.Printf("\033[31m✖ Falha ao reiniciar: %v. Por favor, execute novamente.\033[0m\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	m := model{
		state:             stateMainMenu,
		cursor:            0,
		choices:           []string{"PRIMEIRO MENU", "Sair (Exit)"},
		migrationsChoices: []string{"Criar nova Migration", "Executar Migrations pendentes", "Reverter última Migration (Rollback)", "Voltar ao menu principal"},
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Ocorreu um erro no aplicativo: %v\n", err)
		os.Exit(1)
	}
}

// cleanOldExecutables deleta arquivos temporários .old gerados no auto-update.
// Executado em goroutine com retentativas para dar tempo do processo pai fechar.
func cleanOldExecutables() {
	execPath, err := os.Executable()
	if err == nil {
		oldPath := execPath + ".old"
		if _, err := os.Stat(oldPath); err == nil {
			go func() {
				for i := 0; i < 5; i++ {
					time.Sleep(500 * time.Millisecond)
					err := os.Remove(oldPath)
					if err == nil {
						break
					}
				}
			}()
		}
	}
}

// restartProcess inicia uma nova instância do executável atual com os mesmos argumentos
func restartProcess() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Start()
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

// runSelfUpdate executa a substituição do binário atual em tempo de execução
func runSelfUpdate() error {
	downloadURL := getDirectDownloadURL()

	currentExec, err := os.Executable()
	if err != nil {
		return fmt.Errorf("não foi possível identificar o executável: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "gokit-cli-updater")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("falha na conexão de rede: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("servidor retornou erro (status %d)", resp.StatusCode)
	}

	oldExec := currentExec + ".old"
	_ = os.Remove(oldExec)

	// Renomeia o executável em execução (funciona no Windows)
	err = os.Rename(currentExec, oldExec)
	if err != nil {
		return fmt.Errorf("falha ao preparar arquivos: %v", err)
	}

	// Cria o novo executável no mesmo local original
	out, err := os.OpenFile(currentExec, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		_ = os.Rename(oldExec, currentExec) // Desfaz em caso de erro
		return fmt.Errorf("falha ao abrir novo arquivo: %v", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		_ = os.Rename(oldExec, currentExec)
		return fmt.Errorf("falha ao gravar atualização: %v", err)
	}

	return nil
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
					m.state = stateMigrationsMenu
					m.cursor = 0
				case 1:
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
			"\033[1m\033[36m[Ação]\033[0m Criando nova estrutura de migration...\n\n" +
				"\033[32m✔ Migration criada com sucesso! (gokit_migration_placeholder.go)\033[0m\n\n" +
				"Pressione \033[1m[Enter]\033[0m para voltar.",
		)
		s.WriteString(actionBoxStyle.Render(content) + "\n")

	case stateMigrationRunning:
		content := fmt.Sprintf(
			"\033[1m\033[36m[Ação]\033[0m Executando migrações pendentes...\n\n" +
				"\033[32m✔ Banco de dados atualizado! Todas as migrações foram aplicadas.\033[0m\n\n" +
				"Pressione \033[1m[Enter]\033[0m para voltar.",
		)
		s.WriteString(actionBoxStyle.Render(content) + "\n")

	case stateMigrationRollingBack:
		content := fmt.Sprintf(
			"\033[1m\033[36m[Ação]\033[0m Revertendo última migration (Rollback)...\n\n" +
				"\033[32m✔ Rollback executado com sucesso!\033[0m\n\n" +
				"Pressione \033[1m[Enter]\033[0m para voltar.",
		)
		s.WriteString(actionBoxStyle.Render(content) + "\n")
	}

	if m.state == stateMainMenu || m.state == stateMigrationsMenu {
		s.WriteString(footerStyle.Render("↑/↓: navegar • enter: selecionar • ctrl+c: sair") + "\n")
	}

	return s.String()
}
