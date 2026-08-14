package migraterun

// Resolução dos nomes de uma factory recém-lida.
//
// INVARIANTE: depois de passar por aqui, `Arquivo.Tabela` e `Campo.Coluna` são NOMES
// FÍSICOS. Todo o resto do motor — preservação no create, validação contra o corpus,
// montagem do INSERT — já assumia isso; o que faltava era alguém garantir.
//
// A resolução acontece num lugar só, imediatamente depois da leitura, de propósito. A
// alternativa era cada consumidor resolver antes de usar, e aí basta um esquecer para
// o defeito voltar em silêncio: identificador achatado por ToUpper casa com o nome
// físico em tabela de nome simples e falha em tabela de nome composto.

import (
	"strings"

	"github.com/PhelipeViana/gokit/internal/factorygo"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
)

// resolverNomesFisicos reescreve identificadores de catálogo para nomes físicos.
//
// Nome que não está no catálogo fica como está: pode ser nome físico escrito à mão —
// a outra forma válida de endereçar — e trocá-lo seria inventar.
func resolverNomesFisicos(arquivos []factorygo.Arquivo, projectRoot string) []factorygo.Arquivo {
	if projectRoot == "" || len(arquivos) == 0 {
		return arquivos
	}
	tabelas := migrationgo.PhysicalTableByIdentifier(projectRoot)
	colunas := migrationgo.PhysicalColumnByIdentifier(projectRoot)
	if len(tabelas) == 0 && len(colunas) == 0 {
		return arquivos
	}

	for indice := range arquivos {
		identificadorDaTabela := arquivos[indice].Tabela
		if fisico, tem := tabelas[identificadorDaTabela]; tem {
			arquivos[indice].Tabela = fisico
		}
		// As colunas são resolvidas pelo grupo da TABELA, e o grupo é indexado pelo
		// identificador — por isso ele é lido antes de a tabela ser reescrita.
		doGrupo := colunas[identificadorDaTabela]
		if len(doGrupo) == 0 {
			continue
		}
		for campo := range arquivos[indice].Campos {
			if fisico, tem := doGrupo[arquivos[indice].Campos[campo].Coluna]; tem {
				arquivos[indice].Campos[campo].Coluna = fisico
			}
		}
	}
	return arquivos
}

// raizDoProjetoDaFactory acha a raiz a partir da pasta de factories.
func raizDoProjetoDaFactory(pasta string) string {
	if raiz := projectRoot(pasta); raiz != "" {
		return raiz
	}
	return strings.TrimSpace("")
}
