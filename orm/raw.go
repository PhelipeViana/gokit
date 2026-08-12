package orm

// Bloco 6: Raw contido — o último recurso, explícito e por dialeto.
//
// Depois do bloco 5 a lista do que sobra é curta (REGEXP, JSON, hint de
// otimizador, função do próprio banco legado), mas ela existe. Sem uma porta
// declarada, quem chega nela sai da ORM inteira — e volta a escrever a consulta
// duas ou quatro vezes.
//
// As duas regras que fazem esta porta não virar buraco:
//
//  1. VALOR SÓ POR ARGUMENTO. O texto do fragmento nunca carrega valor. O autor
//     escreve {} onde o valor entra, e a ORM põe o placeholder do dialeto na
//     posição correta. Ele não poderia escrever o placeholder nem se quisesse: o
//     número depende de quantos valores as outras cláusulas vincularam antes.
//
//  2. UM DIALETO SÓ É DECLARADO, NÃO ASSUMIDO. RawPerDialect exige os quatro — faltar um
//     falha na compilação, na máquina de quem escreveu. RawOnly declara que só
//     aquele banco tem o recurso, e nos outros três falha em voz alta. O que não
//     existe é rodar num banco e devolver outra coisa no outro.
//
// Fragmento cru não é avaliável em memória: Matches devolve falso e Evaluable
// também. Esse é o sinal que a camada de regras precisa para saber que aquela
// regra só o banco sabe responder.

import (
	"fmt"
	"sort"
	"strings"
)

// marcadorDeValor é onde o valor entra no fragmento. Neutro de propósito: `?`
// colidiria com os operadores jsonb do Postgres, e qualquer forma numerada
// ($1, :1, @p1) seria específica de dialeto E de posição.
const marcadorDeValor = "{}"

// Raw é um fragmento de SQL escrito à mão.
//
// Serve nos dois papéis, porque no SQL o mesmo fragmento serve nos dois: como
// PREDICADO (dentro de Where ou Having, e aí precisa ser booleano) e como
// EXPRESSÃO escalar (dentro de Select, OrderBy ou GroupBy, e aí precisa de As).
type Raw struct {
	porDialeto map[Dialect]string
	// exclusivo, quando preenchido, é o único dialeto aceito — é o RawOnly.
	exclusivo    Dialect
	temExclusivo bool
	valores      []any
	alias        string
}

// RawPerDialect recebe o fragmento de CADA dialeto e os valores.
//
//	orm.Users.Where(orm.RawPerDialect(map[orm.Dialect]string{
//	    orm.MySQL:     "`nome` REGEXP {}",
//	    orm.Postgres:  `"nome" ~ {}`,
//	    orm.Oracle:    `REGEXP_LIKE("NOME", {})`,
//	    orm.SQLServer: "[nome] LIKE {}",
//	}, "^Ana"))
//
// Os quatro são obrigatórios, e a checagem acontece na compilação de QUALQUER um
// deles: quem desenvolve no MySQL descobre que faltou o Oracle na própria máquina,
// não no banco do cliente.
//
// Os VALORES são os mesmos para os quatro fragmentos — é o que garante que a
// pergunta seja a mesma. Isso impõe um limite: se um banco precisasse do valor em
// outro formato (um regex nos três e um padrão de LIKE no quarto, por exemplo), a
// pergunta já não é a mesma, e o caso é de RawOnly por dialeto — ou de repensar a
// consulta. RawPerDialect serve para a mesma pergunta escrita de quatro jeitos, não para
// quatro perguntas parecidas.
func RawPerDialect(porDialeto map[Dialect]string, valores ...any) Raw {
	return Raw{porDialeto: porDialeto, valores: valores}
}

// RawOnly declara um fragmento que existe em UM dialeto só.
//
//	orm.RawOnly(orm.Oracle, `REGEXP_LIKE("NOME", {})`, "^Ana")
//
// Nos outros três a compilação falha, dizendo qual dialeto o fragmento declarou.
// Falhar é o recurso: o contrário — ignorar o fragmento, ou tentar rodá-lo — daria
// resposta diferente por banco, em silêncio.
func RawOnly(dialeto Dialect, sql string, valores ...any) Raw {
	return Raw{
		porDialeto:   map[Dialect]string{dialeto: sql},
		exclusivo:    dialeto,
		temExclusivo: true,
		valores:      valores,
	}
}

