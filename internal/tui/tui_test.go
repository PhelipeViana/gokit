package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func osStdout() *os.File { return os.Stdout }

func TestCaptureOutputColetaStdoutEPropagaErro(t *testing.T) {
	esperado := fmt.Errorf("falhou")
	saida, err := captureOutput(func() error {
		fmt.Println("linha um")
		fmt.Printf("linha %d\n", 2)
		return esperado
	})
	if err != esperado {
		t.Fatalf("erro esperado %v, veio %v", esperado, err)
	}
	if saida != "linha um\nlinha 2\n" {
		t.Fatalf("saída inesperada: %q", saida)
	}
}

// Um relatório de validação com centenas de linhas não pode travar o pipe:
// sem o leitor concorrente, uma saída maior que o buffer do SO bloquearia.
func TestCaptureOutputAguentaSaidaGrande(t *testing.T) {
	const linhas = 5000
	saida, err := captureOutput(func() error {
		for i := 0; i < linhas; i++ {
			fmt.Printf("linha de relatório número %d\n", i)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got := strings.Count(saida, "\n"); got != linhas {
		t.Fatalf("esperava %d linhas, veio %d", linhas, got)
	}
}

func TestCaptureOutputRestauraStdout(t *testing.T) {
	original := osStdout()
	if _, err := captureOutput(func() error { return nil }); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if osStdout() != original {
		t.Fatal("os.Stdout não voltou ao valor original")
	}
}

func TestLastLinesMantemAsUltimas(t *testing.T) {
	texto := "a\nb\nc\nd\ne"
	if got := lastLines(texto, 10); got != texto {
		t.Fatalf("texto curto não deveria mudar, veio %q", got)
	}
	got := lastLines(texto, 2)
	if !strings.HasSuffix(got, "d\ne") {
		t.Fatalf("esperava terminar em d\\ne, veio %q", got)
	}
	if !strings.Contains(got, "3 linha(s) acima omitidas") {
		t.Fatalf("esperava aviso de linhas omitidas, veio %q", got)
	}
}
