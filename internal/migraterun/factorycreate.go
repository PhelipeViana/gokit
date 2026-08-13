package migraterun

// Geração de factories a partir do corpus de migrations.
//
// A escolha do dado fake sai do que a migration declara: tipo, tamanho,
// precisão, chave primária, chave estrangeira e CHECK. Antes isso era inferido
// por expressão regular em cima do DDL de um dialeto só, o que fazia a factory
// gerada do Oracle não servir no Postgres. Aqui a fonte é o AST, que é a mesma
// para os quatro bancos.

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/factorygo"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
	migrate "github.com/PhelipeViana/gokit/migration"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// FactoryCreate gera ou atualiza a factory de uma tabela. Sem tabela, percorre
// todas as que ainda não têm factory.
func FactoryCreate(root string, state config.ConfigState, table string) error {
	// O import do pacote core, para a factory endereçar tabela e coluna por
	// referência em vez de texto. Sem módulo resolvido, cai no texto — o gerador
	// nunca deve falhar por causa disso.
	importCore := ""
	if pRoot := projectRoot(root); pRoot != "" {
		if modulo, err := GetModuleName(pRoot); err == nil {
			importCore = modulo + "/internal/gokit/core"
		}
	}
	formas, err := tableShapes(root, state)
	if err != nil {
		return err
	}
	if len(formas) == 0 {
		return cliui.NewUserError(
			i18n.T("fac_no_tables"),
			i18n.T("fac_no_tables_fix"),
		)
	}

	checks, err := tableCheckValues(root, state)
	if err != nil {
		return err
	}

	pasta := factoryRoot(root, state)
	if err := os.MkdirAll(pasta, 0o755); err != nil {
		return err
	}

	// O que já está escrito na pasta, venha de onde vier: um arquivo por tabela
	// (a organização antiga) ou o arquivo único. É daqui que sai a preservação
	// das expressões e do Ruler.
	escritas, antigos, err := factoriesEscritas(pasta)
	if err != nil {
		return err
	}

	// O arquivo passa a conter todas as tabelas do corpus. Pedir uma tabela
	// específica não descarta as outras — só garante que ela entre.
	var alvos []string
	if table != "" {
		chave := strings.ToLower(table)
		if _, existe := formas[chave]; !existe {
			return cliui.NewUserError(
				i18n.Tf("fac_table_unknown", table),
				i18n.T("fac_table_unknown_fix"),
			)
		}
		alvos = []string{chave}
		for nome := range formas {
			if _, tem := escritas[strings.ToUpper(formas[nome].Table)]; tem && nome != chave {
				alvos = append(alvos, nome)
			}
		}
	} else {
		for nome := range formas {
			alvos = append(alvos, nome)
		}
	}
	sort.Strings(alvos)

	caminho := filepath.Join(pasta, arquivoDeFactories)
	atual, err := os.ReadFile(caminho)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	novoArquivo := os.IsNotExist(err)

	blocos := make([]string, 0, len(alvos))
	for _, nome := range alvos {
		forma := formas[nome]
		anterior, jaExistia := escritas[strings.ToUpper(forma.Table)]

		// Numa regeneração, a expressão de cada coluna que já existe é mantida
		// como está: o gerador só acerta a lista de colunas contra a migration.
		// Sem isso um ajuste manual se perderia a cada `create`.
		var existentes map[string]string
		var ruler *migrate.Ruler
		if jaExistia {
			existentes = map[string]string{}
			for _, campo := range anterior.Campos {
				// Duas chaves porque o arquivo pode endereçar a coluna de duas
				// formas: o nome físico (`"aprovador_id"`) ou a referência de
				// catálogo (`core.Column.Pedidos.AprovadorId`). Guardar só a
				// primeira fazia a regeneração perder todo ajuste manual de um
				// arquivo escrito na forma nova.
				existentes[strings.ToUpper(campo.Coluna)] = campo.Origem
				existentes[chaveDeColuna(campo.Coluna)] = campo.Origem
			}
			copia := anterior.Ruler
			ruler = &copia
		}

		bloco := blocoDaFactory(forma, checks[strings.ToUpper(forma.Table)], state, existentes, ruler, importCore)
		blocos = append(blocos, bloco)
	}

	conteudo := montaArquivoDeFactories(blocos, importCore)

	// O estado de cada factory é decidido comparando bloco formatado com bloco
	// formatado, extraídos do arquivo antigo e do novo. Comparar o texto cru que
	// o gerador monta não funciona: o gofmt do arquivo inteiro realinha o mapa de
	// Fields, e toda factory apareceria como atualizada a cada execução.
	criadas, atualizadas, preservadas := 0, 0, 0
	for _, nome := range alvos {
		forma := formas[nome]
		funcao := nomeDaFuncao(forma.Table)
		anterior, jaExistia := escritas[strings.ToUpper(forma.Table)]

		if !jaExistia {
			criadas++
			fmt.Printf("  %s %s\n", cliui.Success("+"), funcao)
			continue
		}
		origem, err := os.ReadFile(anterior.Caminho)
		if err == nil && blocoDaFuncao(string(origem), funcao) == blocoDaFuncao(conteudo, funcao) {
			preservadas++
			continue
		}
		atualizadas++
		fmt.Printf("  %s %s\n", cliui.Warning("~"), funcao)
	}

	if string(atual) != conteudo {
		if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
			return err
		}
		if novoArquivo {
			if err := recordGeneratedFile(root, "factories", "factory", caminho); err != nil {
				return i18n.Errf("fac_register_failed", err)
			}
		}
	}

	// Os arquivos por tabela viraram função dentro do arquivo único. Mantê-los
	// seria função duplicada no mesmo pacote: o projeto pararia de compilar.
	for _, obsoleto := range antigos {
		if filepath.Base(obsoleto) == arquivoDeFactories {
			continue
		}
		if err := os.Remove(obsoleto); err != nil {
			return err
		}
		fmt.Printf("  %s %s\n", cliui.Muted("-"), filepath.Base(obsoleto))
	}

	fmt.Printf(i18n.T("fac_create_summary"),
		cliui.Success("✓ OK"), criadas, atualizadas, preservadas)
	return nil
}

