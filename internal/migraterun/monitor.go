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

	"github.com/PhelipeViana/gokit/internal/aviso"
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

// nivelDaOcorrencia traduz o tipo de ocorrência em GRAVIDADE, que é o eixo do filtro
// de notificação.
//
// A separação importa: o tipo diz O QUE aconteceu e serve ao relatório; o nível diz
// QUÃO SÉRIO é e serve a quem assina o canal. Ocorrência que o motor já resolveu
// sozinho é info — notificá-la como problema treinaria a equipe a ignorar o canal.
func nivelDaOcorrencia(tipo string) aviso.Nivel {
	switch tipo {
	case OcorrenciaAliasRenomeado:
		// O motor desempatou o apelido e o nome físico ficou intacto: nada a fazer.
		return aviso.NivelInfo
	case OcorrenciaTipoSemEquivalente, OcorrenciaNaoLido, OcorrenciaSQLNaoPortavel:
		// Degradou de forma registrada. Nada falhou, mas há lacuna a fechar.
		return aviso.NivelWarning
	case OcorrenciaIdentidadeForaDaChave:
		// Resolvido pelo executor, que cria a UNIQUE KEY no MySQL. É só ciência.
		return aviso.NivelInfo
	}
	// Tipo que este tradutor não conhece é justamente o que ninguém mapeou.
	return aviso.NivelWarning
}

// Notificar manda o resumo do monitor para o canal, agrupado por tipo.
//
// Agrupado, e não uma mensagem por ocorrência: um schema legado gera centenas, e 792
// posts no canal seria o mesmo que nenhum. O arquivo tem o detalhe; o canal tem o
// resumo e o caminho para o arquivo.
func (m *Monitor) Notificar(central aviso.Central, caminhoDoArquivo string) {
	if m.Vazio() || !central.Ativa() {
		return
	}

	// Uma mensagem por NÍVEL: quem assina só warning não deve receber o que é info.
	porNivel := map[aviso.Nivel][]string{}
	for tipo, quantidade := range m.PorTipo {
		nivel := nivelDaOcorrencia(tipo)
		porNivel[nivel] = append(porNivel[nivel], fmt.Sprintf("%s: %d", tipo, quantidade))
	}

	for nivel, linhas := range porNivel {
		sort.Strings(linhas)
		total := 0
		for tipo, quantidade := range m.PorTipo {
			if nivelDaOcorrencia(tipo) == nivel {
				total += quantidade
			}
		}
		central.Notificar(aviso.Aviso{
			Nivel: nivel,
			Acao:  "leitura do banco com ocorrências",
			Corpo: fmt.Sprintf("A leitura do banco registrou %d ocorrência(s) neste nível. O detalhe de cada "+
				"uma está em %s, com o que o banco disse cru e o que o gokit decidiu.", total, caminhoDoArquivo),
			Contexto: map[string]string{"comando": m.Comando, "registro": caminhoDoArquivo},
			Bruto:    strings.Join(linhas, "\n"),
		})
	}
}

// registrarDefaultNaoPortavel anota a coluna cujo DEFAULT é expressão que o tradutor
// do executor não reconhece.
//
// O arquivo sai FIEL à origem — a expressão é o que o banco de fato usa —, e por isso
// mesmo ela é uma bomba armada: aplicar esse corpus em outro dialeto produz o erro do
// banco, não uma mensagem do gokit. Foi o caso do `user_name()` do SQL Server, que no
// Oracle virou ORA-04044 no meio da migração, sem citar DEFAULT nem a coluna.
//
// Registrada, ela aparece na lista de trabalho ANTES do run.
func registrarDefaultNaoPortavel(m *Monitor, tabela string, coluna ColunaDoBanco) {
	if coluna.Default == "" || !EhExpressao(coluna.Default) {
		return
	}
	if defaultPortavel(coluna.Default) {
		return
	}
	m.Registrar(Ocorrencia{
		Tipo:    OcorrenciaSQLNaoPortavel,
		Tabela:  strings.ToLower(tabela),
		Coluna:  strings.ToLower(coluna.Nome),
		Origem:  coluna.Default,
		Decisao: "DEFAULT mantido fiel à origem; o tradutor do executor não conhece esta expressão, então ela vai crua para os outros dialetos",
	})
}

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
