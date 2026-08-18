package migraterun

// Regressão da comparação do seed: escala decimal não é divergência.
//
// Os quatro drivers devolvem DECIMAL de formas diferentes — o do Oracle entrega float64, os
// outros três entregam TEXTO com a escala cheia. Um DECIMAL(15,4) com valor 9.001 volta
// "9.0010" do MySQL, do Postgres e do SQL Server, contra o 9.001 que o seed declara.
//
// Medido no corpus legado: `seed run` abortava em 3 dos 4 bancos sobre dado que o PRÓPRIO
// gokit havia escrito, com a mensagem "a linha já existe no banco com outro conteúdo
// (juros: banco tem \"9.0010\", seed declara \"9.001\")". O Oracle passava, o que fazia a
// falha parecer específica dos outros três.

import (
	"testing"

	"github.com/PhelipeViana/gokit/migration/acao"
)

func TestEscalaDecimalNaoEhDivergencia(t *testing.T) {
	linha := acao.Linha{"id": 10, "juros": 9.001}
	numericas := map[string]bool{"id": true, "juros": true}

	// Cada forma em que um dos quatro devolve o mesmo DECIMAL(15,4).
	for _, doBanco := range []any{
		[]byte("9.0010"), // MySQL / SQL Server
		"9.0010",         // Postgres
		9.001,            // Oracle (float64)
		[]byte("9.00100000"),
	} {
		existente := map[string]any{"id": int64(10), "juros": doBanco}
		if diff := seedDifference(linha, existente, []string{"id", "juros"}, numericas); diff != "" {
			t.Errorf("banco devolvendo %T(%v) não deveria divergir de 9.001: %s", doBanco, doBanco, diff)
		}
	}
}

// Diferença de VALOR continua sendo divergência — a tolerância é de forma, não de conteúdo.
func TestDiferencaDeValorSegueSendoDivergencia(t *testing.T) {
	linha := acao.Linha{"id": 1, "juros": 9.001}
	numericas := map[string]bool{"id": true, "juros": true}
	for _, doBanco := range []any{"9.002", "9.0011", "90.01", "-9.001", "0.9001"} {
		existente := map[string]any{"id": int64(1), "juros": doBanco}
		if diff := seedDifference(linha, existente, []string{"id", "juros"}, numericas); diff == "" {
			t.Errorf("banco com %q deveria divergir de 9.001", doBanco)
		}
	}
}

// Coluna de TEXTO não é canonizada: "1.10" e "1.1" são strings diferentes, e num código de
// texto essa diferença importa. É por isso que a tolerância depende do TIPO da coluna.
func TestColunaDeTextoNaoEhCanonizada(t *testing.T) {
	linha := acao.Linha{"codigo": "1.10"}
	semNumericas := map[string]bool{}
	existente := map[string]any{"codigo": "1.1"}
	if diff := seedDifference(linha, existente, []string{"codigo"}, semNumericas); diff == "" {
		t.Error("coluna de texto com \"1.1\" vs \"1.10\" deveria divergir")
	}
}

func TestCanonizaNumero(t *testing.T) {
	casos := map[string]string{
		"9.0010":   "9.001",
		"9.00":     "9",
		"9.":       "9.", // não é número plano: volta intacto
		"100":      "100",
		"0.0001":   "0.0001",
		"-0.0":     "0",
		"-9.100":   "-9.1",
		"0":        "0",
		"1e5":      "1e5", // expoente não é forma plana: intacto
		"Portaria": "Portaria",
		"":         "",
	}
	for entrada, esperado := range casos {
		if obtido := canonizaNumero(entrada); obtido != esperado {
			t.Errorf("canonizaNumero(%q) = %q, esperava %q", entrada, obtido, esperado)
		}
	}
}
