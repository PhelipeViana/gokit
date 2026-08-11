package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/gomodule"
)

// projetoParaModo monta projeto + gokit irmão + um gokit.json mínimo, que é o
// estado de onde a troca de modo parte.
func projetoParaModo(t *testing.T, modo string) string {
	t.Helper()
	root := t.TempDir()
	framework := filepath.Join(root, "gokit")
	project := filepath.Join(root, "app")
	if err := os.MkdirAll(framework, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "internal", "gokit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(framework, "go.mod"),
		[]byte("module "+gomodule.CanonicalGoKitModule+"\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module app\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Uma chave que o gokit não conhece, para provar que a edição do gokit.json
	// não destrói configuração alheia.
	conteudo := `{
  "language": "pt",
  "go": {
    "module": "app",
    "mode": "` + modo + `",
    "gokit_local": "../gokit",
    "campo_do_usuario": "preservar"
  },
  "environment": {"mapper_env": ".env", "ambient": "APP_ENV", "client": "DB_DIALECT"}
}`
	if err := os.WriteFile(filepath.Join(project, "internal", "gokit", "gokit.json"), []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	return project
}

func secaoGo(t *testing.T, project string) map[string]any {
	t.Helper()
	dados, err := os.ReadFile(filepath.Join(project, "internal", "gokit", "gokit.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bruto map[string]any
	if err := json.Unmarshal(dados, &bruto); err != nil {
		t.Fatalf("gokit.json ficou inválido: %v", err)
	}
	secao, ok := bruto["go"].(map[string]any)
	if !ok {
		t.Fatal("seção go ausente")
	}
	return secao
}

func TestDefinirModoGravaOCampoEAplica(t *testing.T) {
	project := projetoParaModo(t, "prod")

	resultado, err := DefinirModo(project, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if resultado.Mode != gomodule.ModoDev {
		t.Fatalf("modo aplicado inesperado: %s", resultado.Mode)
	}

	secao := secaoGo(t, project)
	if secao["mode"] != "dev" {
		t.Errorf("mode no gokit.json: %v", secao["mode"])
	}
	if secao["campo_do_usuario"] != "preservar" {
		t.Error("a edição do gokit.json apagou chave desconhecida")
	}
	if _, err := os.Stat(filepath.Join(project, "go.work")); err != nil {
		t.Error("dev deveria ter criado o go.work")
	}
}

func TestDefinirModoProdDesfazODev(t *testing.T) {
	project := projetoParaModo(t, "dev")
	if _, err := DefinirModo(project, "dev"); err != nil {
		t.Fatal(err)
	}

	if _, err := DefinirModo(project, "prod"); err != nil {
		t.Fatal(err)
	}
	if secaoGo(t, project)["mode"] != "prod" {
		t.Error("mode deveria ter virado prod no gokit.json")
	}
	if _, err := os.Stat(filepath.Join(project, "go.work")); !os.IsNotExist(err) {
		t.Error("prod deveria ter removido o go.work")
	}
}

func TestDefinirModoRecusaValorDesconhecido(t *testing.T) {
	project := projetoParaModo(t, "prod")
	if _, err := DefinirModo(project, "producao"); err == nil {
		t.Fatal("esperado erro para modo inexistente")
	}
	if secaoGo(t, project)["mode"] != "prod" {
		t.Error("um valor recusado não pode alterar o gokit.json")
	}
}

// Projeto criado antes de o campo existir não pode ficar sem modo: o palpite
// pela pasta irmã dá continuidade sem exigir edição manual.
func TestModoDoProjetoInfereQuandoOCampoFalta(t *testing.T) {
	project := projetoParaModo(t, "prod")
	cfg := &Config{}
	if modo := ModoDoProjeto(project, cfg); modo != gomodule.ModoDev {
		t.Errorf("com gokit irmão o modo inferido deveria ser dev, veio %q", modo)
	}
	cfg.Go.Mode = "prod"
	if modo := ModoDoProjeto(project, cfg); modo != gomodule.ModoProd {
		t.Errorf("o campo declarado deve vencer a inferência, veio %q", modo)
	}
}

func TestAplicarModoIgnoraGoWorkNoGit(t *testing.T) {
	project := projetoParaModo(t, "dev")
	if err := os.WriteFile(filepath.Join(project, ".gitignore"), []byte("internal/gokit/.state/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := DefinirModo(project, "dev"); err != nil {
		t.Fatal(err)
	}
	conteudo, err := os.ReadFile(filepath.Join(project, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, linha := range []string{"go.work", "go.work.sum"} {
		if !contemLinha(string(conteudo), linha) {
			t.Errorf(".gitignore sem %q:\n%s", linha, conteudo)
		}
	}
	if !contemLinha(string(conteudo), "internal/gokit/.state/") {
		t.Error("a entrada que já existia foi perdida")
	}

	// Segunda passada não pode duplicar: o reload roda a cada vez.
	if _, err := DefinirModo(project, "dev"); err != nil {
		t.Fatal(err)
	}
	depois, err := os.ReadFile(filepath.Join(project, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(depois), "go.work.sum") != 1 {
		t.Errorf("entrada duplicada:\n%s", depois)
	}
}
