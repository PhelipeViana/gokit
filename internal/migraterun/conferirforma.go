package migraterun

// Invariante 5: `create_table` e `add_column` não podem mentir.
//
// Se o objeto já existe mas DIVERGE do declarado, é erro — não `return nil`. Isso esteve
// escrito e não implementado: o executor fazia `if exists { return nil }` sem comparar coluna
// nenhuma.
//
// O que o no-op silencioso custa não é a mensagem confusa. É isto: se a tabela divergente não
// tiver índice declarado, o run termina **em verde**, a migration é registrada com checksum
// batendo, e ORM, factory e seeder passam a gerar contra uma forma que o banco não tem. O erro
// chega na mão de quem consome, longe da causa. Medido: banco com `cidades(id, nome)` e corpus
// declarando `cidades(cidade_id, uf, …)` passou pelo create_table e morreu dois passos depois,
// culpando a criação de um índice.
//
// ## O que é comparado, e por que não mais que isso
//
// **Presença da coluna declarada** — é a checagem com dentes, e a única sem ambiguidade:
// coluna que o corpus declara e o banco não tem é mentira em qualquer dialeto.
//
// **Família do tipo, em bucket grosso.** Comparar o tipo do catálogo com o do DSL diretamente
// produz falso positivo em todos os dialetos, porque a ida não é injetiva: `boolean` vira
// NUMBER(1) no Oracle e o catálogo do MySQL devolve `tinyint` para BOOLEAN; `text` vira CLOB no
// Oracle e NVARCHAR(MAX) no SQL Server; `datetime` e `timestamp` emitem o MESMO tipo físico em
// três dos quatro. Então os DOIS lados passam pela mesma normalização — o declarado vira tipo
// físico por `columnTypeSQL`, e aí ambos por `familiaDoTipo` — e o resultado é reduzido a um
// bucket que só distingue o que é REALMENTE distinguível.
//
// **Nulidade NÃO é comparada.** Seria legítima, mas o próprio gokit produz a divergência: o
// SQL Server não promove coluna de PK a NOT NULL (ver a tabela de dialetos), e nulidade não
// está no cache de schema. Errar aqui bloquearia migration correta.
//
// **Tamanho menor no banco é AVISO, não erro.** `Varchar(50)` declarado sobre coluna de 30 no
// banco quebra em tempo de execução, então precisa aparecer — mas o leitor promove texto acima
// de 4000 para `Text()`, então o tamanho legitimamente difere depois de uma volta pelo import,
// e transformar isso em erro pararia corpus gerado pelo próprio gokit.
//
// **Coluna a mais no banco não é divergência:** a declaração não mentiu sobre ela. Schema
// legado tem coluna extra em toda parte, e recusar impediria o gokit de conviver com tabela que
// ele não criou.

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// bucketDoTipo reduz a família ao que é distinguível entre os quatro dialetos.
//
// Cada agrupamento tem razão medida:
//
//	numerico  inteiro/numero/booleano — Oracle guarda boolean em NUMBER, MySQL em tinyint
//	textual   texto/char/texto_longo/guid — text vira CLOB no Oracle e NVARCHAR(MAX) no SQL
//	          Server; guid não tem tipo próprio no DSL e sai como VARCHAR(36)
//	temporal  timestamp/datahora — o DSL emite o mesmo tipo para DateTime() e Timestamp() em
//	          três dos quatro, então distinguir aqui seria adivinhar
//	data      sozinho: DATE sem hora é DATE nos quatro, e a distinção importa
//	binario   sozinho
func bucketDoTipo(familia string) string {
	switch familia {
	case "inteiro", "numero", "booleano":
		return "numerico"
	case "texto", "char", "texto_longo", "guid":
		return "textual"
	case "timestamp", "datahora":
		return "temporal"
	case "data":
		return "data"
	case "binario":
		return "binario"
	}
	return ""
}

// bucketDoDeclarado devolve o bucket do tipo que o DSL emitiria NESTE dialeto.
//
// Passa pelo `columnTypeSQL` de propósito: é o mesmo caminho que criaria a coluna, então a
// comparação é contra o que o gokit realmente escreveria, e não contra uma segunda tradução
// que poderia divergir dela.
func bucketDoDeclarado(dialect string, coluna acao.ColunaDefinicao) string {
	fisico := columnTypeSQL(dialect, coluna)
	if fisico == "" {
		// Tipo que o mapa não conhece: não há o que comparar, e inventar seria pior.
		return ""
	}
	return bucketDoTipo(familiaDoTipo(fisico, dialect))
}

