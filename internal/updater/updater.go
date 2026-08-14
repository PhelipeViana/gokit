package updater

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/i18n"
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
		return "", i18n.Errf("upd_remote_version_failed", resp.StatusCode)
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
		return i18n.Errf("upd_exec_unknown", err)
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
		return i18n.Errf("upd_network_failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return i18n.Errf("upd_server_error", resp.StatusCode)
	}

	newExec := currentExec + ".new"
	oldExec := currentExec + ".old"
	// Sobra de tentativa anterior. Se resistir, o OpenFile abaixo é quem reporta.
	_ = os.Remove(newExec)
	out, err := os.OpenFile(newExec, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return i18n.Errf("upd_prepare_failed", err)
	}
	written, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(newExec)
		if copyErr != nil {
			return i18n.Errf("upd_write_failed", copyErr)
		}
		return i18n.Errf("upd_finish_failed", closeErr)
	}
	if written < 1024 {
		_ = os.Remove(newExec)
		return i18n.Errf("upd_download_too_small", written)
	}
	if err := os.Chmod(newExec, 0o755); err != nil {
		_ = os.Remove(newExec)
		return i18n.Errf("upd_chmod_failed", err)
	}

	// Backup anterior. Se resistir, o Rename abaixo é quem reporta.
	_ = os.Remove(oldExec)
	if err := os.Rename(currentExec, oldExec); err != nil {
		_ = os.Remove(newExec)
		return i18n.Errf("upd_backup_failed", err)
	}
	// Daqui para baixo o executável em uso já foi movido para .old, e todo caminho de
	// falha tem de trazê-lo de volta. É por isso que o retorno passa por restaurar():
	// se a volta também falhar, o usuário fica SEM gokit nenhum, e a mensagem original
	// ("não deu para instalar") não diria isso nem como resolver.
	if err := os.Rename(newExec, currentExec); err != nil {
		_ = os.Remove(newExec)
		return restaurar(oldExec, currentExec, i18n.Errf("upd_install_failed", err))
	}
	if _, err := os.Stat(currentExec); err != nil {
		return restaurar(oldExec, currentExec, i18n.Errf("upd_write_failed", err))
	}
	if err := prepareExecutable(currentExec); err != nil {
		// O baixado sai da frente para o antigo poder voltar ao nome. Se ele não sair, o
		// Rename da restauração falha e restaurar() diz o que fazer à mão.
		_ = os.Remove(currentExec)
		return restaurar(oldExec, currentExec, i18n.Errf("upd_macos_validate_failed", err))
	}
	return nil
}

// restaurar devolve o executável antigo ao lugar depois de uma atualização falhada e
// junta as duas notícias em um erro só.
//
// A falha da restauração é MAIS grave que a da atualização: a atualização falhada
// deixa o usuário na versão anterior, que funciona; a restauração falhada deixa o
// comando sem binário. Antes o segundo erro era descartado e o usuário lia apenas
// "não foi possível instalar", com o gokit inexistente no caminho.
func restaurar(oldExec, currentExec string, causa error) error {
	if err := os.Rename(oldExec, currentExec); err != nil {
		return i18n.Errf("upd_restore_failed", causa, oldExec, currentExec, err)
	}
	return causa
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
		return i18n.Errf("upd_codesign", strings.TrimSpace(string(output)), err)
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
