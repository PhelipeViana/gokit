package main

import (
	"fmt"
	"os"
	"time"

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
