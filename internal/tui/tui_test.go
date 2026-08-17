package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/updater"
	tea "github.com/charmbracelet/bubbletea"
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

func TestCorDoSelectRefleteEstadoDoExec(t *testing.T) {
	m := menuDeTeste()
	if got := string(m.statusColor()); got != "#FF5555" {
		t.Fatalf("sem conexão deveria ser vermelho, veio %s", got)
	}
	m.configData.Config = &config.Config{}
	m.configData.ConnSuccess = true
	m.updateStatus = updater.Status{Available: true}
	if got := string(m.statusColor()); got != "#F1FA8C" {
		t.Fatalf("com atualização deveria ser amarelo, veio %s", got)
	}
	m.updateStatus.Available = false
	if got := string(m.statusColor()); got != "#50FA7B" {
		t.Fatalf("estado saudável deveria ser verde, veio %s", got)
	}
}

func TestSelecaoDeMigrationTemScrollAutomatico(t *testing.T) {
	m := menuDeTeste()
	m.state = stateMigrationSelectTable
	for i := 0; i < 50; i++ {
		m.availableTables = append(m.availableTables, fmt.Sprintf("TABELA_%02d", i))
	}
	m.tableCursor = 30
	saida := m.View()
	if !strings.Contains(saida, "TABELA_30") || !strings.Contains(saida, "31 de 50") {
		t.Fatalf("cursor e posição devem permanecer visíveis:\n%s", saida)
	}
	if strings.Count(saida, "TABELA_") > 12 {
		t.Fatalf("a lista não deveria ultrapassar a janela do terminal:\n%s", saida)
	}
}

func TestDoctorEhChecklistSemCaixa(t *testing.T) {
	m := menuDeTeste()
	m.state = stateConfigScreen
	m.configData.Config = &config.Config{}
	m.doctorReport.ConnSuccess = true
	m.doctorReport.VersionOK = true
	m.doctorReport.DDLSuccess = true
	saida := m.View()
	if !strings.Contains(saida, "🔍 Doctor · checklist") || !strings.Contains(saida, "✅") {
		t.Fatalf("doctor deveria ser uma lista de checks:\n%s", saida)
	}
	if strings.Contains(saida, "╭") || strings.Contains(saida, "╰") {
		t.Fatalf("doctor não deveria usar caixa:\n%s", saida)
	}
}

func TestCabecalhoDeAmbienteEhCompacto(t *testing.T) {
	m := menuDeTeste()
	m.configData.ActiveEnv = "development"
	m.configData.ActiveClient = "mysql"
	m.configData.ConnSuccess = true
	m.configData.Config = &config.Config{Connections: map[string]config.ConnConfig{
		"mysql": {Dialect: "mysql", Host: "127.0.0.1", Port: "3306", Database: "app"},
	}}
	header := m.renderHeader()
	for _, expected := range []string{"DEV", "🐬 MySQL", "✅ sucesso"} {
		if !strings.Contains(header, expected) {
			t.Fatalf("cabeçalho não contém %q:\n%s", expected, header)
		}
	}
}

