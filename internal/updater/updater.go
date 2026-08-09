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

type Status struct {
	Available bool
	Local     string
	Remote    string
	Error     error
}

func Check(commitHash string) Status {
	if commitHash == "development" || commitHash == "local" || commitHash == "" {
		return Status{Local: commitHash}
	}
	if os.Getenv("GOKIT_NO_UPDATE") == "true" {
		return Status{Local: shortHash(commitHash)}
	}
	remote, err := fetchLatestRemoteCommit()
	status := Status{Local: shortHash(commitHash), Remote: shortHash(remote), Error: err}
	if err == nil {
		status.Available = !strings.HasPrefix(remote, commitHash) && !strings.HasPrefix(commitHash, remote)
	}
	return status
}

func shortHash(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 7 {
		return value[:7]
	}
	return value
}

// CleanOldExecutables deleta arquivos temporários .old gerados no auto-update.
// Executado em goroutine com retentativas para dar tempo do processo pai fechar.
func CleanOldExecutables() {
	execPath, err := os.Executable()
	if err == nil {
		oldPath := execPath + ".old"
		_ = os.Remove(execPath + ".new")
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

	status := Check(commitHash)
	return status.Available, status.Local, status.Remote
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

	newExec := currentExec + ".new"
	oldExec := currentExec + ".old"
	_ = os.Remove(newExec)
	out, err := os.OpenFile(newExec, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("falha ao preparar download: %v", err)
	}
	written, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(newExec)
		if copyErr != nil {
			return fmt.Errorf("falha ao gravar atualização: %v", copyErr)
		}
		return fmt.Errorf("falha ao finalizar atualização: %v", closeErr)
	}
	if written < 1024 {
		_ = os.Remove(newExec)
		return fmt.Errorf("download inválido: executável recebido tem apenas %d bytes", written)
	}
	if err := os.Chmod(newExec, 0o755); err != nil {
		_ = os.Remove(newExec)
		return fmt.Errorf("falha ao tornar atualização executável: %v", err)
	}

	_ = os.Remove(oldExec)
	if err := os.Rename(currentExec, oldExec); err != nil {
		_ = os.Remove(newExec)
		return fmt.Errorf("falha ao preservar executável atual: %v", err)
	}
	if err := os.Rename(newExec, currentExec); err != nil {
		_ = os.Rename(oldExec, currentExec)
		_ = os.Remove(newExec)
		return fmt.Errorf("falha ao instalar atualização: %v", err)
	}
	if _, err := os.Stat(currentExec); err != nil {
		_ = os.Rename(oldExec, currentExec)
		return fmt.Errorf("falha ao gravar atualização: %v", err)
	}
	if err := prepareExecutable(currentExec); err != nil {
		_ = os.Remove(currentExec)
		_ = os.Rename(oldExec, currentExec)
		return fmt.Errorf("falha ao validar atualização no macOS: %v", err)
	}
	return nil
}

// prepareExecutable renova a assinatura ad-hoc depois que o arquivo é
// substituído. Sem isso, o macOS pode encerrar o processo com SIGKILL antes
// mesmo de o GoKit conseguir mostrar uma mensagem de erro.
func prepareExecutable(path string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	output, err := exec.Command("codesign", "--force", "--sign", "-", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("codesign: %s: %w", strings.TrimSpace(string(output)), err)
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