// arquivoDeFactories é o arquivo único da pasta. Não há regra implícita que
// justifique fatiar por tabela, e um arquivo só deixa o conjunto legível de uma
// vez. Continua sendo editável à mão: o Active é por FUNÇÃO, não por arquivo.
const arquivoDeFactories = "factories.go"

// factoriesEscritas devolve o que já está na pasta, indexado pelo nome físico da
// tabela, mais os arquivos de onde isso veio.
func factoriesEscritas(pasta string) (map[string]factorygo.Arquivo, []string, error) {
	entradas, err := os.ReadDir(pasta)
	if os.IsNotExist(err) {
		return map[string]factorygo.Arquivo{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	escritas := map[string]factorygo.Arquivo{}
	var arquivos []string
	for _, entrada := range entradas {
		if entrada.IsDir() || !strings.HasSuffix(entrada.Name(), ".go") || strings.HasSuffix(entrada.Name(), "_test.go") {
			continue
		}
		caminho := filepath.Join(pasta, entrada.Name())
		// Arquivo que não parseia não pode sumir em silêncio: ele é preservado e
		// o autor vê o erro no `factory validate`.
		lidas, err := factorygo.ParseArquivos(caminho)
		if err != nil || len(lidas) == 0 {
			continue
		}
		arquivos = append(arquivos, caminho)
		for _, lida := range lidas {
			escritas[strings.ToUpper(lida.Tabela)] = lida
		}
	}
	return escritas, arquivos, nil
}

// blocoDaFuncao recorta o texto de uma função do arquivo. Fecha na primeira `}`
// em coluna zero, o que basta porque o arquivo é sempre gofmt.
func blocoDaFuncao(conteudo, funcao string) string {
	abertura := "func " + funcao + "() migrate.Factory {"
	inicio := strings.Index(conteudo, abertura)
	if inicio < 0 {
		return ""
	}
	resto := conteudo[inicio:]
	if fim := strings.Index(resto, "\n}\n"); fim >= 0 {
		return resto[:fim+3]
	}
	return resto
}

// tableCheckValues extrai os domínios declarados em AddCheck, no formato
// `COLUNA IN ('A','B')`. É o que impede a factory de gerar um valor que a
// própria migration proíbe.
func tableCheckValues(root string, state config.ConfigState) (map[string]map[string][]string, error) {
	files, err := loadPlans(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if err != nil {
		return nil, describeLoadError(err)
	}

	resultado := map[string]map[string][]string{}
	for _, file := range files {
		for _, operation := range file.Plan.Operations {
			if operation.Kind != string(acao.AddCheck) || operation.SQL == "" {
				continue
			}
			coluna, valores := valoresDoCheck(operation.SQL)
			if coluna == "" || len(valores) == 0 {
				continue
			}
			tabela := strings.ToUpper(operation.Table)
			if resultado[tabela] == nil {
				resultado[tabela] = map[string][]string{}
			}
			resultado[tabela][strings.ToUpper(coluna)] = valores
		}
	}
	return resultado, nil
}

// valoresDoCheck lê `COLUNA IN ('A', 'B', 'C')`. Formas mais complexas são
// ignoradas de propósito: um CHECK que o gerador não entende vira um dado
// genérico, e o erro de INSERT diz o que corrigir.
func valoresDoCheck(expressao string) (string, []string) {
	alto := strings.ToUpper(expressao)
	posicao := strings.Index(alto, " IN ")
	if posicao < 0 {
		return "", nil
	}
	coluna := strings.Trim(strings.TrimSpace(expressao[:posicao]), `"'`+"`[]()")
	if coluna == "" || strings.ContainsAny(coluna, " ") {
		return "", nil
	}

	resto := expressao[posicao+4:]
	abre := strings.Index(resto, "(")
	fecha := strings.LastIndex(resto, ")")
	if abre < 0 || fecha <= abre {
		return "", nil
	}

	var valores []string
	for _, item := range strings.Split(resto[abre+1:fecha], ",") {
		valor := strings.Trim(strings.TrimSpace(item), `'"`)
		if valor != "" {
			valores = append(valores, valor)
		}
	}
	return coluna, valores
}

// chaveDeColuna reduz as duas formas de escrever a mesma coluna a uma chave só:
// `aprovador_id` e `AprovadorId` viram ambos APROVADORID.
func chaveDeColuna(nome string) string {
	return strings.ToUpper(migrationgo.ExportedIdentifier(nome))
}

// renderizaFactory monta um arquivo com uma factory só. É o que os testes usam e
// o caminho de quem quiser gerar avulso; a geração normal escreve todas juntas.
func renderizaFactory(forma acao.Operacao, checks map[string][]string, state config.ConfigState, existentes map[string]string, ruler *migrate.Ruler, importCore string) string {
	bloco := blocoDaFactory(forma, checks, state, existentes, ruler, importCore)
	return montaArquivoDeFactories([]string{bloco}, importCore)
}

// montaArquivoDeFactories junta os blocos num arquivo do pacote factories.
func montaArquivoDeFactories(blocos []string, importCore string) string {
	var texto strings.Builder
	texto.WriteString("package factories\n\n")
	if importCore != "" {
		fmt.Fprintf(&texto, "import (\n\tcore %q\n\tmigrate \"github.com/PhelipeViana/gokit/migration\"\n)\n\n", importCore)
	} else {
		texto.WriteString("import migrate \"github.com/PhelipeViana/gokit/migration\"\n\n")
	}
	texto.WriteString(strings.Join(blocos, "\n"))

	// O alinhamento das colunas segue uma heurística própria do gofmt, que
	// quebra o bloco quando uma linha destoa muito das outras. Formatar com a
	// biblioteca padrão evita que o arquivo gerado apareça sujo no `gofmt -l`.
	formatado, err := format.Source([]byte(texto.String()))
	if err != nil {
		return texto.String()
	}
	return string(formatado)
}

// blocoDaFactory escreve a função de uma tabela. existentes traz as expressões já
// escritas para ela — elas vencem a heurística; ruler preserva o Count/Active que
// o autor tenha ajustado.
func blocoDaFactory(forma acao.Operacao, checks map[string][]string, state config.ConfigState, existentes map[string]string, ruler *migrate.Ruler, importCore string) string {
	tabela := strings.ToUpper(forma.Table)
	regra := migrate.Ruler{Count: 10, Active: true}
	if ruler != nil {
		regra = *ruler
	}

	campos := make([][2]string, 0, len(forma.Columns))
	for _, column := range forma.Columns {
		// Coluna de identidade é preenchida pelo banco; escrever nela obrigaria
		// a ligar IDENTITY_INSERT sem necessidade.
		if column.AutoIncrement {
			continue
		}
		expressao, mantida := existentes[strings.ToUpper(column.Name)]
		if !mantida {
			expressao, mantida = existentes[chaveDeColuna(column.Name)]
		}
		if !mantida {
			expressao = expressaoParaColuna(tabela, column, checks[strings.ToUpper(column.Name)], state, importCore != "")
		}
		campos = append(campos, [2]string{column.Name, expressao})
	}

	largura := 0
	for _, campo := range campos {
		if tamanho := len(campo[0]) + 3; tamanho > largura {
			largura = tamanho
		}
	}

	// Endereçamento por REFERÊNCIA de catálogo quando o módulo é conhecido: aí o
	// compilador passa a checar nome de tabela e de coluna. Sem módulo resolvido —
	// projeto sem go.mod legível —, cai no texto, que continua válido.
	entidade := migrationgo.ExportedIdentifier(forma.Table)
	refTabela := fmt.Sprintf("%q", tabela)
	if importCore != "" {
		refTabela = "core.Table." + entidade
	}

	var texto strings.Builder
	fmt.Fprintf(&texto, i18n.T("fac_gen_doc"), nomeDaFuncao(forma.Table), tabela)
	fmt.Fprintf(&texto, "func %s() migrate.Factory {\n", nomeDaFuncao(forma.Table))
	texto.WriteString("\treturn migrate.Factory{\n")
	fmt.Fprintf(&texto, "\t\tTable: %s,\n", refTabela)
	fmt.Fprintf(&texto, "\t\tRuler: migrate.Ruler{Count: %d, Active: %t},\n", regra.Count, regra.Active)
	texto.WriteString("\t\tData: migrate.Fields{\n")
	for _, campo := range campos {
		chave := fmt.Sprintf("%q:", campo[0])
		if importCore != "" {
			chave = "core.Column." + entidade + "." + migrationgo.ExportedIdentifier(campo[0]) + ":"
		}
		fmt.Fprintf(&texto, "\t\t\t%-*s %s,\n", largura, chave, campo[1])
	}
	texto.WriteString("\t\t},\n\t}\n}\n")
	return texto.String()
}

func nomeDaFuncao(tabela string) string {
	var nome strings.Builder
	for _, parte := range strings.Split(strings.ToLower(tabela), "_") {
		if parte == "" {
			continue
		}
		nome.WriteString(strings.ToUpper(parte[:1]) + parte[1:])
	}
	nome.WriteString("Factory")
	return nome.String()
}

// expressaoParaColuna escolhe o gerador de dado da coluna.
//
// A ordem importa: o mais específico decide primeiro. Um override do projeto
// vence tudo; depois vem o domínio fechado do CHECK; depois a chave, que exige
// unicidade; e só então as heurísticas por nome e tipo.
func expressaoParaColuna(tabela string, column acao.ColunaDefinicao, check []string, state config.ConfigState, usaCore bool) string {
	nome := strings.ToUpper(column.Name)

	if expressao, existe := overrideDoProjeto(tabela, nome, state); existe {
		return expressao
	}
	if column.ReferenceTable != "" {
		if usaCore {
			// A referência aponta para a coluna do PAI, então o agrupador é o da
			// tabela referenciada, não o da tabela atual.
			// A coluna do pai sai da FK declarada, então não precisa ser escrita.
			return fmt.Sprintf("migrate.Reference(core.Table.%s)",
				migrationgo.ExportedIdentifier(column.ReferenceTable))
		}
		return fmt.Sprintf("migrate.Reference(%q, %q)", strings.ToUpper(column.ReferenceTable), strings.ToUpper(column.ReferenceColumn))
	}
	if len(check) > 0 {
		return expressaoDeDominio(check)
	}

	tamanho := column.Length
	precisao := column.Precision
	if precisao == 0 {
		precisao = 10
	}

	// Chave primária precisa ser única por linha, então nada de valor fixo.
	if column.PrimaryKey || column.Unique {
		switch column.Type {
		case "string", "char", "text":
			return fmt.Sprintf("migrate.FakeCode(%d)", ouEntao(tamanho, 30))
		case "int", "integer", "decimal":
			return fmt.Sprintf("migrate.FakeInt(1, %d)", tetoNumerico(precisao))
		}
	}

	switch column.Type {
	case "date":
		return "migrate.FakeDate()"
	case "datetime", "timestamp":
		return "migrate.FakeDateTime()"
	case "boolean":
		// 0/1 alternando pelo índice, não "S"/"N": a coerção de valor recusa "S" nos
		// quatro dialetos ("não é um booleano válido"), então a factory gerada para
		// coluna boolean simplesmente não rodava. O S/N abaixo continua valendo para
		// CHAR(1), que em schema legado é flag de texto — ali é o tipo que difere.
		return "migrate.FakeInt(0, 1)"
	case "binary":
		return "migrate.FakeBytes(128)"
	}

	documento := tamanho
	if documento == 0 && ehNumerica(column.Type) {
		documento = precisao
	}
	if strings.Contains(nome, "CNPJ") {
		return fmt.Sprintf("migrate.FakeUniqueCNPJ(%d)", documento)
	}
	if strings.Contains(nome, "CPF") {
		return fmt.Sprintf("migrate.FakeUniqueCPF(%d)", documento)
	}

	if ehNumerica(column.Type) {
		if column.Scale > 0 {
			return fmt.Sprintf("migrate.FakeDecimal(%d, %d)", precisao, column.Scale)
		}
		if ehCategorica(nome) || precisao == 1 {
			return "migrate.FakeInt(1, 2)"
		}
		return fmt.Sprintf("migrate.FakeInt(1, %d)", tetoNumerico(precisao))
	}

	// Coluna de um caractere quase sempre é flag S/N, menos sexo.
	if tamanho == 1 && !strings.Contains(nome, "SEXO") {
		return "migrate.FakeChoice(\"S\", \"N\")"
	}

	switch {
	case strings.Contains(nome, "ESTADOCIVIL"), strings.Contains(nome, "ESTADO_CIVIL"):
		return "migrate.FakeChoice(\"1\", \"2\", \"3\", \"4\", \"5\")"
	case strings.Contains(nome, "SEXO"):
		return "migrate.FakeChoice(\"M\", \"F\")"
	case strings.Contains(nome, "EMAIL"):
		return conceito("FakeEmail", tamanho)
	case strings.Contains(nome, "CEP"):
		return conceito("FakeCEP", tamanho)
	case nome == "UF", strings.HasSuffix(nome, "_UF"), strings.Contains(nome, "RG_UF"), strings.Contains(nome, "CTPS_UF"):
		return "migrate.FakeUF()"
	case strings.Contains(nome, "FONE"), strings.Contains(nome, "TELEFONE"),
		strings.Contains(nome, "CELULAR"), strings.Contains(nome, "CONTATO"):
		return conceito("FakePhone", tamanho)
	case ehIP(nome):
		return "migrate.FakeIPv4()"
	case strings.Contains(nome, "USERAGENT"):
		return "migrate.FakeUserAgent()"
	case strings.Contains(nome, "REQUESTID"), strings.Contains(nome, "UUID"):
		return "migrate.FakeUUID()"
	case strings.Contains(nome, "METHOD"):
		return "migrate.FakeChoice(\"GET\", \"POST\", \"PUT\", \"PATCH\", \"DELETE\")"
	case strings.Contains(nome, "STATUSCODE"):
		return "migrate.FakeInt(200, 599)"
	case strings.Contains(nome, "HASH"), strings.Contains(nome, "SENHA"):
		return conceito("FakeHash", tamanho)
	case strings.Contains(nome, "LOGIN"), strings.Contains(nome, "USUARIO"):
		return conceito("FakeUsername", tamanho)
	case strings.Contains(nome, "NOMEARQ"), strings.Contains(nome, "ARQPDF"), strings.Contains(nome, "FOTO"):
		return conceito("FakeFileName", tamanho)
	case ehCodigoDeCidade(nome):
		return conceito("FakeCityCode", tamanho)
	case ehCidade(nome):
		return conceito("FakeCity", tamanho)
	case ehEstado(nome):
		if tamanho > 2 {
			return conceito("FakeState", tamanho)
		}
		return "migrate.FakeUF()"
	case strings.Contains(nome, "PAIS"):
		return conceito("FakeCountry", tamanho)
	case strings.Contains(nome, "NOME"), strings.Contains(nome, "RAZAO_SOCIAL"):
		return conceito("FakeName", tamanho)
	case strings.Contains(nome, "BAIRRO"):
		return conceito("FakeDistrict", tamanho)
	case strings.Contains(nome, "RUA"), strings.Contains(nome, "LOGRADOURO"):
		return conceito("FakeStreet", tamanho)
	case strings.Contains(nome, "MATRICULA"):
		return "migrate.FakeMatricula()"
	case ehCodigo(nome):
		return fmt.Sprintf("migrate.FakeCode(%d)", ouEntao(tamanho, 30))
	}

	padrao := 255
	if column.Type == "text" {
		padrao = 500
	}
	return fmt.Sprintf("migrate.FakeUniqueText(%q, %d)", tituloDaColuna(nome), ouEntao(tamanho, padrao))
}

// overrideDoProjeto consulta factory.expressions.mappers do gokit.json. Regra
// específica de um projeto mora na configuração dele, não no plugin.
func overrideDoProjeto(tabela, coluna string, state config.ConfigState) (string, bool) {
	if state.Config == nil {
		return "", false
	}
	mappers := state.Config.Factory.Expressions.Mappers
	if len(mappers) == 0 {
		return "", false
	}
	for chave, expressao := range mappers {
		alvoTabela, alvoColuna, especifico := strings.Cut(chave, ".")
		if especifico {
			if strings.EqualFold(alvoTabela, tabela) && strings.EqualFold(alvoColuna, coluna) {
				return expressao, true
			}
			continue
		}
		if strings.EqualFold(chave, coluna) {
			return expressao, true
		}
	}
	return "", false
}

// expressaoDeDominio traduz o domínio de um CHECK. Faixa numérica contígua
// vira FakeInt; o resto vira escolha circular, que percorre todos os valores.
func expressaoDeDominio(valores []string) string {
	numeros := make([]int, 0, len(valores))
	for _, valor := range valores {
		numero, err := strconv.Atoi(valor)
		if err != nil {
			numeros = nil
			break
		}
		numeros = append(numeros, numero)
	}

	if len(numeros) > 0 {
		menor, maior := numeros[0], numeros[0]
		for _, numero := range numeros[1:] {
			if numero < menor {
				menor = numero
			}
			if numero > maior {
				maior = numero
			}
		}
		if maior-menor+1 == len(numeros) {
			return fmt.Sprintf("migrate.FakeInt(%d, %d)", menor, maior)
		}
	}

	citados := make([]string, len(valores))
	for posicao, valor := range valores {
		citados[posicao] = strconv.Quote(valor)
	}
	return fmt.Sprintf("migrate.FakeChoice(%s)", strings.Join(citados, ", "))
}

// conceito escreve a chamada de um conceito de texto. O tamanho é sempre escrito,
// porque não há mais variante sem ele: 0 significa "sem limite". E o índice não
// aparece — o motor o injeta.
func conceito(nome string, tamanho int) string {
	return fmt.Sprintf("migrate.%s(%d)", nome, tamanho)
}

func ouEntao(valor, padrao int) int {
	if valor > 0 {
		return valor
	}
	return padrao
}

// tetoNumerico devolve o maior valor que cabe na precisão declarada.
func tetoNumerico(precisao int) int {
	if precisao <= 0 {
		return 9999
	}
	if precisao > 9 {
		precisao = 9
	}
	teto := 1
	for range precisao {
		teto *= 10
	}
	return teto - 1
}

func ehNumerica(tipo string) bool {
	switch tipo {
	case "int", "integer", "decimal":
		return true
	}
	return false
}

// ehCategorica marca as colunas que guardam um código de classificação curto,
// onde um valor alto não faria sentido.
func ehCategorica(nome string) bool {
	return strings.Contains(nome, "TIPO") ||
		strings.Contains(nome, "ESPECIE") ||
		strings.Contains(nome, "NATUREZA") ||
		strings.Contains(nome, "ESCOLARIDADE") ||
		strings.Contains(nome, "REGRA")
}

func ehIP(nome string) bool {
	return nome == "IP" || strings.HasSuffix(nome, "_IP") ||
		strings.Contains(nome, "IPADDRESS") || strings.Contains(nome, "IP_ADDRESS") ||
		strings.Contains(nome, "ACESSO_IP")
}

func ehCidade(nome string) bool {
	return strings.HasSuffix(nome, "_CIDADE") || strings.Contains(nome, "_CIDADE_") ||
		strings.Contains(nome, "CIDADE_ID") || strings.Contains(nome, "MUNICIPIO")
}

func ehCodigoDeCidade(nome string) bool {
	temCodigo := strings.Contains(nome, "CODG") || strings.Contains(nome, "CODIGO") || strings.Contains(nome, "_COD")
	return temCodigo && (strings.Contains(nome, "CIDADE") || strings.Contains(nome, "MUNICIPIO"))
}

func ehEstado(nome string) bool {
	if strings.Contains(nome, "ESTADOCIVIL") || strings.Contains(nome, "ESTADO_CIVIL") {
		return false
	}
	return strings.HasSuffix(nome, "ESTADO") || strings.Contains(nome, "_ESTADO") || strings.Contains(nome, "ESTADO_")
}

func ehCodigo(nome string) bool {
	return strings.Contains(nome, "CODIGO") || strings.Contains(nome, "CODG") ||
		strings.HasSuffix(nome, "_COD") || strings.Contains(nome, "NUMERO")
}

// tituloDaColuna transforma SIS_USER_INSERT em "Sis User Insert", que é o
// prefixo legível usado nos textos gerados.
func tituloDaColuna(nome string) string {
	partes := strings.Split(strings.ToLower(nome), "_")
	for posicao, parte := range partes {
		if parte != "" {
			partes[posicao] = strings.ToUpper(parte[:1]) + parte[1:]
		}
	}
	return strings.Join(partes, " ")
}
