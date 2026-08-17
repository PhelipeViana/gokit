package migraterun

// Regressão de normalização: DEFAULT numérico em coluna booleana.
//
// O corpus legado vem do SQL Server, onde a coluna é BIT e o default é `0`. Oracle
// (NUMBER(1)), MySQL e SQL Server (BIT) aceitam `DEFAULT 0`; o **Postgres recusa** — ele
// não faz a coerção implícita de integer para boolean nem no DEFAULT. Medido: o corpus de
// 754 tabelas morria em `create_config_regra` com
// "column \"eh_militar\" is of type boolean but default expression is of type integer".

import (
	"testing"

	"github.com/PhelipeViana/gokit/migration/acao"
)

func TestDefaultDeBooleanoSaiNaFormaDeCadaBanco(t *testing.T) {
	booleana := func(padrao string) acao.ColunaDefinicao {
		return acao.ColunaDefinicao{Name: "eh_militar", Type: "boolean", Default: padrao}
	}

	casos := []struct {
		padrao   string
		dialeto  string
		esperado string
	}{
		// Postgres exige a palavra; era aqui que o corpus parava.
		{"0", "postgres", "FALSE"},
		{"1", "postgres", "TRUE"},
		// MySQL tem BOOLEAN e aceita as duas formas; a palavra é a mais explícita.
		{"0", "mysql", "FALSE"},
		{"1", "mysql", "TRUE"},
		// SQL Server é BIT e T-SQL NÃO tem literal booleano: `DEFAULT FALSE` falha com
		// "Invalid column name 'FALSE'". Tem de voltar a 0/1, como no Oracle.
		{"0", "sqlserver", "0"},
		{"1", "sqlserver", "1"},
		// Oracle é NUMBER(1): o defaultSQL já devolvia 0/1 para true/false, e continua.
		{"0", "oracle", "0"},
		{"1", "oracle", "1"},
		// Quem escreveu a palavra continua chegando ao mesmo lugar.
		{"false", "postgres", "FALSE"},
		{"true", "oracle", "1"},
		// E a palavra escrita à mão também tem de virar 0/1 no SQL Server — este caminho
		// já existia antes da normalização e já estava errado; ninguém batia nele porque
		// o corpus legado escreve 0/1.
		{"false", "sqlserver", "0"},
		{"true", "sqlserver", "1"},
	}
	for _, caso := range casos {
		if obtido := columnDefaultSQL(caso.dialeto, booleana(caso.padrao)); obtido != caso.esperado {
			t.Errorf("boolean DEFAULT %q em %s: esperava %q, veio %q",
				caso.padrao, caso.dialeto, caso.esperado, obtido)
		}
	}
}

// O 0/1 só vira palavra em coluna BOOLEANA. Numa coluna numérica é um número, e trocá-lo
// mudaria o valor gravado — este é o teste que impede a normalização de vazar.
func TestDefaultNumericoDeColunaNaoBooleanaNaoMuda(t *testing.T) {
	for _, tipo := range []string{"integer", "biginteger", "decimal", "string", ""} {
		coluna := acao.ColunaDefinicao{Name: "quantidade", Type: tipo, Default: "0"}
		for _, dialeto := range []string{"postgres", "oracle", "mysql", "sqlserver"} {
			if obtido := columnDefaultSQL(dialeto, coluna); obtido != "0" {
				t.Errorf("tipo %q em %s: DEFAULT 0 deveria seguir 0, veio %q", tipo, dialeto, obtido)
			}
		}
	}
}
