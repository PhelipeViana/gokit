package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEscreverConfigEditoresGeraTudoEDepoisNaoRepete(t *testing.T) {
	raiz := t.TempDir()

	escritos, aviso, err := EscreverConfigEditores(raiz)
	if err != nil {
		t.Fatalf("primeira geração: %v", err)
	}
	if aviso != "" {
		t.Errorf("não devia haver aviso numa pasta limpa: %s", aviso)
	}
	if len(escritos) != 4 {
		t.Errorf("esperado 4 arquivos, veio %d: %v", len(escritos), escritos)
	}
	for _, esperado := range []string{
		".vscode/extensions.json", ".vscode/settings.json", ".editorconfig", "docs/editores.md",
	} {
		if _, err := os.Stat(filepath.Join(raiz, filepath.FromSlash(esperado))); err != nil {
			t.Errorf("%s não foi criado", esperado)
		}
	}

	// Rodar de novo não pode reescrever nada: o reload roda a cada vez e o
	// arquivo está no Git — um diff por execução seria ruído puro.
	escritos, _, err = EscreverConfigEditores(raiz)
	if err != nil {
		t.Fatalf("segunda geração: %v", err)
	}
	if len(escritos) != 0 {
		t.Errorf("segunda passada devia ser inerte, escreveu: %v", escritos)
	}
}

func TestSettingsPreservaChaveDoUsuarioEAcrescentaAsQueFaltam(t *testing.T) {
	raiz := t.TempDir()
	vscode := filepath.Join(raiz, ".vscode")
	if err := os.MkdirAll(vscode, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{
  "todo-tree.tree.groupedByTag": false,
  "editor.fontSize": 15
}`
	alvo := filepath.Join(vscode, "settings.json")
	if err := os.WriteFile(alvo, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, aviso, err := EscreverConfigEditores(raiz); err != nil || aviso != "" {
		t.Fatalf("geração: err=%v aviso=%q", err, aviso)
	}

	var final map[string]any
	corpo, err := os.ReadFile(alvo)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(corpo, &final); err != nil {
		t.Fatalf("settings.json ficou inválido: %v", err)
	}
	if final["editor.fontSize"] != float64(15) {
		t.Error("chave alheia ao gokit foi perdida")
	}
	if final["todo-tree.tree.groupedByTag"] != false {
		t.Error("escolha do usuário foi sobrescrita")
	}
	if _, tem := final["todo-tree.general.tagGroups"]; !tem {
		t.Error("chave que faltava não foi acrescentada")
	}
}

func TestSettingsComComentariosNaoEModificado(t *testing.T) {
	raiz := t.TempDir()
	vscode := filepath.Join(raiz, ".vscode")
	if err := os.MkdirAll(vscode, 0o755); err != nil {
		t.Fatal(err)
	}
	// JSONC: o VS Code aceita, o encoding/json não. Reescrever perderia o
	// comentário, então o esperado é sair de lado e avisar.
	original := "{\n  // fonte grande porque o monitor é 4K\n  \"editor.fontSize\": 15\n}"
	alvo := filepath.Join(vscode, "settings.json")
	if err := os.WriteFile(alvo, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	_, aviso, err := EscreverConfigEditores(raiz)
	if err != nil {
		t.Fatal(err)
	}
	if aviso == "" {
		t.Error("esperado aviso explicando que o arquivo não foi tocado")
	}
	atual, err := os.ReadFile(alvo)
	if err != nil {
		t.Fatal(err)
	}
	if string(atual) != original {
		t.Error("settings.json com comentários foi modificado")
	}
	if _, err := os.Stat(filepath.Join(vscode, "gokit.settings.json")); err != nil {
		t.Error("as chaves do gokit deviam ter ido para o arquivo ao lado")
	}
}

func TestTagsGeradasBatemComOGuia(t *testing.T) {
	raiz := t.TempDir()
	if _, _, err := EscreverConfigEditores(raiz); err != nil {
		t.Fatal(err)
	}

	guia, err := os.ReadFile(filepath.Join(raiz, "docs", "editores.md"))
	if err != nil {
		t.Fatal(err)
	}
	corpo, err := os.ReadFile(filepath.Join(raiz, ".vscode", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(corpo, &settings); err != nil {
		t.Fatal(err)
	}
	tags, ok := settings["todo-tree.general.tags"].([]any)
	if !ok {
		t.Fatal("todo-tree.general.tags ausente")
	}

	// Toda tag varrida precisa estar documentada, senão o usuário encontra na
	// árvore uma tag que o guia não explica.
	for _, tag := range tags {
		if !strings.Contains(string(guia), tag.(string)) {
			t.Errorf("tag %v está no settings.json mas não no guia", tag)
		}
	}
	// E a exclusão dos gerados é a razão de existir do glob: não pode faltar.
	globs, _ := settings["todo-tree.filtering.excludeGlobs"].([]any)
	achou := false
	for _, glob := range globs {
		if glob == "**/*.gen.go" {
			achou = true
		}
	}
	if !achou {
		t.Error("**/*.gen.go não está em excludeGlobs")
	}
}