func TestAssinaturaDoAmbienteMudaQuandoEnvForSalvo(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "gokit.json")
	if err := os.WriteFile(envPath, []byte("DB_PORT=3306\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := config.ConfigState{
		ConfigPath: configPath,
		Config:     &config.Config{Environment: config.EnvConfig{MapperEnv: envPath}},
	}
	before := environmentSignature(state)
	if err := os.WriteFile(envPath, []byte("DB_PORT=3307\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if after := environmentSignature(state); after == before {
		t.Fatal("salvar o .env deveria disparar uma nova verificação de status")
	}
}

func TestResultadoDeErroEhListaSemCaixa(t *testing.T) {
	m := menuDeTeste()
	m.migrationError = cliui.NewUserError("Conexão indisponível.", "Revise o host.\nInicie o container.")
	saida := m.renderActionResult("OK", "Falha")
	for _, expected := range []string{"❌", "⚠️ Possíveis soluções:", "• Revise o host.", "• Inicie o container."} {
		if !strings.Contains(saida, expected) {
			t.Fatalf("resultado não contém %q:\n%s", expected, saida)
		}
	}
	if strings.Contains(saida, "╭") || strings.Contains(saida, "╰") {
		t.Fatalf("resultado não deveria usar caixa:\n%s", saida)
	}
}

func TestReloadMostraEstadoDeExecucaoAntesDoResultado(t *testing.T) {
	m := menuDeTeste()
	m.state = stateReloadRunning
	m.actionRunning = true
	saida := m.View()
	if !strings.Contains(saida, "Reload em andamento") || !strings.Contains(saida, "Aguarde") {
		t.Fatalf("reload deveria mostrar retorno visual imediato:\n%s", saida)
	}
	if strings.Contains(saida, "Reload concluído") {
		t.Fatalf("reload não pode indicar sucesso antes de terminar:\n%s", saida)
	}
}

func TestListaDeMetodosSeAdaptaAoTerminalPequeno(t *testing.T) {
	m := menuDeTeste()
	m.state = stateMigrationSelectMethod
	m.terminalHeight = 11
	m.methodsChoices = []string{"create_table", "drop_table", "add_column", "alter_column", "drop_column", "raw_sql", "todo"}
	m.methodCursor = 0
	saida := m.View()
	if !strings.Contains(saida, "create_table") {
		t.Fatalf("primeira opção deve continuar visível em terminal pequeno:\n%s", saida)
	}
	if strings.Count(saida, "_table") > 3 {
		t.Fatalf("lista deveria respeitar a altura disponível:\n%s", saida)
	}

	m.methodCursor = len(m.methodsChoices) - 1
	saida = m.View()
	if !strings.Contains(saida, "todo") || !strings.Contains(saida, "↑ mais opções") {
		t.Fatalf("seleção final e indicação de opções anteriores devem aparecer:\n%s", saida)
	}
}

// O menu de migrations ganhou a leitura do banco. O teste guarda duas coisas: os
// itens aparecem, e o "Voltar" continua sendo o ÚLTIMO — a execução das escolhas é
// por índice, então inserir item no meio sem acertar os `case` faria "Voltar"
// disparar um import.
func TestMenuDeMigrationsOfereceLeituraDoBanco(t *testing.T) {
	m := menuDeTeste()
	m.migrationsChoices = opcoesDeMigration()
	m.state = stateMigrationsMenu
	saida := m.View()
	for _, esperado := range []string{"Ler o banco", "Escrever migrations das tabelas"} {
		if !strings.Contains(saida, esperado) {
			t.Fatalf("faltou %q no menu de migrations:\n%s", esperado, saida)
		}
	}
	if ultimo := m.migrationsChoices[len(m.migrationsChoices)-1]; !strings.Contains(ultimo, "Voltar") {
		t.Fatalf("o último item deveria ser Voltar, é %q", ultimo)
	}
	// Índices que o switch de stateMigrationsMenu assume, escritos aqui para que
	// mudar a ordem sem mudar o switch falhe.
	esperados := []string{"Criar", "Validar", "Executar", "Desfazer", "Ler o banco", "Escrever migrations", "Voltar"}
	if len(m.migrationsChoices) != len(esperados) {
		t.Fatalf("o menu tem %d itens, o switch trata %d", len(m.migrationsChoices), len(esperados))
	}
	for posicao, prefixo := range esperados {
		if !strings.Contains(m.migrationsChoices[posicao], prefixo) {
			t.Fatalf("posição %d deveria conter %q, tem %q", posicao, prefixo, m.migrationsChoices[posicao])
		}
	}
}

// A área ORM tem o mesmo risco de índice do menu de migrations, com um agravante: um
// dos itens é DESABILITADO e outro APAGA pasta. Deslocar a lista sem acertar os `case`
// faria "Voltar" rodar o Special Mapper com --confirm.
func TestMenuDaORMMantemAOrdemQueOSwitchTrata(t *testing.T) {
	m := menuDeTeste()
	m.ormChoices = opcoesDaORM()
	m.state = stateORMMenu

	esperados := []string{"Tudo", "Colunas, tabelas e entidades", "prévia", "aplicar", "indisponível", "Voltar"}
	if len(m.ormChoices) != len(esperados) {
		t.Fatalf("o menu tem %d itens, o switch trata %d", len(m.ormChoices), len(esperados))
	}
	for posicao, trecho := range esperados {
		if !strings.Contains(m.ormChoices[posicao], trecho) {
			t.Fatalf("posição %d deveria conter %q, tem %q", posicao, trecho, m.ormChoices[posicao])
		}
	}
	// O índice do item desabilitado é uma constante usada pelo switch E pela pintura.
	if !strings.Contains(m.ormChoices[indiceDeServicosNaORM], "Serviços") {
		t.Fatalf("indiceDeServicosNaORM aponta para %q", m.ormChoices[indiceDeServicosNaORM])
	}

	// O cabeçalho declara que nada aqui escreve no banco — é o que separa este menu do
	// reload para quem escolhe.
	saida := m.View()
	if !strings.Contains(saida, "nenhuma opção escreve no banco") {
		t.Fatalf("o menu deveria declarar que não escreve no banco:\n%s", saida)
	}
}

// O menu principal ganhou a área ORM entre Factories e Configuração. A ordem importa:
// o switch de stateMainMenu despacha por índice.
func TestMenuPrincipalOfereceAreaORM(t *testing.T) {
	m := menuDeTeste()
	m.choices = []string{"Reload", "Migrations", "Seeds", "Factories", i18n.T("menu_orm"), "Configuração", "Sair"}
	m.state = stateMainMenu
	if !strings.Contains(m.View(), "Área ORM") {
		t.Fatalf("o menu principal deveria listar a Área ORM:\n%s", m.View())
	}
}

// A tela de resultado da área ORM volta para o menu da ORM, não para o principal: as
// ações se encadeiam (gerar entidade, depois mapear os especiais). O `default` do switch
// mandava tudo para o principal, então isto precisa de um `case` próprio — e de guarda,
// porque o `default` é fácil de reconquistar sem querer.
func TestResultadoDaORMVoltaParaOMenuDaORM(t *testing.T) {
	m := menuDeTeste()
	m.ormChoices = opcoesDaORM()
	m.state = stateORMRunning
	m.cursor = 3

	atualizado, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	depois, ok := atualizado.(model)
	if !ok {
		t.Fatalf("Update devolveu %T", atualizado)
	}
	if depois.state != stateORMMenu {
		t.Errorf("deveria voltar ao menu da ORM, foi para o estado %d", depois.state)
	}
	if depois.cursor != 0 {
		t.Errorf("o cursor deveria voltar ao topo, está em %d", depois.cursor)
	}
}

// O contraste: a tela de resultado da factory continua voltando ao menu principal. Sem
// isto, dar `case` próprio a um estado novo poderia ser generalizado por engano.
func TestResultadoDaFactoryContinuaVoltandoAoPrincipal(t *testing.T) {
	m := menuDeTeste()
	m.state = stateFactoryRunning

	atualizado, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	depois, ok := atualizado.(model)
	if !ok {
		t.Fatalf("Update devolveu %T", atualizado)
	}
	if depois.state != stateMainMenu {
		t.Errorf("deveria voltar ao menu principal, foi para o estado %d", depois.state)
	}
}
