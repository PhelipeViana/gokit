package migraterun

import "testing"

// O que os quatro catálogos devolveram para a MESMA tabela legada, capturado com
// `gokit migrate scan --detail` contra os containers em 13/08/2026. É fixture de
// medição, não invenção: cada linha é o que o banco realmente disse.
//
//	CREATE TABLE legado_contratos (
//	  contrato_id  INT identity, numero VARCHAR(40) NOT NULL, descricao TEXT NULL,
//	  valor DECIMAL(12,2) NOT NULL, assinado_em DATE NULL, criado_em TIMESTAMP NULL,
//	  ativo CHAR(1) NOT NULL, user_id BIGINT NULL)
var lidoDosQuatro = map[string][]ColunaDoBanco{
	// coluna                mysql                                        postgres                                                oracle                                             sqlserver
	"contrato_id": {
		{Tipo: "int", Precisao: 10},
		{Tipo: "integer", Precisao: 32},
		{Tipo: "number", Precisao: 10},
		{Tipo: "int", Precisao: 10},
	},
	"numero": {
		{Tipo: "varchar", Tamanho: 40},
		{Tipo: "character varying", Tamanho: 40},
		{Tipo: "varchar2", Tamanho: 40},
		{Tipo: "nvarchar", Tamanho: 40},
	},
	"descricao": {
		{Tipo: "text", Tamanho: 65535, Nulo: true},
		{Tipo: "text", Nulo: true},
		{Tipo: "clob", Nulo: true},
		{Tipo: "nvarchar", Tamanho: -1, Nulo: true},
	},
	"valor": {
		{Tipo: "decimal", Precisao: 12, Escala: 2},
		{Tipo: "numeric", Precisao: 12, Escala: 2},
		{Tipo: "number", Precisao: 12, Escala: 2},
		{Tipo: "decimal", Precisao: 12, Escala: 2},
	},
	"assinado_em": {
		{Tipo: "date", Nulo: true},
		{Tipo: "date", Nulo: true},
		{Tipo: "date", Nulo: true},
		{Tipo: "date", Nulo: true},
	},
	"criado_em": {
		{Tipo: "timestamp", Nulo: true},
		{Tipo: "timestamp without time zone", Nulo: true},
		{Tipo: "timestamp(6)", Escala: 6, Nulo: true},
		{Tipo: "datetime", Nulo: true},
	},
	"ativo": {
		{Tipo: "char", Tamanho: 1},
		{Tipo: "character", Tamanho: 1},
		{Tipo: "char", Tamanho: 1},
		{Tipo: "char", Tamanho: 1},
	},
	"user_id": {
		{Tipo: "bigint", Precisao: 19, Nulo: true},
		{Tipo: "bigint", Precisao: 64, Nulo: true},
		{Tipo: "number", Precisao: 19, Nulo: true},
		{Tipo: "bigint", Precisao: 19, Nulo: true},
	},
}

// A ordem importa: é a ordem das leituras em lidoDosQuatro. O pacote já tem um
// `dialetos` em outra ordem, e reusá-lo trocaria os rótulos das mensagens.
var dialetosDaLeitura = []string{"mysql", "postgres", "oracle", "sqlserver"}

// É este o teste que sustenta o `migrate import`: se os quatro não convergirem
// para a MESMA declaração, importar um schema legado produz um corpus preso ao
// banco de onde saiu, e o corpus único deixa de existir.
func TestOsQuatroDialetosConvergemNaMesmaDeclaracao(t *testing.T) {
	for coluna, leituras := range lidoDosQuatro {
		if len(leituras) != len(dialetosDaLeitura) {
			t.Fatalf("%s: fixture com %d leituras, esperava %d", coluna, len(leituras), len(dialetosDaLeitura))
		}
		esperado := declaracaoDaColuna(leituras[0], dialetosDaLeitura[0])
		for posicao, leitura := range leituras[1:] {
			obtido := declaracaoDaColuna(leitura, dialetosDaLeitura[posicao+1])
			if obtido != esperado {
				t.Errorf("%s divergiu: %s deu %q, %s deu %q",
					coluna, dialetosDaLeitura[0], esperado, dialetosDaLeitura[posicao+1], obtido)
			}
		}
	}
}

