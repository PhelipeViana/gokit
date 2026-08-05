package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// CommitHash é injetado em tempo de compilação via -ldflags.
// Se vazio, significa que está rodando em modo desenvolvimento.
var CommitHash = "development"

// URL oficial do repositório e endpoint de commits do GitHub
const RepoURL = "https://github.com/PhelipeViana/gokit"
const CommitsAPI = "https://api.github.com/repos/PhelipeViana/gokit/commits"

// Cores ANSI para estilização do terminal
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Cyan    = "\033[36m"
)

type Commit struct {
	SHA string `json:"sha"`
}

func main() {
	setupSignalHandler()
	
	// Limpa a tela inicial
	clearScreen()
	
	// Executa a checagem de atualizações
	checkUpdates()

	// Inicia a interface CLI interativa
	runCLI()
}

// setupSignalHandler intercepta o Ctrl+C para sair de forma amigável
func setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Printf("\n\n%sExecução interrompida pelo usuário. Saindo...%s\n", Red, Reset)
		os.Exit(0)
	}()
}

// terminalLink gera um link clicável usando a especificação OSC 8.
// Exibe também a URL em parênteses como fallback caso o terminal não suporte.
func terminalLink(url, text string) string {
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\ (%s)", url, text, url)
}

// fetchLatestRemoteCommit busca o hash do commit mais recente na API do GitHub
func fetchLatestRemoteCommit() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", CommitsAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gokit-cli")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status HTTP inválido: %s", resp.Status)
	}

	var commits []Commit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return "", err
	}

	if len(commits) == 0 {
		return "", errors.New("nenhum commit encontrado no repositório remoto")
	}

	return commits[0].SHA, nil
}

// checkUpdates compara a versão do binário atual com o último commit do GitHub
func checkUpdates() {
	if CommitHash == "development" {
		fmt.Printf("%s[Modo Desenvolvimento] Ignorando verificação de atualizações.%s\n\n", Yellow, Reset)
		return
	}

	fmt.Printf("%sVerificando atualizações no repositório remoto...%s\n", Cyan, Reset)
	remoteSHA, err := fetchLatestRemoteCommit()
	if err != nil {
		fmt.Printf("%s[Aviso] Não foi possível verificar atualizações: %v%s\n\n", Yellow, err, Reset)
		return
	}

	// Compara os hashes de commit
	shortLocal := CommitHash
	if len(shortLocal) > 7 {
		shortLocal = shortLocal[:7]
	}
	shortRemote := remoteSHA
	if len(shortRemote) > 7 {
		shortRemote = shortRemote[:7]
	}

	if !strings.HasPrefix(remoteSHA, CommitHash) && !strings.HasPrefix(CommitHash, remoteSHA) {
		fmt.Println()
		fmt.Println(strings.Repeat("=", 65))
		fmt.Printf("%s%s    ATENÇÃO: NOVA VERSÃO DO GOKIT DETECTADA!%s\n", Red, Bold, Reset)
		fmt.Println(strings.Repeat("=", 65))
		fmt.Printf("Sua versão local:      %s%s%s\n", Yellow, shortLocal, Reset)
		fmt.Printf("Versão mais recente:   %s%s%s\n", Green, shortRemote, Reset)
		fmt.Println()
		fmt.Println("Você está executando uma versão desatualizada do executável.")
		
		link := terminalLink(RepoURL, "Repositório do Go Kit")
		fmt.Printf("Por favor, faça o download manual da nova versão em:\n%s%s%s\n", Cyan, link, Reset)
		fmt.Println(strings.Repeat("=", 65))
		fmt.Println()

		fmt.Printf("Pressione %s[Enter]%s para continuar assim mesmo ou %s[Ctrl+C]%s para sair...", Bold, Reset, Bold, Reset)
		var discard string
		fmt.Scanln(&discard)
		fmt.Println()
	} else {
		fmt.Printf("%s[Sucesso] Seu gokit.exe está atualizado! (Commit: %s)%s\n\n", Green, shortLocal, Reset)
	}
}

// runCLI gerencia o menu principal da aplicação
func runCLI() {
	for {
		clearScreen()
		fmt.Println(Bold + Cyan + "=========================================" + Reset)
		fmt.Println(Bold + Cyan + "          G O   K I T   C L I            " + Reset)
		fmt.Println(Bold + Cyan + "=========================================" + Reset)
		fmt.Printf("Versão: %s%s%s\n\n", Yellow, CommitHash, Reset)
		
		fmt.Println(Bold + "Menu Principal:" + Reset)
		fmt.Printf("  %s[1]%s Migration Options\n", Green, Reset)
		fmt.Printf("  %s[2]%s Sair (Exit)\n", Red, Reset)
		fmt.Println()
		fmt.Printf("Escolha uma opção (1-2): ")

		var input string
		fmt.Scanln(&input)
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			runMigrationOptions()
		case "2":
			fmt.Printf("\n%sSaindo... Obrigado por usar o Go Kit!%s\n", Green, Reset)
			return
		default:
			fmt.Printf("\n%sOpção inválida. Pressione [Enter] para tentar novamente.%s", Red, Reset)
			fmt.Scanln()
		}
	}
}

// runMigrationOptions gerencia o submenu de migrações
func runMigrationOptions() {
	for {
		clearScreen()
		fmt.Println(Bold + Cyan + "=========================================" + Reset)
		fmt.Println(Bold + Cyan + "        M I G R A T I O N S   M E N U    " + Reset)
		fmt.Println(Bold + Cyan + "=========================================" + Reset)
		fmt.Println()
		fmt.Println(Bold + "Opções de Migração:" + Reset)
		fmt.Printf("  %s[1]%s Criar nova Migration\n", Green, Reset)
		fmt.Printf("  %s[2]%s Executar Migrations pendentes\n", Green, Reset)
		fmt.Printf("  %s[3]%s Reverter última Migration (Rollback)\n", Yellow, Reset)
		fmt.Printf("  %s[4]%s Voltar ao menu principal\n", Red, Reset)
		fmt.Println()
		fmt.Printf("Escolha uma opção (1-4): ")

		var input string
		fmt.Scanln(&input)
		input = strings.TrimSpace(input)

		switch input {
		case "1":
			fmt.Printf("\n%s[Ação]%s Criando uma nova estrutura de migration...\n", Cyan, Reset)
			fmt.Printf("\n%sMigration criada com sucesso! (Placeholder)%s\n", Green, Reset)
			fmt.Println("\nPressione [Enter] para voltar.")
			fmt.Scanln()
		case "2":
			fmt.Printf("\n%s[Ação]%s Executando migrações pendentes...\n", Cyan, Reset)
			fmt.Printf("\n%sBanco de dados atualizado com sucesso! (Placeholder)%s\n", Green, Reset)
			fmt.Println("\nPressione [Enter] para voltar.")
			fmt.Scanln()
		case "3":
			fmt.Printf("\n%s[Ação]%s Revertendo migrações...\n", Cyan, Reset)
			fmt.Printf("\n%sRollback executado com sucesso! (Placeholder)%s\n", Green, Reset)
			fmt.Println("\nPressione [Enter] para voltar.")
			fmt.Scanln()
		case "4":
			return
		default:
			fmt.Printf("\n%sOpção inválida. Pressione [Enter] para tentar novamente.%s", Red, Reset)
			fmt.Scanln()
		}
	}
}

// clearScreen limpa o terminal escrevendo sequências de escape ANSI
func clearScreen() {
	fmt.Print("\033[H\033[2J")
}
