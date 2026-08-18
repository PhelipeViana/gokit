package migraterun

// Invariante 5: create_table/add_column não podem mentir.
//
// A metade fácil é detectar divergência. A metade DIFÍCIL — e a que decide se isto pode existir
// em produção — é não acusar divergência onde não há: a ida DSL → tipo físico não é injetiva, e
// uma comparação ingênua acusaria toda tabela que o próprio gokit criou.
//
// Os casos de "não deve acusar" vêm do mapa real de tipos: `boolean` vira NUMBER(1) no Oracle e
// o catálogo do MySQL devolve `tinyint`; `text` vira CLOB no Oracle e NVARCHAR(MAX) no SQL
// Server; `datetime` e `timestamp` emitem o mesmo tipo em três dos quatro.

import (
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/migration/acao"
)

// oQueOBancoDevolveria é o que o catálogo de cada dialeto reporta em `data_type` para o tipo
// físico que o gokit emite. É a tabela que faz o teste medir a volta, e não a ida.
var oQueOBancoDevolveria = map[string]map[string]string{
	"boolean": {
		"oracle": "NUMBER", "postgres": "boolean", "mysql": "tinyint", "sqlserver": "bit",
	},
	"integer": {
		"oracle": "NUMBER", "postgres": "bigint", "mysql": "bigint", "sqlserver": "bigint",
	},
	"int": {
		"oracle": "NUMBER", "postgres": "integer", "mysql": "int", "sqlserver": "int",
	},
	"decimal": {
		"oracle": "NUMBER", "postgres": "numeric", "mysql": "decimal", "sqlserver": "decimal",
	},
	"string": {
		"oracle": "VARCHAR2", "postgres": "character varying", "mysql": "varchar", "sqlserver": "nvarchar",
	},
	"char": {
		"oracle": "CHAR", "postgres": "bpchar", "mysql": "char", "sqlserver": "nchar",
	},
	"text": {
		"oracle": "CLOB", "postgres": "text", "mysql": "longtext", "sqlserver": "nvarchar",
	},
	"date": {
		"oracle": "DATE", "postgres": "date", "mysql": "date", "sqlserver": "date",
	},
	"timestamp": {
		"oracle": "TIMESTAMP(6)", "postgres": "timestamp without time zone", "mysql": "datetime", "sqlserver": "datetime2",
	},
	"datetime": {
		"oracle": "TIMESTAMP(6)", "postgres": "timestamp with time zone", "mysql": "datetime", "sqlserver": "datetime2",
	},
	"binary": {
		"oracle": "BLOB", "postgres": "bytea", "mysql": "longblob", "sqlserver": "varbinary",
	},
}

// O teste que decide se isto pode ir para produção: coluna que o gokit criou não pode ser
// acusada de divergir da própria declaração, em nenhum dos quatro dialetos.
func TestColunaCriadaPeloGokitNaoAcusaDivergencia(t *testing.T) {
	for tipo, porDialeto := range oQueOBancoDevolveria {
		for _, dialeto := range []string{"oracle", "postgres", "mysql", "sqlserver"} {
			declarada := acao.ColunaDefinicao{Name: "coluna", Type: tipo, Length: 100, Precision: 15, Scale: 4}
			real := columnMeta{DataType: porDialeto[dialeto], Length: 100}
			if diferenca := divergenciaDaColuna(dialeto, declarada, real); diferenca != "" {
				t.Errorf("FALSO POSITIVO: %s em %s — o banco devolve %q e acusou: %s",
					tipo, dialeto, porDialeto[dialeto], diferenca)
			}
		}
	}
}

// E o outro lado: divergência de verdade tem de ser acusada, nos quatro.
func TestTipoIncompativelEhAcusado(t *testing.T) {
	casos := []struct {
		declarado string
		noBanco   string
	}{
		{"integer", "varchar"},   // numérico declarado sobre coluna textual
		{"string", "int"},        // textual declarado sobre coluna numérica
		{"date", "varchar"},      // data declarada sobre texto
		{"timestamp", "date"},    // temporal com hora sobre coluna só-data
		{"binary", "varchar"},    // binário sobre texto
		{"decimal", "timestamp"}, // numérico sobre temporal
	}
	for _, caso := range casos {
		for _, dialeto := range []string{"oracle", "postgres", "mysql", "sqlserver"} {
			declarada := acao.ColunaDefinicao{Name: "coluna", Type: caso.declarado, Length: 50, Precision: 15, Scale: 2}
			real := columnMeta{DataType: caso.noBanco, Length: 50}
			if divergenciaDaColuna(dialeto, declarada, real) == "" {
				t.Errorf("%s declarado sobre %s em %s deveria acusar divergência",
					caso.declarado, caso.noBanco, dialeto)
			}
		}
	}
}

// `date` × `timestamp` é a distinção que o bucket PRESERVA de propósito: DATE sem hora é DATE
// nos quatro, e confundir os dois perderia a hora gravada em silêncio.
func TestDataNaoSeConfundeComTemporal(t *testing.T) {
	for _, dialeto := range []string{"oracle", "postgres", "mysql", "sqlserver"} {
		data := acao.ColunaDefinicao{Name: "c", Type: "date"}
		if divergenciaDaColuna(dialeto, data, columnMeta{DataType: oQueOBancoDevolveria["timestamp"][dialeto]}) == "" {
			t.Errorf("em %s, date declarado sobre coluna temporal deveria acusar", dialeto)
		}
	}
}