// As três armadilhas medidas, cada uma travada em separado — sem isso, uma
// mudança na tabela de famílias poderia reintroduzir qualquer delas e o teste de
// convergência acima continuaria passando (os quatro errariam junto).
func TestArmadilhasDaTraducao(t *testing.T) {
	casos := []struct {
		nome     string
		coluna   ColunaDoBanco
		dialeto  string
		esperado string
	}{
		{"precisão de inteiro do Postgres é em bits, não dígitos",
			ColunaDoBanco{Tipo: "integer", Precisao: 32}, "postgres", ".Integer()"},
		{"bigint do Postgres também",
			ColunaDoBanco{Tipo: "bigint", Precisao: 64}, "postgres", ".Integer()"},
		{"escala do TIMESTAMP do Oracle é fração de segundo, não decimal",
			ColunaDoBanco{Tipo: "timestamp(6)", Escala: 6}, "oracle", ".Timestamp()"},
		{"TEXT do MySQL declara 65535 e não cabe em varchar dos quatro",
			ColunaDoBanco{Tipo: "text", Tamanho: 65535}, "mysql", ".Text()"},
		{"NVARCHAR(MAX) do SQL Server chega como tamanho -1",
			ColunaDoBanco{Tipo: "nvarchar", Tamanho: -1}, "sqlserver", ".Text()"},
		{"varchar acima do teto do Oracle vira Text",
			ColunaDoBanco{Tipo: "varchar", Tamanho: 8000}, "mysql", ".Text()"},
		{"NUMBER com escala 0 é inteiro, que é como o Oracle guarda inteiro",
			ColunaDoBanco{Tipo: "number", Precisao: 10, Escala: 0}, "oracle", ".Integer()"},
		{"tipo desconhecido degrada para texto, visível no arquivo",
			ColunaDoBanco{Tipo: "geography"}, "sqlserver", ".Text()"},

		// Estes três só apareceram ao ler um schema real de 755 tabelas; nenhum
		// existia na tabela sintética que eu tinha criado para o teste.
		{"timestamp do SQL Server é rowversion, um binário — não é temporal",
			ColunaDoBanco{Tipo: "timestamp"}, "sqlserver", ".Binary()"},
		{"e o mesmo nome nos outros três continua sendo temporal",
			ColunaDoBanco{Tipo: "timestamp"}, "mysql", ".Timestamp()"},
		{"uniqueidentifier não informa tamanho e cairia em Char(1)",
			ColunaDoBanco{Tipo: "uniqueidentifier"}, "sqlserver", ".Varchar(36)"},
		{"xml existe no schema real e não tem tipo no DSL",
			ColunaDoBanco{Tipo: "xml"}, "sqlserver", ".Text()"},
		{"money é numérico com escala",
			ColunaDoBanco{Tipo: "money", Precisao: 19, Escala: 4}, "sqlserver", ".Decimal(19, 4)"},
	}
	for _, caso := range casos {
		if obtido := tipoDoDSL(caso.coluna, caso.dialeto); obtido != caso.esperado {
			t.Errorf("%s: veio %q, esperava %q", caso.nome, obtido, caso.esperado)
		}
	}
}

// Nulidade é escrita sempre, nas duas formas: ler um schema legado não pode mudar
// a nulidade de uma coluna por omissão.
func TestNulidadeEExplicita(t *testing.T) {
	if obtido := declaracaoDaColuna(ColunaDoBanco{Tipo: "varchar", Tamanho: 40, Nulo: true}, "mysql"); obtido != ".Varchar(40).Nullable()" {
		t.Errorf("nulável veio %q", obtido)
	}
	if obtido := declaracaoDaColuna(ColunaDoBanco{Tipo: "varchar", Tamanho: 40}, "mysql"); obtido != ".Varchar(40).NotNull()" {
		t.Errorf("não nulável veio %q", obtido)
	}
}