// divergenciaDaColuna descreve a divergência, ou vazio se a coluna está de acordo.
func divergenciaDaColuna(dialect string, declarada acao.ColunaDefinicao, real columnMeta) string {
	esperado := bucketDoDeclarado(dialect, declarada)
	obtido := bucketDoTipo(familiaDoTipo(real.DataType, dialect))
	if esperado == "" || obtido == "" || esperado == obtido {
		return ""
	}
	return i18n.Tf("run_shape_type_diff", declarada.Name,
		strings.ToLower(strings.TrimSpace(real.DataType)), obtido, declarada.Type, esperado)
}

// conferirFormaDaTabela é o invariante 5 para o create_table de tabela que já existe.
func conferirFormaDaTabela(ctx context.Context, db *sql.DB, cache *schemaCache, dialect, table string, declaradas []acao.ColunaDefinicao) error {
	if err := cache.load(ctx, db); err != nil {
		return err
	}
	reais := cache.tables[strings.ToLower(table)]

	var ausentes, divergentes, estreitas []string
	for _, declarada := range declaradas {
		real, existe := reais[strings.ToLower(declarada.Name)]
		if !existe {
			ausentes = append(ausentes, declarada.Name)
			continue
		}
		if diferenca := divergenciaDaColuna(dialect, declarada, real); diferenca != "" {
			divergentes = append(divergentes, diferenca)
		}
		if estreita := tamanhoInsuficiente(declarada, real); estreita != "" {
			estreitas = append(estreitas, estreita)
		}
	}
	sort.Strings(ausentes)
	sort.Strings(divergentes)

	if len(estreitas) > 0 {
		avisarFormaEstreita(table, estreitas)
	}
	if len(ausentes) == 0 && len(divergentes) == 0 {
		return nil
	}

	var detalhe strings.Builder
	if len(ausentes) > 0 {
		detalhe.WriteString(i18n.Tf("run_shape_missing", strings.Join(ausentes, ", ")))
	}
	for _, linha := range divergentes {
		if detalhe.Len() > 0 {
			detalhe.WriteString("\n")
		}
		detalhe.WriteString("  " + linha)
	}
	return cliui.NewUserError(
		i18n.Tf("run_shape_diverged", table, detalhe.String()),
		i18n.T("run_shape_diverged_fix"),
	)
}

// conferirColunaDeclarada é o invariante 5 para o add_column de coluna que já existe.
func conferirColunaDeclarada(ctx context.Context, db *sql.DB, cache *schemaCache, dialect, table string, declarada acao.ColunaDefinicao) error {
	real, existe, err := cache.column(ctx, db, table, declarada.Name)
	if err != nil {
		return err
	}
	if !existe {
		// Chamado só quando o hasColumn disse que existe. Se sumiu no caminho, deixa o
		// ALTER seguinte falar por si.
		return nil
	}
	if estreita := tamanhoInsuficiente(declarada, real); estreita != "" {
		avisarFormaEstreita(table, []string{estreita})
	}
	diferenca := divergenciaDaColuna(dialect, declarada, real)
	if diferenca == "" {
		return nil
	}
	return cliui.NewUserError(
		i18n.Tf("run_shape_diverged", table, "  "+diferenca),
		i18n.T("run_shape_diverged_fix"),
	)
}

// tamanhoInsuficiente aponta coluna de texto mais CURTA no banco do que o declarado.
func tamanhoInsuficiente(declarada acao.ColunaDefinicao, real columnMeta) string {
	if declarada.Length <= 0 || real.Length <= 0 {
		return ""
	}
	if int64(declarada.Length) <= real.Length {
		return ""
	}
	return i18n.Tf("run_shape_narrower", declarada.Name, real.Length, declarada.Length)
}

// formasEstreitas acumula os avisos de coluna mais curta, para o relatório do fim do run.
var formasEstreitas []string

func avisarFormaEstreita(tabela string, avisos []string) {
	for _, aviso := range avisos {
		formasEstreitas = append(formasEstreitas, fmt.Sprintf("%s.%s", strings.ToLower(tabela), aviso))
	}
}

// FormasEstreitas devolve e ZERA a lista acumulada.
func FormasEstreitas() []string {
	saida := formasEstreitas
	formasEstreitas = nil
	return saida
}
