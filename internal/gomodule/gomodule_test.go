package gomodule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCreatesLocalDevelopmentModule(t *testing.T) {
	root := t.TempDir()
	framework := filepath.Join(root, "gokit")
	project := filepath.Join(root, "Minha Aplicacao")
	if err := os.MkdirAll(framework, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(framework, "go.mod"), []byte("module "+CanonicalGoKitModule+"\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Ensure(Options{Root: project, GoKitLocal: "../gokit"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Module != "minha-aplicacao" {
		t.Fatalf("modulo inesperado: %s", result.Module)
	}
	data, err := os.ReadFile(filepath.Join(project, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, expected := range []string{
		"module minha-aplicacao",
		"require " + CanonicalGoKitModule + " v0.0.0",
		"replace " + CanonicalGoKitModule + " => ../gokit",
	} {
		if !strings.Contains(content, expected) {
			t.Errorf("go.mod não contém %q:\n%s", expected, content)
		}
	}

	if _, err := Ensure(Options{Root: project, GoKitLocal: "../gokit"}); err != nil {
		t.Fatalf("segunda execução deve ser idempotente: %v", err)
	}
}

func TestEnsureRejectsMissingLocalGoKit(t *testing.T) {
	_, err := Ensure(Options{Root: t.TempDir(), GoKitLocal: "../inexistente"})
	if err == nil || !strings.Contains(err.Error(), "nao encontrado") {
		t.Fatalf("erro esperado para caminho local inexistente, recebido: %v", err)
	}
}
