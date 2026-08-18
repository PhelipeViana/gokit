package migraterun

// Tradução do tipo do banco para o DSL das migrations.
//
// A premissa do gokit é um corpus único que serve aos quatro bancos. Uma migration
// gerada a partir do Oracle tem de ser idêntica à gerada a partir do Postgres para
// a mesma tabela — senão importar um schema legado produz um corpus preso ao banco
// de onde saiu, e o plugin perde a razão de existir.
//
// Três medições ditam as regras daqui (feitas com a mesma tabela criada nos quatro):
//
//  1. Precisão de inteiro NÃO é comparável. O Postgres devolve 32 e 64 para INT e
//     BIGINT — são bits. MySQL e Oracle devolvem 10 e 19, que são dígitos. Por isso
//     a família inteiro ignora precisão: `Integer()` não recebe argumento.
//
//  2. O Oracle usa data_scale para a fração de segundo: TIMESTAMP volta com
//     precisão 0 e escala 6. A regra "escala > 0 logo é decimal" transformaria todo
//     TIMESTAMP do Oracle em Decimal(0,6). Por isso a FAMÍLIA É DECIDIDA PELO NOME
//     do tipo, e a escala só é consultada dentro da família numérica.
//
//  3. O MySQL declara TEXT com tamanho 65535. Traduzir literalmente daria
//     Varchar(65535), que o Oracle recusa — VARCHAR2 para em 4000. O nome decide
//     antes do tamanho.

import (
	"fmt"
	"strings"
)

// limiteDeVarchar é o teto do VARCHAR2 do Oracle, o menor dos quatro. Coluna de
// texto acima disso vira Text(): é o único tipo que os quatro aceitam para
// conteúdo longo.
const limiteDeVarchar = 4000

// declaracaoDaColuna devolve a cadeia de métodos do DSL para uma coluna lida do
// banco, sem o `migrate.Col("nome")` inicial.
//
// A chave primária e a identidade não entram aqui: quem as conhece é a tabela, não
// a coluna isolada.
func declaracaoDaColuna(coluna ColunaDoBanco, dialect string) string {
	declaracao := tipoDoDSL(coluna, dialect)

	// DEFAULT antes das constraints inline: o Oracle recusa
	// "PRIMARY KEY DEFAULT x" com ORA-03076. O executor já monta o DDL nessa ordem,
	// e escrever na mesma ordem mantém o arquivo parecido com o que ele gera.
	if coluna.Default != "" {
		if EhExpressao(coluna.Default) {
			declaracao += fmt.Sprintf(".DefaultExpr(%q)", coluna.Default)
		} else {
			declaracao += fmt.Sprintf(".Default(%q)", coluna.Default)
		}
	}

	// Nulável é o padrão do banco, mas não do DSL: escrever explicitamente evita
	// que a leitura de um schema legado mude a nulidade sem querer.
	if coluna.Nulo {
		declaracao += ".Nullable()"
	} else {
		declaracao += ".NotNull()"
	}
	return declaracao
}

// tipoDoDSL escolhe o método de tipo. É aqui que os quatro dialetos convergem.
func tipoDoDSL(coluna ColunaDoBanco, dialect string) string {
	nome := familiaDoTipo(coluna.Tipo, dialect)

	switch nome {
	case "texto_longo":
		return ".Text()"
	case "texto":
		tamanho := coluna.Tamanho
		// O SQL Server devolve -1 para NVARCHAR(MAX), e o MySQL devolve o tamanho
		// real de um TEXT. Nos dois casos não cabe em varchar dos quatro.
		if tamanho <= 0 || tamanho > limiteDeVarchar {
			return ".Text()"
		}
		return fmt.Sprintf(".Varchar(%d)", tamanho)
	case "char":
		tamanho := coluna.Tamanho
		if tamanho <= 0 {
			tamanho = 1
		}
		if tamanho > limiteDeVarchar {
			return ".Text()"
		}
		return fmt.Sprintf(".Char(%d)", tamanho)
	case "numero":
		// Só aqui a escala é consultada. Escala 0 é inteiro — é como o Oracle
		// guarda todo inteiro, em NUMBER(n,0).
		if coluna.Escala > 0 {
			precisao := coluna.Precisao
			if precisao <= 0 {
				precisao = coluna.Escala + 10
			}
			return fmt.Sprintf(".Decimal(%d, %d)", precisao, coluna.Escala)
		}
		return ".Integer()"
	case "inteiro":
		return ".Integer()"
	case "booleano":
		return ".Boolean()"
	case "data":
		return ".Date()"
	case "datahora":
		return ".DateTime()"
	case "timestamp":
		return ".Timestamp()"
	case "binario":
		return ".Binary()"
	case "guid":
		// GUID não tem tipo próprio no DSL, e o catálogo não informa tamanho para
		// ele. 36 é o comprimento da forma textual com hífens.
		return ".Varchar(36)"
	}

	// Tipo que não reconhecemos vira texto: é o que todos os quatro aceitam. Cair
	// no Text() é degradação visível no arquivo gerado, não erro silencioso — quem
	// revisa a migration vê e corrige.
	return ".Text()"
}

