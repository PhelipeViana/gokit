package updater

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
)

// CleanOldExecutables deleta arquivos temporários .old gerados no auto-update.
// Executado em goroutine com retentativas para dar tempo do processo pai fechar.
func CleanOldExecutables() {
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

// RunSilentUpdateCheck verifica atualizações de forma silenciosa
func RunSilentUpdateCheck(commitHash string) (bool, string, string) {
	if commitHash == "development" || commitHash == "local" {
		return false, "", ""
	}

	// Se a variável de ambiente GOKIT_NO_UPDATE for igual a true, desativa o update remoto
	if os.Getenv("GOKIT_NO_UPDATE") == "true" {
		return false, "", ""
	}

	// Se já foi reiniciado pós-update, pula o check para evitar loops
	if os.Getenv("GOKIT_AUTO_UPDATED") == "true" {
		return false, "", ""
	}

	remoteSHA, err := fetchLatestRemoteCommit()
	if err != nil {
		return false, "", ""
	}

	if !strings.HasPrefix(remoteSHA, commitHash) && !strings.HasPrefix(commitHash, remoteSHA) {
		shortLocal := commitHash
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

// GetDirectDownloadURL gera o link de download direto do arquivo binário bruto no repositório GitHub
func GetDirectDownloadURL() string {
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

// RunSelfUpdate executa a substituição do binário atual em tempo de execução
func RunSelfUpdate() error {
	downloadURL := GetDirectDownloadURL()
	
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

// RestartProcess inicia uma nova instância do executável atual com os mesmos argumentos
func RestartProcess() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Repassa variáveis do ambiente e sinaliza que este é o processo pós-update
	cmd.Env = append(os.Environ(), "GOKIT_AUTO_UPDATED=true")

	return cmd.Start()
}