// DateTime() e Timestamp() do DSL emitem o MESMO tipo em três dos quatro bancos,
// então a volta não pode distingui-los: só tipo COM FUSO vira DateTime(), porque é
// o único caso realmente distinguível (TIMESTAMPTZ do Postgres).
func TestTemporalComHoraColapsaEmTimestamp(t *testing.T) {
	colapsam := []string{"timestamp", "timestamp without time zone", "timestamp(6)", "datetime", "datetime2", "smalldatetime"}
	for _, tipo := range colapsam {
		if obtido := tipoDoDSL(ColunaDoBanco{Tipo: tipo}, "mysql"); obtido != ".Timestamp()" {
			t.Errorf("%s deveria colapsar em Timestamp, veio %q", tipo, obtido)
		}
	}
	comFuso := []string{"timestamp with time zone", "timestamptz", "datetimeoffset"}
	for _, tipo := range comFuso {
		if obtido := tipoDoDSL(ColunaDoBanco{Tipo: tipo}, "postgres"); obtido != ".DateTime()" {
			t.Errorf("%s tem fuso e deveria virar DateTime, veio %q", tipo, obtido)
		}
	}
}

// Como os quatro catálogos devolvem o MESMO default, capturado com
// `migrate scan --detail` contra os containers. O de "agora" é o que divergia: o
// SQL Server chama getdate() e os outros três CURRENT_TIMESTAMP.
func TestDefaultConvergeNosQuatro(t *testing.T) {
	casos := []struct {
		nome     string
		leituras map[string]string // dialeto -> valor cru do catálogo
		esperado string
	}{
		{"literal de texto", map[string]string{
			"mysql": "X", "postgres": "'X'::character varying", "oracle": "'X' ", "sqlserver": "('X')",
		}, "X"},
		{"literal numérico", map[string]string{
			"mysql": "0", "postgres": "0", "oracle": "0 ", "sqlserver": "((0))",
		}, "0"},
		{"instante atual", map[string]string{
			"mysql": "CURRENT_TIMESTAMP", "postgres": "CURRENT_TIMESTAMP",
			"oracle": "CURRENT_TIMESTAMP ", "sqlserver": "(getdate())",
		}, "CURRENT_TIMESTAMP"},
		{"now() do Postgres é o mesmo instante", map[string]string{
			"postgres": "now()",
		}, "CURRENT_TIMESTAMP"},
	}
	for _, caso := range casos {
		for dialeto, cru := range caso.leituras {
			if obtido := normalizaDefault(dialeto, cru, false); obtido != caso.esperado {
				t.Errorf("%s (%s): %q virou %q, esperava %q", caso.nome, dialeto, cru, obtido, caso.esperado)
			}
		}
	}
}

// A sequência de identidade não é default: declará-la daria à coluna um default
// que ela não tem, ao lado do AutoIncrement.
func TestSequenciaDeIdentidadeNaoEDefault(t *testing.T) {
	if obtido := normalizaDefault("postgres", "nextval('zz_def_id_seq'::regclass)", false); obtido != "" {
		t.Errorf("nextval deveria ser descartado, veio %q", obtido)
	}
	if obtido := normalizaDefault("sqlserver", "((0))", true); obtido != "" {
		t.Errorf("coluna de identidade não deveria ter default, veio %q", obtido)
	}
}

// A distinção decide entre .Default() e .DefaultExpr(): escrever CURRENT_TIMESTAMP
// como literal gravaria a string "CURRENT_TIMESTAMP" na coluna.
func TestExpressaoSeparadaDeLiteral(t *testing.T) {
	expressoes := []string{"CURRENT_TIMESTAMP", "abs(-1)", "1+1"}
	for _, valor := range expressoes {
		if !EhExpressao(valor) {
			t.Errorf("%q deveria ser tratado como expressão", valor)
		}
	}
	literais := []string{"X", "0", "texto com espaco", ""}
	for _, valor := range literais {
		if EhExpressao(valor) {
			t.Errorf("%q deveria ser tratado como literal", valor)
		}
	}
}