// As nomeia a coluna de saída. Obrigatório para usar o fragmento em Select: o nome
// que cada banco daria a uma expressão anônima é diferente, e é pelo alias que o
// Record acha o valor.
func (r Raw) As(alias string) Raw {
	r.alias = alias
	return r
}

func (Raw) expression() {}

// active é sempre verdadeiro: fragmento cru é escrito à mão, não nasce de campo de
// formulário vazio como o Condition dinâmico.
func (Raw) active() bool { return true }

// OutputName é o alias no resultado.
func (r Raw) OutputName() string {
	if r.alias != "" {
		return r.alias
	}
	return "raw"
}

// columnField faz o Raw valer como Column, para entrar em Select, OrderBy e
// GroupBy. O Field devolvido só carrega o nome de saída — um fragmento cru não
// pertence a coluna nenhuma, e é por isso que o As importa.
func (r Raw) columnField() Field {
	return Field{Column: r.OutputName()}
}

// expressao rende o fragmento do dialeto ativo, trocando cada {} pelo placeholder
// da posição correta.
func (r Raw) expressao(ctx *compileCtx) (string, error) {
	if err := r.validar(); err != nil {
		return "", err
	}
	texto, tem := r.porDialeto[ctx.dialect]
	if !tem {
		if r.temExclusivo {
			return "", errRawDeOutroDialeto(r.exclusivo, ctx.dialect)
		}
		return "", errRawSemDialeto([]Dialect{ctx.dialect})
	}

	// A troca é sequencial, e é aqui que a regra "valor só por argumento" se paga:
	// o placeholder sai numerado pela ordem real de vinculação da pesquisa inteira.
	partes := strings.Split(texto, marcadorDeValor)
	var b strings.Builder
	for i, parte := range partes {
		b.WriteString(parte)
		if i < len(partes)-1 {
			b.WriteString(ctx.bind(r.valores[i]))
		}
	}
	return b.String(), nil
}

// validar checa o que dá para checar sem banco: cobertura de dialetos, contagem de
// marcadores e placeholder escrito à mão.
func (r Raw) validar() error {
	if len(r.porDialeto) == 0 {
		return errRawVazio()
	}

	// RawPerDialect exige os quatro. A checagem é da cobertura INTEIRA, não só do dialeto
	// que está compilando: é o que faz a falta aparecer para quem escreveu.
	if !r.temExclusivo {
		var faltando []Dialect
		for _, d := range []Dialect{MySQL, Postgres, Oracle, SQLServer} {
			if _, tem := r.porDialeto[d]; !tem {
				faltando = append(faltando, d)
			}
		}
		if len(faltando) > 0 {
			return errRawSemDialeto(faltando)
		}
	}

	// Placeholder à mão vem antes da contagem: quem escreveu ? ou $1 no lugar do {}
	// tem zero marcadores, e a mensagem de contagem esconderia a causa real.
	for _, dialeto := range dialetosOrdenados(r.porDialeto) {
		if marca := placeholderEscritoAMao(dialeto, r.porDialeto[dialeto]); marca != "" {
			return errRawComPlaceholder(dialeto, marca)
		}
	}

	// Todos os fragmentos precisam ter a MESMA quantidade de marcadores, senão os
	// valores entrariam em posições diferentes em cada banco — e o SQL continuaria
	// válido em todos eles.
	for _, dialeto := range dialetosOrdenados(r.porDialeto) {
		marcadores := strings.Count(r.porDialeto[dialeto], marcadorDeValor)
		if marcadores != len(r.valores) {
			return errRawContagemDeValores(dialeto, marcadores, len(r.valores))
		}
	}
	return nil
}

