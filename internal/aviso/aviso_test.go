package aviso

import (
	"strings"
	"testing"
)

// canalDeTeste guarda o que seria enviado, em vez de postar.
type canalDeTeste struct{ recebidos []string }

func (c *canalDeTeste) Nome() string { return "teste" }
func (c *canalDeTeste) Enviar(texto string) error {
	c.recebidos = append(c.recebidos, texto)
	return nil
}

func ambienteDeTeste() Ambiente {
	return Ambiente{Projeto: "projeto_x", Cliente: "client_sqlserver", Dialeto: "sqlserver", Versao: "v0.1.0"}
}

// Quem classifica é o gokit; quem recebe filtra. O padrão cala só info, que é o que
// já está resolvido — os outros três pedem alguma ação de alguém.
func TestFiltroPadraoCalaSoInfo(t *testing.T) {
	canal := &canalDeTeste{}
	central := Nova(ambienteDeTeste(), nil, canal)

	for _, nivel := range []Nivel{NivelInfo, NivelWarning, NivelError, NivelDanger} {
		central.Notificar(Aviso{Nivel: nivel, Acao: string(nivel)})
	}

	if len(canal.recebidos) != 3 {
		t.Fatalf("esperava warning, error e danger, veio %d envio(s)", len(canal.recebidos))
	}
	for _, texto := range canal.recebidos {
		if strings.Contains(texto, "· INFO") {
			t.Error("info não deveria ser enviado por padrão")
		}
	}
}

// O filtro do gokit.json manda, e pode INVERTER o padrão: projeto que quer só info
// recebe só info, e o danger é calado. Quem recebe decide.
func TestFiltroDoProjetoSubstituiOPadrao(t *testing.T) {
	canal := &canalDeTeste{}
	central := Nova(ambienteDeTeste(), []string{"info"}, canal)

	central.Notificar(Aviso{Nivel: NivelDanger, Acao: "erro"})
	central.Notificar(Aviso{Nivel: NivelInfo, Acao: "aviso"})

	if len(canal.recebidos) != 1 || !strings.Contains(canal.recebidos[0], "AVISO") {
		t.Fatalf("só o nível configurado deveria passar, veio %v", canal.recebidos)
	}
}

// Sem canal, notificar é no-op: notificação é opt-in por projeto.
func TestSemCanalNaoEnviaNada(t *testing.T) {
	central := Nova(ambienteDeTeste(), nil, NovoSlack("", false))
	if central.Ativa() {
		t.Fatal("sem webhook a central deveria estar inerte")
	}
	central.Notificar(Aviso{Nivel: NivelDanger, Acao: "erro"})
}

// A mensagem tem duas partes com propósitos diferentes: o cabeçalho, que segue a
// convenção do canal, e o RELATÓRIO COLÁVEL, que é o que permite resolver sem acesso
// à máquina de quem rodou. O gokit não tem IA dentro — o bloco é o que se cola.
func TestMensagemTemCabecalhoERelatorioColavel(t *testing.T) {
	canal := &canalDeTeste{}
	central := Nova(ambienteDeTeste(), nil, canal)

	central.Notificar(Aviso{
		Nivel:   NivelDanger,
		Acao:     "erro não mapeado",
		Emoji:    ":rotating_light:",
		Corpo:    "O comando falhou.",
		Contexto: map[string]string{"comando": "migrate run", "arquivo": "2026_01_01_000001_users.go"},
		Bruto:    "mssql: Could not create constraint or index.",
	})

	if len(canal.recebidos) != 1 {
		t.Fatalf("esperava 1 envio, veio %d", len(canal.recebidos))
	}
	texto := canal.recebidos[0]

	// Cabeçalho na convenção que o canal já usa.
	for _, esperado := range []string{
		":rotating_light: AgendaGoKit · ERRO NÃO MAPEADO",
		"Quem: ", "Projeto: projeto_x", "Cliente: client_sqlserver (sqlserver)",
	} {
		if !strings.Contains(texto, esperado) {
			t.Errorf("faltou %q no cabeçalho:\n%s", esperado, texto)
		}
	}

	// Relatório colável, com o que quem resolve precisa.
	if !strings.Contains(texto, legendaDoRelatorio) || strings.Count(texto, "```") != 2 {
		t.Errorf("o relatório deveria vir em bloco de código:\n%s", texto)
	}
	for _, esperado := range []string{
		"nivel: danger", "produto: AgendaGoKit v0.1.0",
		"comando: migrate run", "arquivo: 2026_01_01_000001_users.go",
		"Could not create constraint",
	} {
		if !strings.Contains(texto, esperado) {
			t.Errorf("faltou %q no relatório:\n%s", esperado, texto)
		}
	}
}

// Texto cru de banco pode vir enorme; cortar mantém o começo, que é onde está a causa.
func TestRecortarMantemOComeco(t *testing.T) {
	longo := strings.Repeat("x", 3000)
	cortado := Recortar(longo, 100)
	if len(cortado) > 130 || !strings.HasPrefix(cortado, "xxxx") || !strings.Contains(cortado, "cortado") {
		t.Fatalf("recorte inesperado: %d caracteres", len(cortado))
	}
}
