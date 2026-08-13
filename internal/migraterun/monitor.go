package migraterun

// Monitor da leitura do banco: registro estruturado de tudo que o gokit NÃO
// conseguiu carregar com fidelidade.
//
// Por que existe um arquivo, e não só a mensagem no terminal: importar um schema
// legado degrada em vários pontos — tipo que o DSL não tem, CHECK que o catálogo
// não descreve, view cujo SQL só vale no banco de origem. Num banco de 755 tabelas
// esse aviso rola na tela e se perde. Gravado, ele é a lista de trabalho: dá para
// corrigir atipicidade uma a uma e conferir depois se sumiu.
//
// O formato é legível por máquina de propósito. É o mesmo registro que o playground
// vai consumir para responder "por que esta coluna virou texto?" e "o que muda
// deste banco para aquele?" — então o nome de cada campo é contrato, não detalhe de
// apresentação.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Tipos de ocorrência. São string, e não número, porque o arquivo é lido por
// humano e por outra ferramenta: acrescentar um tipo novo não pode invalidar o que
// já foi gravado.
const (
	// OcorrenciaTipoSemEquivalente: o tipo do banco não existe no DSL e a coluna
	// caiu no tipo mais permissivo que os quatro aceitam.
	OcorrenciaTipoSemEquivalente = "tipo_sem_equivalente"
	// OcorrenciaIdentidadeForaDaChave: legal em três dialetos, e no MySQL obriga o
	// executor a criar uma UNIQUE KEY que a origem não tem.
	OcorrenciaIdentidadeForaDaChave = "identidade_fora_da_chave"
	// OcorrenciaNaoLido: o objeto existe no banco e o import não o carrega.
	OcorrenciaNaoLido = "nao_lido"
	// OcorrenciaSQLNaoPortavel: o texto veio de um dialeto e não foi verificado
	// nos outros.
	OcorrenciaSQLNaoPortavel = "sql_nao_portavel"
	// OcorrenciaAliasRenomeado: dois nomes físicos gerariam o mesmo identificador
	// de catálogo, e o apelido de um deles foi desempatado.
	OcorrenciaAliasRenomeado = "alias_renomeado"
)

// Ocorrencia é uma perda ou desvio na leitura, com o suficiente para agir.
type Ocorrencia struct {
	Tipo   string `json:"tipo"`
	Tabela string `json:"tabela,omitempty"`
	Coluna string `json:"coluna,omitempty"`
	Objeto string `json:"objeto,omitempty"`
	// Origem é o que o banco disse, cru. É o que permite reproduzir o caso sem
	// acesso ao banco.
	Origem string `json:"origem,omitempty"`
	// Decisao é o que o gokit fez a respeito.
	Decisao string `json:"decisao,omitempty"`
}

// Monitor acumula as ocorrências de uma execução.
type Monitor struct {
	Versao      int            `json:"versao"`
	Comando     string         `json:"comando"`
	Dialeto     string         `json:"dialeto"`
	Conexao     string         `json:"conexao"`
	Em          string         `json:"em"`
	Total       int            `json:"total"`
	PorTipo     map[string]int `json:"por_tipo"`
	Ocorrencias []Ocorrencia   `json:"ocorrencias"`
}

// NovoMonitor abre um registro para o comando corrente.
func NovoMonitor(comando, conexao, dialeto string) *Monitor {
	return &Monitor{
		Versao:  1,
		Comando: comando,
		Dialeto: dialeto,
		Conexao: conexao,
		Em:      time.Now().Format(time.RFC3339),
		PorTipo: map[string]int{},
	}
}

// Registrar acrescenta uma ocorrência. Monitor nulo aceita a chamada e ignora, para
// que quem produz ocorrência não precise checar.
func (m *Monitor) Registrar(ocorrencia Ocorrencia) {
	if m == nil {
		return
	}
	m.Ocorrencias = append(m.Ocorrencias, ocorrencia)
	m.PorTipo[ocorrencia.Tipo]++
	m.Total++
}

// Vazio diz se não houve nenhuma perda — o caso em que não há o que gravar.
func (m *Monitor) Vazio() bool { return m == nil || m.Total == 0 }

// Resumo devolve as contagens por tipo, em ordem estável, para a mensagem final.
func (m *Monitor) Resumo() []string {
	if m == nil {
		return nil
	}
	tipos := make([]string, 0, len(m.PorTipo))
	for tipo := range m.PorTipo {
		tipos = append(tipos, tipo)
	}
	sort.Strings(tipos)
	linhas := make([]string, 0, len(tipos))
	for _, tipo := range tipos {
		linhas = append(linhas, fmt.Sprintf("%s: %d", tipo, m.PorTipo[tipo]))
	}
	return linhas
}

// Escrever grava o registro na pasta de documentação do projeto e devolve o caminho
// relativo.
//
// Vai em docs, e não na pasta de estado ignorada pelo Git: o que o import não
// conseguiu carregar é informação da equipe, não lixo de execução — alguém precisa
// revisar, e revisão que não é versionada não acontece.
func (m *Monitor) Escrever(root, pastaDeDocs string) (string, error) {
	if m.Vazio() {
		return "", nil
	}
	// Ordem estável: o arquivo é comparado entre execuções para ver se a lista
	// diminuiu, e ordenação instável faria todo diff parecer mudança.
	sort.SliceStable(m.Ocorrencias, func(i, j int) bool {
		a, b := m.Ocorrencias[i], m.Ocorrencias[j]
		if a.Tipo != b.Tipo {
			return a.Tipo < b.Tipo
		}
		if a.Tabela != b.Tabela {
			return a.Tabela < b.Tabela
		}
		if a.Objeto != b.Objeto {
			return a.Objeto < b.Objeto
		}
		return a.Coluna < b.Coluna
	})

	destino := pastaDeDocs
	if destino == "" {
		destino = filepath.Join("internal", "gokit", "docs")
	}
	pasta := destino
	if !filepath.IsAbs(pasta) {
		pasta = filepath.Join(root, filepath.FromSlash(destino))
	}
	if err := os.MkdirAll(pasta, 0o755); err != nil {
		return "", err
	}

	dados, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	dados = append(dados, '\n')

	caminho := filepath.Join(pasta, nomeDoMonitor)
	if err := os.WriteFile(caminho, dados, 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(destino, nomeDoMonitor)), nil
}

// nomeDoMonitor é fixo: reescrever o mesmo arquivo deixa o diff entre execuções
// mostrar o que foi resolvido. Um arquivo por execução esconderia isso.
const nomeDoMonitor = "monitor-import.json"

// registrarTipoDegradado anota a coluna cujo tipo o DSL não sabe representar.
func registrarTipoDegradado(m *Monitor, tabela string, coluna ColunaDoBanco, dialect, declarado string) {
	if familiaDoTipo(coluna.Tipo, dialect) != "" {
		return
	}
	m.Registrar(Ocorrencia{
		Tipo:    OcorrenciaTipoSemEquivalente,
		Tabela:  strings.ToLower(tabela),
		Coluna:  strings.ToLower(coluna.Nome),
		Origem:  coluna.Tipo,
		Decisao: declarado,
	})
}