// placeholderEscritoAMao acha o placeholder de dialeto que o autor tentou escrever
// à mão. Numerá-lo certo é impossível de fora: a posição depende de quantos valores
// as outras cláusulas vincularam antes deste fragmento.
//
// O `?` só é acusado no fragmento do MySQL, onde é o placeholder de verdade e onde
// não significa mais nada. No Postgres, `?`, `?|` e `?&` são operadores de jsonb —
// acusá-los ali proibiria consulta legítima.
func placeholderEscritoAMao(dialeto Dialect, texto string) string {
	if dialeto == MySQL && strings.Contains(texto, "?") {
		return "?"
	}
	for i := 0; i < len(texto); i++ {
		switch texto[i] {
		case '$', ':':
			if i+1 < len(texto) && ehDigito(texto[i+1]) {
				return texto[i : i+2]
			}
		case '@':
			if i+2 < len(texto) && texto[i+1] == 'p' && ehDigito(texto[i+2]) {
				return texto[i : i+3]
			}
		}
	}
	return ""
}

func ehDigito(b byte) bool { return b >= '0' && b <= '9' }

// dialetosOrdenados dá ordem estável ao mapa, para que a mensagem de erro seja
// sempre a mesma — mapa em Go itera aleatório.
func dialetosOrdenados(porDialeto map[Dialect]string) []Dialect {
	saida := make([]Dialect, 0, len(porDialeto))
	for d := range porDialeto {
		saida = append(saida, d)
	}
	sort.Slice(saida, func(i, j int) bool { return saida[i] < saida[j] })
	return saida
}

func nomesDeDialetos(ds []Dialect) string {
	nomes := make([]string, 0, len(ds))
	for _, d := range ds {
		nomes = append(nomes, string(d))
	}
	return strings.Join(nomes, ", ")
}

// compileRaw é o ponto de entrada quando o fragmento está no papel de predicado.
func compileRaw(r Raw, ctx *compileCtx) (string, error) {
	texto, err := r.expressao(ctx)
	if err != nil {
		return "", err
	}
	// Parênteses porque o fragmento entra numa árvore de AND/OR: sem eles, um
	// fragmento com OR interno mudaria a precedência do Where inteiro.
	return "(" + texto + ")", nil
}

func errRawNaProjecaoSemAlias() error {
	return fmt.Errorf("orm: fragmento cru na projeção precisa de .As(\"nome\"): o nome que cada banco daria a uma expressão anônima é diferente, e é pelo alias que o Record acha o valor — vale também para GroupBy(fragmento), que projeta a chave do grupo")
}

func errRawVazio() error {
	return fmt.Errorf("orm: Raw sem nenhum fragmento; use RawPerDialect com os quatro dialetos ou RawOnly com um")
}

func errRawSemDialeto(faltando []Dialect) error {
	return fmt.Errorf("orm: RawPerDialect não declara o fragmento de %s; declare os quatro, ou use RawOnly(dialeto, sql) se o recurso existe em um só — assim os outros falham em voz alta em vez de devolver outra coisa",
		nomesDeDialetos(faltando))
}

func errRawDeOutroDialeto(declarado, ativo Dialect) error {
	return fmt.Errorf("orm: este fragmento foi declarado só para %s e a conexão ativa é %s; use RawPerDialect para cobrir os quatro, ou uma expressão do orm (Lower, Concat, Coalesce, YearOf...) que já é cross-dialect",
		declarado, ativo)
}

func errRawComPlaceholder(dialeto Dialect, marca string) error {
	return fmt.Errorf("orm: o fragmento de %s escreve o placeholder %q à mão; use %s no lugar do valor — a numeração depende do que as outras cláusulas vincularam antes e só o compilador sabe",
		dialeto, marca, marcadorDeValor)
}

func errRawContagemDeValores(dialeto Dialect, marcadores, valores int) error {
	return fmt.Errorf("orm: o fragmento de %s tem %d %s e foram informados %d valor(es); a contagem precisa bater em todos os dialetos, senão o valor entra em posição diferente em cada banco",
		dialeto, marcadores, marcadorDeValor, valores)
}