// familiaDoTipo reduz os nomes dos quatro catálogos a uma família. O nome vem em
// minúsculas; a lista cobre o que os quatro emitem para os tipos do DSL.
//
// O dialeto entra por UMA exceção, medida num schema real de 755 tabelas: no SQL
// Server, `timestamp` NÃO é temporal — é sinônimo de `rowversion`, um binário de 8
// bytes que o servidor atualiza sozinho. Tratá-lo como os outros três faria a
// migration gerada criar um DATETIME2 no lugar de um rowversion.
func familiaDoTipo(tipo, dialect string) string {
	tipo = strings.TrimSpace(strings.ToLower(tipo))
	// O Oracle carrega a precisão no nome: `timestamp(6)`, `timestamp(6) with time
	// zone`. O Postgres carrega o fuso: `timestamp without time zone`.
	base := tipo
	if abre := strings.Index(base, "("); abre >= 0 {
		base = base[:abre]
	}
	base = strings.TrimSpace(base)

	// A exceção do SQL Server vem antes de tudo, senão o `timestamp` dele cairia na
	// família temporal junto com o dos outros três.
	if dialect == "sqlserver" && (base == "timestamp" || base == "rowversion") {
		return "binario"
	}

	// Temporal com hora colapsa em Timestamp(), e a razão é medida: o DSL emite o
	// MESMO tipo para DateTime() e Timestamp() em três dos quatro bancos —
	// DATETIME no MySQL, DATETIME2 no SQL Server, TIMESTAMP no Oracle (ver o mapa
	// de tipos em runner.go). Só o Postgres os separa, mandando DateTime() para
	// TIMESTAMPTZ. Ou seja, a volta banco → DSL é irrecuperável por natureza:
	// distinguir os dois exigiria adivinhar, e adivinhar aqui faria a migration
	// gerada a partir do MySQL divergir da gerada a partir do Postgres.
	//
	// Então só o que é REALMENTE distinguível vira DateTime(): tipo com fuso.
	switch {
	case strings.Contains(base, "with time zone"), base == "timestamptz", base == "datetimeoffset":
		return "datahora"
	case strings.HasPrefix(base, "timestamp"), base == "datetime", base == "datetime2", base == "smalldatetime":
		return "timestamp"
	case base == "date":
		return "data"
	case base == "time", strings.HasPrefix(base, "time "):
		return "timestamp"
	}

	switch base {
	case "int", "integer", "int4", "int8", "int2", "bigint", "smallint", "mediumint", "tinyint", "serial", "bigserial", "smallserial":
		return "inteiro"
	case "number", "numeric", "decimal", "dec", "money", "smallmoney", "float", "double", "double precision", "real", "binary_float", "binary_double":
		return "numero"
	case "bool", "boolean", "bit":
		return "booleano"
	case "varchar", "varchar2", "nvarchar", "nvarchar2", "character varying", "varying character", "string":
		return "texto"
	case "char", "character", "nchar", "bpchar":
		return "char"
	case "uniqueidentifier", "uuid":
		return "guid"
	case "text", "clob", "nclob", "ntext", "longtext", "mediumtext", "tinytext", "long", "xmltype", "xml", "json", "jsonb":
		return "texto_longo"
	// As variantes de BLOB do MySQL estavam de fora, e `longblob` é justamente o que o
	// gokit EMITE para `.Binary()` naquele dialeto. Ou seja: ele criava a coluna e depois
	// não sabia lê-la de volta — o import caía no default e devolvia `.Text()`, trocando
	// binário por texto em silêncio. As de TEXT já estavam todas.
	case "blob", "tinyblob", "mediumblob", "longblob",
		"bytea", "binary", "varbinary", "image", "raw", "long raw", "bfile":
		return "binario"
	}
	return ""
}
