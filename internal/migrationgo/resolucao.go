package migrationgo

// Resolução de referência de catálogo de volta ao NOME FÍSICO.
//
// O corpus pode endereçar tabela e coluna de duas formas:
//
//	migrate.CreateTable("wkf_ouvidoria_assunto")        // nome físico, literal
//	Table: core.Table.WkfOuvidoriaAssunto               // referência de catálogo
//
// Quem lê o arquivo por AST recebe, na segunda forma, o IDENTIFICADOR Go — e tratá-lo
// como se fosse o nome físico é um erro que passa despercebido em nome curto e quebra
// em nome composto: `strings.ToUpper("WkfOuvidoriaAssunto")` dá
// `WKFOUVIDORIAASSUNTO`, que não é `WKF_OUVIDORIA_ASSUNTO`. Os underscores se perdem.
//
// O efeito medido num schema legado: a preservação do `factory create` falhava em toda
// tabela com underscore — quase todas —, então o comando reescrevia o arquivo e apagava
// as expressões ajustadas à mão; e o `factory validate` acusava "a tabela X não é
// criada por nenhuma migration" para tabela que existia. Não aparecia no projeto de
// teste porque lá os nomes são `users`, `cidades`, `paises`: sem underscore, achatar
// não muda nada.
//
// A resolução mora aqui, e não em cada consumidor, porque cada consumidor que resolver
// por conta própria é um lugar novo para o mesmo defeito nascer.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// grupoDeColunas casa a abertura de um grupo do literal de Column:
//
//	Acoesauditorium: struct {
var grupoDeColunas = regexp.MustCompile(`(?m)^\t([A-Za-z_][A-Za-z0-9_]*):\s*struct\s*\{`)

// entradaDeColuna casa a atribuição de uma coluna dentro do grupo:
//
//	Acoesauditoriumid: "acoesauditoriumid",
var entradaDeColuna = regexp.MustCompile(`(?m)^\t\t([A-Za-z_][A-Za-z0-9_]*):\s*"([^"]+)",`)

// PhysicalTableByIdentifier devolve identificador Go → nome FÍSICO da tabela.
//
// Note a diferença em relação ao valor do catálogo: `core.Table.X` guarda o APELIDO, e
// o nome físico só aparece no comentário `// physical:` quando os dois divergem. Sem
// comentário, apelido e físico coincidem.
func PhysicalTableByIdentifier(projectRoot string) map[string]string {
	porIdentificador := map[string]string{}
	caminhos := append(caminhosLegadosDoCatalogo(projectRoot), caminhoDoCatalogo(projectRoot))
	for _, caminho := range caminhos {
		dados, err := os.ReadFile(caminho)
		if err != nil {
			continue
		}
		for _, casado := range tableEntryComFisico.FindAllStringSubmatch(string(dados), -1) {
			identificador, apelido, fisico := casado[1], casado[2], casado[3]
			if fisico == "" {
				fisico = apelido
			}
			porIdentificador[identificador] = fisico
		}
	}
	return porIdentificador
}

// PhysicalColumnByIdentifier devolve, por identificador de TABELA, o mapa
// identificador de coluna → nome físico da coluna.
//
// A leitura é do LITERAL do catálogo (a parte depois do `}{`), não da declaração da
// struct: é só ali que o nome físico está escrito.
func PhysicalColumnByIdentifier(projectRoot string) map[string]map[string]string {
	dados, err := os.ReadFile(caminhoDoCatalogoDeColunas(projectRoot))
	if err != nil {
		return map[string]map[string]string{}
	}
	texto := string(dados)

	// O arquivo declara a struct e depois o literal, com os MESMOS grupos. Só o
	// segundo bloco tem as atribuições, então basta cortar no `}{` que separa os dois.
	if corte := strings.Index(texto, "}{"); corte > 0 {
		texto = texto[corte:]
	}

	porTabela := map[string]map[string]string{}
	grupos := grupoDeColunas.FindAllStringSubmatchIndex(texto, -1)
	for posicao, grupo := range grupos {
		identificadorDaTabela := texto[grupo[2]:grupo[3]]
		fim := len(texto)
		if posicao+1 < len(grupos) {
			fim = grupos[posicao+1][0]
		}
		colunas := map[string]string{}
		for _, casado := range entradaDeColuna.FindAllStringSubmatch(texto[grupo[1]:fim], -1) {
			colunas[casado[1]] = casado[2]
		}
		if len(colunas) > 0 {
			porTabela[identificadorDaTabela] = colunas
		}
	}
	return porTabela
}

// ProjectRootFrom expõe a busca da raiz do projeto a partir de um caminho qualquer.
// Quem lê um arquivo do corpus precisa dela para achar o catálogo.
func ProjectRootFrom(caminho string) string {
	if info, err := os.Stat(caminho); err == nil && !info.IsDir() {
		caminho = filepath.Dir(caminho)
	}
	return projectRoot(caminho)
}
