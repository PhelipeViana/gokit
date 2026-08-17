package migraterun

// Regressão do Special Mapper: leitura parcial não é evidência de ausência.
//
// Os passos 3, 4 e 5 decidem por AUSÊNCIA — pasta registrada que o banco não tem vira
// pedido de análise, poda ou desaparece do acessador. Com o banco inacessível toda
// consulta ao catálogo falha, o mapa sai vazio, e "não está no banco" passa a valer para
// TODO objeto registrado. O `--confirm` então apagava o registro inteiro, e o passo 5
// reescrevia o view.gen.go sem view nenhuma — quebrando a compilação de quem cita
// `core.View.X`.
//
// O critério é o mesmo do `conferido` da conferência de pendência: sem leitura confiável,
// não se age sobre ausência. O que FOI lido continua valendo, porque é evidência positiva.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/config"
)

// estadoComBancoInacessivel aponta para uma porta fechada em loopback. É o cenário que
// interessa e não precisa de container: a conexão é recusada de imediato.
func estadoComBancoInacessivel(pastaEspecial string) config.ConfigState {
	return config.ConfigState{
		ActiveClient:  "local",
		ActiveDialect: "mysql",
		Config: &config.Config{
			Connections: map[string]config.ConnConfig{
				"local": {
					Dialect:  "mysql",
					Host:     "127.0.0.1",
					Port:     "1", // porta reservada: nada escuta nela
					User:     "gokit",
					Database: "gokit",
				},
			},
			Output: config.OutputConfig{Special: pastaEspecial},
		},
	}
}

func TestSpecialMapNaoPodaQuandoALeituraFalha(t *testing.T) {
	raiz := t.TempDir()
	especial := filepath.Join(raiz, "special")

	// Uma view registrada com definição real de um dialeto, e outra sem conteúdo em
	// nenhum. Com leitura boa a segunda seria podada; com leitura falha, nenhuma pode.
	comConteudo := filepath.Join(especial, "views", "vw_saldo")
	semConteudo := filepath.Join(especial, "views", "vw_vazia")
	for _, pasta := range []string{comConteudo, semConteudo} {
		if err := os.MkdirAll(pasta, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(comConteudo, "sqlserver.sql"),
		[]byte("SELECT 1 AS saldo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Só comentário: o `temConteudo` ignora, então esta pasta é candidata a poda.
	if err := os.WriteFile(filepath.Join(semConteudo, "mysql.sql"),
		[]byte("-- pedido de análise\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// --confirm: é a execução que escreve e apaga. Sem ela o defeito não se manifesta.
	if err := SpecialMap(raiz, estadoComBancoInacessivel(especial), true); err != nil {
		t.Fatalf("banco fora não deveria ser erro do mapper — o registro lido segue valendo: %v", err)
	}

	for _, pasta := range []string{comConteudo, semConteudo} {
		if _, err := os.Stat(pasta); err != nil {
			t.Errorf("a pasta %s foi apagada com o banco inacessível: %v",
				filepath.Base(pasta), err)
		}
	}
}

// O acessador reflete o que o mapper leu. Com leitura falha ele não pode ser reescrito:
// um view.gen.go sem view nenhuma quebra a compilação de quem cita core.View.X.
func TestSpecialMapNaoReescreveOAcessadorQuandoALeituraFalha(t *testing.T) {
	raiz := t.TempDir()
	especial := filepath.Join(raiz, "special")
	if err := os.MkdirAll(filepath.Join(especial, "views", "vw_saldo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(especial, "views", "vw_saldo", "mysql.sql"),
		[]byte("SELECT 1 AS saldo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	core := filepath.Join(raiz, "internal", "gokit", "core")
	if err := os.MkdirAll(core, 0o755); err != nil {
		t.Fatal(err)
	}
	acessador := filepath.Join(core, arquivoDeViews)
	anterior := "package core\n\n// conteudo anterior do acessador\nvar View = struct{}{}\n"
	if err := os.WriteFile(acessador, []byte(anterior), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SpecialMap(raiz, estadoComBancoInacessivel(especial), true); err != nil {
		t.Fatalf("SpecialMap: %v", err)
	}

	depois, err := os.ReadFile(acessador)
	if err != nil {
		t.Fatalf("o acessador desapareceu: %v", err)
	}
	if !strings.Contains(string(depois), "conteudo anterior") {
		t.Errorf("o acessador foi reescrito a partir de uma leitura falha:\n%s", depois)
	}
}
