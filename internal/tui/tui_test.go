package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"gokit/internal/config"
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

// menuDeTeste monta o modelo como Start faz, sem abrir conexão.
func menuDeTeste() model {
	return model{
		choices: []string{"Configuração", "Migration Options", "Seed Options", "Factory Options", "Sair (Exit)"},
		factoryChoices: []string{
			"Gerar Factories a partir das Migrations",
			"Validar Factories (não toca no banco)",
			"Popular todas as tabelas ativas",
			"Popular uma tabela (traz as dependências)",
			"Voltar ao menu principal",
		},
		configData: config.ConfigState{ActiveClient: "postgres", ActiveDialect: "postgres"},
	}
}

func TestMenuPrincipalOfereceFactory(t *testing.T) {
	m := menuDeTeste()
	m.state = stateMainMenu
	if !strings.Contains(m.View(), "Factory Options") {
		t.Fatalf("o menu principal deveria listar Factory Options:\n%s", m.View())
	}
}

func TestMenuDeFactoryListaAsQuatroAcoes(t *testing.T) {
	m := menuDeTeste()
	m.state = stateFactoryMenu
	saida := m.View()
	for _, esperado := range []string{"Gerar Factories", "Validar Factories", "Popular todas", "Popular uma tabela", "Voltar"} {
		if !strings.Contains(saida, esperado) {
			t.Fatalf("faltou %q no menu:\n%s", esperado, saida)
		}
	}
	// O aviso de que popular limpa a tabela precisa estar visível antes do ato.
	if !strings.Contains(saida, "limpa a tabela") {
		t.Fatalf("o menu deveria avisar que popular limpa a tabela:\n%s", saida)
	}
}

// A lista tem centenas de tabelas: só uma janela em volta do cursor aparece.
func TestSelecaoDeTabelaUsaJanela(t *testing.T) {
	m := menuDeTeste()
	m.state = stateFactorySelectTable
	for i := 0; i < 40; i++ {
		m.factoryTables = append(m.factoryTables, fmt.Sprintf("TABELA_%02d", i))
	}
	m.factoryCursor = 20
	saida := m.View()

	if strings.Count(saida, "TABELA_") > 14 {
		t.Fatalf("a janela deveria limitar os itens visíveis:\n%s", saida)
	}
	if !strings.Contains(saida, "TABELA_20") {
		t.Fatal("o item sob o cursor precisa aparecer")
	}
	if !strings.Contains(saida, "21 de 40") {
		t.Fatalf("faltou a posição na lista:\n%s", saida)
	}
}

func TestSelecaoVaziaOrientaVoltar(t *testing.T) {
	m := menuDeTeste()
	m.state = stateFactorySelectTable
	if !strings.Contains(m.View(), "Nenhuma factory encontrada") {
		t.Fatalf("lista vazia deveria explicar o que houve:\n%s", m.View())
	}
}
