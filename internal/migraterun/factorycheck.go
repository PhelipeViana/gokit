package migraterun

// Conferência das factories SEM tocar no banco.
//
// O avaliador de AST consegue produzir as N linhas em memória, e a forma da
// tabela vem do corpus. Então tudo que o banco recusaria por causa do VALOR pode
// ser dito antes: tamanho estourado, tipo incompatível, NOT NULL sem valor, CHECK
// violado e — a que só o banco pegava — chave repetida entre as linhas geradas.
//
// A diferença prática: o erro do banco aparece na linha 2 de 85, com a mensagem do
// driver, e derruba a execução inteira. Aqui aparece a lista completa, por arquivo
// e por coluna, antes de qualquer INSERT.
//
// O que NÃO dá para conferir aqui é a Reference: o valor dela vem de uma linha que
// existe no banco. Fica de fora, explicitamente.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// problemaDeValor é uma recusa que o banco faria, dita antes.
type problemaDeValor struct {
	Arquivo string
	Coluna  string
	Linha   int // índice da linha gerada, 1-based para leitura humana
	Motivo  string
}

func (p problemaDeValor) String() string {
	return fmt.Sprintf("%s: linha %d, coluna %s: %s", p.Arquivo, p.Linha, p.Coluna, p.Motivo)
}

// conferirValoresDaFactory gera as linhas da factory em memória e devolve o que o
// banco recusaria.
func conferirValoresDaFactory(atual plano, checks map[string][]string) []problemaDeValor {
	var problemas []problemaDeValor
	arquivo := filepath.Base(atual.Arquivo.Caminho)

	// Chave que precisa ser distinta entre as linhas: PK e UNIQUE. O valor gerado
	// é acumulado para achar repetição — é o erro que hoje só aparece no banco, na
	// segunda linha do lote.
	vistos := map[string]map[string]int{}
	porNome := map[string]acao.ColunaDefinicao{}
	for _, column := range atual.Forma.Columns {
		porNome[strings.ToUpper(column.Name)] = column
		if column.PrimaryKey || column.Unique {
			vistos[strings.ToUpper(column.Name)] = map[string]int{}
		}
	}

	total := quantidadeDeLinhas(atual)
	for indice := 0; indice < total; indice++ {
		for _, campo := range atual.Arquivo.Campos {
			// Reference vem do banco: não há o que conferir em memória.
			if campo.Referencia != nil || campo.Valor == nil {
				continue
			}
			fisica := atual.colunaFisica(campo.Coluna)
			column, conhecida := porNome[strings.ToUpper(fisica)]
			if !conhecida {
				continue // coluna inexistente já é acusada pela conferência de nomes
			}

			valor, err := campo.Valor(indice)
			if err != nil {
				problemas = append(problemas, problemaDeValor{arquivo, campo.Coluna, indice + 1, err.Error()})
				continue
			}
			problemas = append(problemas, conferirValor(arquivo, campo.Coluna, indice+1, valor, column,
				checks[strings.ToUpper(fisica)], vistos)...)
		}
	}
	return problemas
}

// conferirValor aplica as regras que não dependem de banco a um único valor.
func conferirValor(arquivo, coluna string, linha int, valor any, column acao.ColunaDefinicao,
	dominio []string, vistos map[string]map[string]int) []problemaDeValor {

	var problemas []problemaDeValor
	adicionar := func(motivo string) {
		problemas = append(problemas, problemaDeValor{arquivo, coluna, linha, motivo})
	}

	if valor == nil {
		if !column.Nullable {
			adicionar(i18n.T("fck_null_not_allowed"))
		}
		return problemas
	}

	// Coerção: é a mesma que o executor aplica antes do INSERT, então recusar aqui
	// é recusar exatamente o que o driver recusaria.
	convertido, err := migrationgo.CoerceValue(valor, column.Type)
	if err != nil {
		adicionar(err.Error())
		return problemas
	}

	texto, ehTexto := convertido.(string)
	if ehTexto && column.Length > 0 && len([]rune(texto)) > column.Length {
		adicionar(i18n.Tf("fck_too_long", len([]rune(texto)), column.Length))
	}

	if len(dominio) > 0 && ehTexto {
		permitido := false
		for _, opcao := range dominio {
			if strings.EqualFold(opcao, texto) {
				permitido = true
				break
			}
		}
		if !permitido {
			adicionar(i18n.Tf("fck_check_violated", texto, strings.Join(dominio, ", ")))
		}
	}

	// Repetição em coluna que exige valor distinto. O comparativo é textual porque
	// o que importa é a igualdade que o índice único do banco veria.
	if chave, exige := vistos[strings.ToUpper(column.Name)]; exige {
		assinatura := fmt.Sprintf("%v", convertido)
		if anterior, repetido := chave[assinatura]; repetido {
			adicionar(i18n.Tf("fck_duplicate", assinatura, anterior))
		} else {
			chave[assinatura] = linha
		}
	}

	return problemas
}