// Tipo que o mapa não conhece não vira acusação: inventar seria pior do que calar.
func TestTipoDesconhecidoNaoAcusa(t *testing.T) {
	declarada := acao.ColunaDefinicao{Name: "c", Type: "tipo_que_nao_existe"}
	if diferenca := divergenciaDaColuna("oracle", declarada, columnMeta{DataType: "NUMBER"}); diferenca != "" {
		t.Errorf("tipo desconhecido no DSL não deveria acusar: %s", diferenca)
	}
	conhecida := acao.ColunaDefinicao{Name: "c", Type: "integer"}
	if diferenca := divergenciaDaColuna("oracle", conhecida, columnMeta{DataType: "SDO_GEOMETRY"}); diferenca != "" {
		t.Errorf("tipo desconhecido no banco não deveria acusar: %s", diferenca)
	}
}

// Tamanho menor no banco é AVISO, nunca erro — o leitor promove texto acima de 4000 para
// Text(), então o tamanho legitimamente difere depois de uma volta pelo import.
func TestTamanhoMenorEhAvisoENaoErro(t *testing.T) {
	declarada := acao.ColunaDefinicao{Name: "nome", Type: "string", Length: 50}
	if tamanhoInsuficiente(declarada, columnMeta{DataType: "varchar", Length: 30}) == "" {
		t.Error("coluna mais curta no banco deveria gerar aviso")
	}
	if tamanhoInsuficiente(declarada, columnMeta{DataType: "varchar", Length: 50}) != "" {
		t.Error("tamanho igual não é aviso")
	}
	if tamanhoInsuficiente(declarada, columnMeta{DataType: "varchar", Length: 300}) != "" {
		t.Error("coluna MAIS LARGA no banco não é problema")
	}
	// E não é divergência de tipo — o run não pode parar por isso.
	if divergenciaDaColuna("postgres", declarada, columnMeta{DataType: "varchar", Length: 30}) != "" {
		t.Error("tamanho menor não deveria virar divergência de tipo")
	}
}

// Comprimento zero (o que o catálogo devolve para tipo não textual) não gera aviso.
func TestTamanhoZeroNaoGeraAviso(t *testing.T) {
	declarada := acao.ColunaDefinicao{Name: "n", Type: "integer"}
	if tamanhoInsuficiente(declarada, columnMeta{DataType: "bigint", Length: 0}) != "" {
		t.Error("comprimento 0 não é comparável")
	}
}

// O bucket precisa cobrir toda família que o leitor produz. Família sem bucket devolve vazio,
// e vazio DESLIGA a comparação — então uma família nova sem entrada aqui faria o invariante
// parar de valer em silêncio para aquele tipo.
func TestTodaFamiliaDoLeitorTemBucket(t *testing.T) {
	familias := []string{
		"inteiro", "numero", "booleano", "texto", "char", "texto_longo",
		"guid", "binario", "data", "timestamp", "datahora",
	}
	for _, familia := range familias {
		if bucketDoTipo(familia) == "" {
			t.Errorf("família %q do leitor não tem bucket — a comparação fica desligada para ela", familia)
		}
	}
}

// A mensagem tem de trazer as DUAS formas: sem os dois lados, o erro só transfere a dúvida.
func TestMensagemTrazAsDuasFormas(t *testing.T) {
	declarada := acao.ColunaDefinicao{Name: "eh_militar", Type: "boolean"}
	diferenca := divergenciaDaColuna("postgres", declarada, columnMeta{DataType: "character varying"})
	if diferenca == "" {
		t.Fatal("deveria acusar")
	}
	for _, esperado := range []string{"eh_militar", "character varying", "boolean"} {
		if !strings.Contains(diferenca, esperado) {
			t.Errorf("a mensagem deveria citar %q, veio: %s", esperado, diferenca)
		}
	}
}

// Todo tipo que o gokit EMITE tem de ser classificável na volta, nos quatro dialetos.
//
// Família vazia desliga a comparação do invariante 5 — e, pior, faz o import cair no default e
// devolver `.Text()`. Foi assim que se descobriu que `familiaDoTipo` não conhecia `longblob`,
// que é exatamente o que o gokit emite para `.Binary()` no MySQL: ele criava a coluna e não
// sabia lê-la de volta, trocando binário por texto sem avisar.
func TestTodoTipoEmitidoEhClassificavelNaVolta(t *testing.T) {
	tipos := []string{
		"int", "integer", "string", "char", "text",
		"boolean", "decimal", "date", "datetime", "timestamp", "binary",
	}
	for _, tipo := range tipos {
		for _, dialeto := range []string{"oracle", "postgres", "mysql", "sqlserver"} {
			declarada := acao.ColunaDefinicao{Name: "c", Type: tipo, Length: 50, Precision: 15, Scale: 2}
			fisico := columnTypeSQL(dialeto, declarada)
			if fisico == "" {
				t.Errorf("%s em %s não tem tipo físico", tipo, dialeto)
				continue
			}
			if familiaDoTipo(fisico, dialeto) == "" {
				t.Errorf("%s em %s emite %q, que o leitor NÃO classifica", tipo, dialeto, fisico)
			}
		}
	}
}
