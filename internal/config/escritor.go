package config

// Coletor de falhas de escrita do scaffold.
//
// Existe porque o scaffold tinha 17 escritas com o erro DESCARTADO — `_ = os.WriteFile`
// — e o chamador descartava também o erro que a função devolvia. O efeito é o pior
// possível numa criação de projeto: disco cheio, permissão negada ou caminho longo
// demais, e o projeto nasce sem a migration de exemplo, sem o docker-compose ou sem o
// .gitignore, sem uma palavra. A pessoa descobre muito depois, e não relaciona com a
// criação.
//
// A escolha aqui é acumular em vez de abortar no primeiro. Numa criação de projeto,
// saber que faltaram três arquivos de uma vez é melhor que descobrir um por execução —
// e quase toda falha de escrita tem a mesma causa raiz, que aparece melhor no conjunto.

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// escritor acumula as falhas em vez de descartá-las.
type escritor struct {
	falhas map[string]string
}

func novoEscritor() *escritor {
	return &escritor{falhas: map[string]string{}}
}

// arquivo grava e registra a falha, se houver.
func (e *escritor) arquivo(caminho string, conteudo []byte) {
	if err := os.WriteFile(caminho, conteudo, 0o644); err != nil {
		e.falhas[caminho] = err.Error()
	}
}

// pasta cria a árvore e registra a falha, se houver.
func (e *escritor) pasta(caminho string) {
	if err := os.MkdirAll(caminho, 0o755); err != nil {
		e.falhas[caminho] = err.Error()
	}
}

// registrar guarda a falha de um passo que não é escrita de arquivo.
func (e *escritor) registrar(oQue string, err error) {
	if err != nil {
		e.falhas[oQue] = err.Error()
	}
}

// Falhas devolve o que não pôde ser escrito, em ordem estável.
func (e *escritor) Falhas() []string {
	if e == nil || len(e.falhas) == 0 {
		return nil
	}
	caminhos := make([]string, 0, len(e.falhas))
	for caminho := range e.falhas {
		caminhos = append(caminhos, caminho)
	}
	sort.Strings(caminhos)

	lista := make([]string, 0, len(caminhos))
	for _, caminho := range caminhos {
		lista = append(lista, fmt.Sprintf("%s: %s", caminho, e.falhas[caminho]))
	}
	return lista
}

// Erro resume as falhas num erro só, ou nil quando tudo foi escrito.
func (e *escritor) Erro() error {
	falhas := e.Falhas()
	if len(falhas) == 0 {
		return nil
	}
	return fmt.Errorf("%d item(ns) do scaffold não pôde(ram) ser criado(s):\n  - %s",
		len(falhas), strings.Join(falhas, "\n  - "))
}
