package migraterun

// Teto de linha do MySQL: promoção de VARCHAR/CHAR para TEXT quando a linha não cabe.
//
// O MySQL limita a DEFINIÇÃO da linha a 65 535 bytes, somando todas as colunas — é limite
// de formato, não de armazenamento, então ROW_FORMAT não resolve. Os outros três dialetos
// não têm nada equivalente, e o mesmo corpus atravessa neles inteiro.
//
// Medido no corpus legado de 754 tabelas: exatamente 4 estouram — `processo_laudo_pericia`
// e sua cópia `_excluidos_02062026` (232 524 bytes), `anamnese` (93 820) e
// `pericia_conclusao` (85 276). São tabelas de perícia médica com centenas de colunas de
// texto.
//
// A decisão é do usuário e tem preço, que por isso é REPORTADO: TEXT no MySQL não aceita
// DEFAULT literal e precisa de prefixo para ser indexado. Ou seja, nessas tabelas o MySQL
// fica com semântica diferente dos outros três — e é melhor do que não ter a tabela.
//
// A decisão é por TABELA, não por coluna: só a soma da linha diz se falta espaço. Daí isto
// morar aqui e não no columnTypeSQL, que vê uma coluna de cada vez.

import (
	"sort"
	"strings"

	"github.com/PhelipeViana/gokit/migration/acao"
)

// tetoDeLinhaNoMySQL é o limite do MySQL, com margem.
//
// A margem existe porque a conta abaixo é uma ESTIMATIVA: o MySQL cobra bytes de cabeçalho
// que variam com nulidade e formato, e errar por baixo devolve o erro 1118 que a promoção
// existe para evitar. Errar por cima só promove uma coluna a mais.
const tetoDeLinhaNoMySQL = 60000

// bytesNoMySQL estima o custo de uma coluna na definição da linha.
//
// utf8mb4 é 4 bytes por caractere — é o charset da conexão que o gokit monta, e é o que faz
// um VARCHAR(1000) custar 4000. Tipo grande (TEXT/BLOB) sai da conta: fica fora da linha, só
// o ponteiro conta, e é justamente por isso que promover resolve.
func bytesNoMySQL(coluna acao.ColunaDefinicao) int {
	tamanho := coluna.Length
	if tamanho == 0 {
		tamanho = 255
	}
	switch strings.ToLower(coluna.Type) {
	case "string", "char":
		return tamanho*4 + 2
	case "text", "binary":
		return 12
	case "integer", "datetime", "timestamp":
		return 8
	case "decimal":
		return 9
	case "int":
		return 4
	case "date":
		return 3
	case "boolean":
		return 1
	}
	return 8
}

// promoverParaTextNoMySQL devolve as colunas ajustadas e o nome das promovidas.
//
// Promove da MAIS LARGA para a mais estreita, e para assim que a linha cabe: é o que troca o
// menor número de colunas. Coluna com DEFAULT ou dentro de índice NÃO é candidata — no MySQL
// TEXT não aceita DEFAULT literal, e índice sobre TEXT exige prefixo; promover ali trocaria
// um erro por outro, mais difícil de ler.
func promoverParaTextNoMySQL(colunas []acao.ColunaDefinicao, indexadas map[string]bool) ([]acao.ColunaDefinicao, []string) {
	total := 0
	for _, coluna := range colunas {
		total += bytesNoMySQL(coluna)
	}
	if total <= tetoDeLinhaNoMySQL {
		return colunas, nil
	}

	// Índice sobre as posições candidatas, da mais larga para a mais estreita.
	type candidata struct {
		posicao int
		bytes   int
	}
	var candidatas []candidata
	for posicao, coluna := range colunas {
		tipo := strings.ToLower(coluna.Type)
		if tipo != "string" && tipo != "char" {
			continue
		}
		if strings.TrimSpace(coluna.Default) != "" {
			continue
		}
		if coluna.PrimaryKey || coluna.Unique || indexadas[strings.ToLower(coluna.Name)] {
			continue
		}
		candidatas = append(candidatas, candidata{posicao, bytesNoMySQL(coluna)})
	}
	sort.SliceStable(candidatas, func(i, j int) bool { return candidatas[i].bytes > candidatas[j].bytes })

	ajustadas := make([]acao.ColunaDefinicao, len(colunas))
	copy(ajustadas, colunas)

	var promovidas []string
	for _, c := range candidatas {
		if total <= tetoDeLinhaNoMySQL {
			break
		}
		antes := bytesNoMySQL(ajustadas[c.posicao])
		ajustadas[c.posicao].Type = "text"
		ajustadas[c.posicao].Length = 0
		total += bytesNoMySQL(ajustadas[c.posicao]) - antes
		promovidas = append(promovidas, colunas[c.posicao].Name)
	}
	return ajustadas, promovidas
}

// promocoesParaText acumula as trocas de VARCHAR/CHAR por TEXT feitas no MySQL.
//
// Variável de pacote pela mesma razão de indicesRedundantes: o executor de operação não
// carrega um coletor, e propagar um tocaria dezenas de assinaturas por causa de um aviso.
// É lida e zerada por quem inicia o run.
var promocoesParaText []string

func avisarPromocaoParaText(tabela string, colunas []string) {
	promocoesParaText = append(promocoesParaText,
		strings.ToLower(tabela)+": "+strings.Join(colunas, ", "))
}

// PromocoesParaText devolve e ZERA a lista acumulada.
func PromocoesParaText() []string {
	saida := promocoesParaText
	promocoesParaText = nil
	return saida
}

// indexadasPorTabela é o mapa tabela → colunas que o corpus indexa.
//
// Preenchido ANTES da execução, e não consultado durante: o índice pode vir de uma migration
// POSTERIOR ao create_table, e nesse caso a promoção já teria acontecido sem saber. Conhecer
// o corpus inteiro de antemão é o que evita promover justamente a coluna que vai ser
// indexada — no MySQL, índice sobre TEXT exige prefixo.
var indexadasPorTabela map[string]map[string]bool

// RegistrarIndicesDoCorpus preenche o mapa consultado pela promoção do MySQL.
func RegistrarIndicesDoCorpus(operacoes []acao.Operacao) {
	indexadasPorTabela = map[string]map[string]bool{}
	for _, operacao := range operacoes {
		switch operacao.Kind {
		case "create_index", "add_unique", "add_primary_key":
		default:
			continue
		}
		tabela := strings.ToLower(operacao.Table)
		if indexadasPorTabela[tabela] == nil {
			indexadasPorTabela[tabela] = map[string]bool{}
		}
		for _, coluna := range operacao.IndexColumns {
			indexadasPorTabela[tabela][strings.ToLower(coluna)] = true
		}
	}
}

func colunasIndexadasNoCorpus(tabela string) map[string]bool {
	return indexadasPorTabela[strings.ToLower(tabela)]
}
