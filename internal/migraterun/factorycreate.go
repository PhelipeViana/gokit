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

	"gokit/internal/cliui"
	"gokit/internal/config"
	"gokit/internal/factorygo"
	migrate "gokit/migration"
	"gokit/migration/acao"
)

// FactoryCreate gera ou atualiza a factory de uma tabela. Sem tabela, percorre
// todas as que ainda não têm factory.
func FactoryCreate(root string, state config.ConfigState, table string) error {
	formas, err := tableShapes(root, state)
	if err != nil {
		return err
	}
	if len(formas) == 0 {
		return cliui.NewUserError(
			"Nenhuma tabela encontrada nas migrations.",
			"Rode `gokit migrate create` para declarar a tabela antes de gerar a factory.",
		)
	}

	checks, err := tableCheckValues(root, state)
	if err != nil {
		return err
	}

	var alvos []string
	if table != "" {
		chave := strings.ToLower(table)
		if _, existe := formas[chave]; !existe {
			return cliui.NewUserError(
				fmt.Sprintf("A tabela %s não é criada por nenhuma migration.", table),
				"Confira o nome ou declare a tabela primeiro com `gokit migrate create`.",
			)
		}
		alvos = []string{chave}
	} else {
		for nome := range formas {
			alvos = append(alvos, nome)
		}
		sort.Strings(alvos)
	}

	pasta := factoryRoot(root, state)
	if err := os.MkdirAll(pasta, 0o755); err != nil {
		return err
	}

	criadas, atualizadas, preservadas := 0, 0, 0
	for _, nome := range alvos {
		caminho := filepath.Join(pasta, nome+"_factory.go")

		atual, err := os.ReadFile(caminho)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		novo := os.IsNotExist(err)

		// Update=false é a forma de dizer "este arquivo é meu".
		if !novo && !permiteAtualizar(string(atual)) {
			preservadas++
			continue
		}

		// Numa regeneração, a expressão de cada coluna que já existe é
		// mantida como está: o gerador só acerta a lista de colunas contra a
		// migration. Sem isso um ajuste manual se perderia a cada `create`.
		var existentes map[string]string
		var ruler *migrate.Ruler
		if !novo {
			if arquivo, err := factorygo.ParseArquivo(caminho); err == nil {
				existentes = map[string]string{}
				for _, campo := range arquivo.Campos {
					existentes[strings.ToUpper(campo.Coluna)] = campo.Origem
				}
				copia := arquivo.Ruler
				ruler = &copia
			}
		}

		conteudo := renderizaFactory(formas[nome], checks[strings.ToUpper(formas[nome].Table)], state, existentes, ruler)

		if novo {
			if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
				return err
			}
			fmt.Printf("  %s %s\n", cliui.Success("+"), filepath.Base(caminho))
			criadas++
			continue
		}
		if string(atual) == conteudo {
			preservadas++
			continue
		}
		if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
			return err
		}
		fmt.Printf("  %s %s\n", cliui.Warning("~"), filepath.Base(caminho))
		atualizadas++
	}

	fmt.Printf("\n  %s %d criada(s), %d atualizada(s), %d preservada(s)\n",
		cliui.Success("✓ OK"), criadas, atualizadas, preservadas)
	return nil
}

// permiteAtualizar lê o Ruler do arquivo existente sem parsear tudo: se o autor
// marcou Update: false, o arquivo é dele.
func permiteAtualizar(conteudo string) bool {
	return !strings.Contains(strings.ReplaceAll(conteudo, " ", ""), "Update:false")
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

// renderizaFactory monta o arquivo. existentes traz as expressões já escritas
// para a tabela — elas vencem a heurística; ruler preserva o Count/Active que
// o autor tenha ajustado.
func renderizaFactory(forma acao.Operacao, checks map[string][]string, state config.ConfigState, existentes map[string]string, ruler *migrate.Ruler) string {
	tabela := strings.ToUpper(forma.Table)
	regra := migrate.Ruler{Count: 10, Update: true, Active: true}
	if ruler != nil {
		regra = *ruler
	}

	campos := make([][2]string, 0, len(forma.Columns))
	usaLocalidade := false
	for _, column := range forma.Columns {
		// Coluna de identidade é preenchida pelo banco; escrever nela obrigaria
		// a ligar IDENTITY_INSERT sem necessidade.
		if column.AutoIncrement {
			continue
		}
		expressao, mantida := existentes[strings.ToUpper(column.Name)]
		if !mantida {
			expressao = expressaoParaColuna(tabela, column, checks[strings.ToUpper(column.Name)], state)
		}
		if strings.Contains(expressao, "local(") {
			usaLocalidade = true
		}
		campos = append(campos, [2]string{column.Name, expressao})
	}

	largura := 0
	for _, campo := range campos {
		if tamanho := len(campo[0]) + 3; tamanho > largura {
			largura = tamanho
		}
	}

	var texto strings.Builder
	texto.WriteString("package factories\n\n")
	texto.WriteString("import migrate \"gokit/migration\"\n\n")
	fmt.Fprintf(&texto, "// %s gera dados fake para a tabela %s.\n", nomeDaFuncao(forma.Table), tabela)
	fmt.Fprintf(&texto, "func %s() migrate.Factory {\n", nomeDaFuncao(forma.Table))
	texto.WriteString("\treturn migrate.Factory{\n")
	fmt.Fprintf(&texto, "\t\tTable: %q,\n", tabela)
	fmt.Fprintf(&texto, "\t\tRuler: migrate.Ruler{Count: %d, Update: %t, Active: %t},\n", regra.Count, regra.Update, regra.Active)
	texto.WriteString("\t\tData: func(index int) migrate.Fields {\n")
	if usaLocalidade {
		texto.WriteString("\t\t\tlocal := migrate.FakeLocation()\n")
	}
	texto.WriteString("\t\t\treturn migrate.Fields{\n")
	for _, campo := range campos {
		chave := fmt.Sprintf("%q:", campo[0])
		fmt.Fprintf(&texto, "\t\t\t\t%-*s %s,\n", largura, chave, campo[1])
	}
	texto.WriteString("\t\t\t}\n\t\t},\n\t}\n}\n")

	// O alinhamento das colunas segue uma heurística própria do gofmt, que
	// quebra o bloco quando uma linha destoa muito das outras. Formatar com a
	// biblioteca padrão evita que o arquivo gerado apareça sujo no `gofmt -l`.
	formatado, err := format.Source([]byte(texto.String()))
	if err != nil {
		return texto.String()
	}
	return string(formatado)
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
func expressaoParaColuna(tabela string, column acao.ColunaDefinicao, check []string, state config.ConfigState) string {
	nome := strings.ToUpper(column.Name)

	if expressao, existe := overrideDoProjeto(tabela, nome, state); existe {
		return expressao
	}
	if column.ReferenceTable != "" {
		return fmt.Sprintf("migrate.Vinculo(%q, %q)", strings.ToUpper(column.ReferenceTable), strings.ToUpper(column.ReferenceColumn))
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
			return fmt.Sprintf("migrate.FakeCodeIndex(index, %d)", ouEntao(tamanho, 30))
		case "int", "integer", "decimal":
			return fmt.Sprintf("migrate.FakeIntIndex(index, 1, %d)", tetoNumerico(precisao))
		}
	}

	switch column.Type {
	case "date":
		return "migrate.FakeDate()"
	case "datetime", "timestamp":
		return "migrate.FakeDateTime()"
	case "boolean":
		return "migrate.FakeChoiceIndex(index, \"S\", \"N\")"
	case "binary":
		return "migrate.FakeBytes(128)"
	}

	documento := tamanho
	if documento == 0 && ehNumerica(column.Type) {
		documento = precisao
	}
	if strings.Contains(nome, "CNPJ") {
		return comTamanho("migrate.FakeUniqueCNPJ", "migrate.FakeUniqueCNPJLength", documento)
	}
	if strings.Contains(nome, "CPF") {
		return comTamanho("migrate.FakeUniqueCPF", "migrate.FakeUniqueCPFLength", documento)
	}

	if ehNumerica(column.Type) {
		if column.Scale > 0 {
			return fmt.Sprintf("migrate.FakeDecimal(%d, %d)", precisao, column.Scale)
		}
		if ehCategorica(nome) || precisao == 1 {
			return "migrate.FakeInt(1, 2)"
		}
		return fmt.Sprintf("migrate.FakeIntIndex(index, 1, %d)", tetoNumerico(precisao))
	}

	// Coluna de um caractere quase sempre é flag S/N, menos sexo.
	if tamanho == 1 && !strings.Contains(nome, "SEXO") {
		return "migrate.FakeChoiceIndex(index, \"S\", \"N\")"
	}

	switch {
	case strings.Contains(nome, "ESTADOCIVIL"), strings.Contains(nome, "ESTADO_CIVIL"):
		return "migrate.FakeChoiceIndex(index, \"1\", \"2\", \"3\", \"4\", \"5\")"
	case strings.Contains(nome, "SEXO"):
		return "migrate.FakeChoiceIndex(index, \"M\", \"F\")"
	case strings.Contains(nome, "EMAIL"):
		return porIndice("FakeEmailIndex", "FakeEmailIndexLength", tamanho)
	case strings.Contains(nome, "CEP"):
		return porIndice("FakeCEPIndex", "FakeCEPIndexLength", tamanho)
	case nome == "UF", strings.HasSuffix(nome, "_UF"), strings.Contains(nome, "RG_UF"), strings.Contains(nome, "CTPS_UF"):
		return localidade("uf", tamanho)
	case strings.Contains(nome, "FONE"), strings.Contains(nome, "TELEFONE"),
		strings.Contains(nome, "CELULAR"), strings.Contains(nome, "CONTATO"):
		return porIndice("FakePhoneIndex", "FakePhoneIndexLength", tamanho)
	case ehIP(nome):
		return "migrate.FakeIPv4()"
	case strings.Contains(nome, "USERAGENT"):
		return "migrate.FakeUserAgent()"
	case strings.Contains(nome, "REQUESTID"), strings.Contains(nome, "UUID"):
		return "migrate.FakeUUIDIndex(index)"
	case strings.Contains(nome, "METHOD"):
		return "migrate.FakeChoiceIndex(index, \"GET\", \"POST\", \"PUT\", \"PATCH\", \"DELETE\")"
	case strings.Contains(nome, "STATUSCODE"):
		return "migrate.FakeInt(200, 599)"
	case strings.Contains(nome, "HASH"), strings.Contains(nome, "SENHA"):
		if tamanho > 0 {
			return fmt.Sprintf("migrate.FakeHashIndexLength(index, %d)", tamanho)
		}
		return "migrate.FakeHashIndex(index)"
	case strings.Contains(nome, "LOGIN"), strings.Contains(nome, "USUARIO"):
		return porIndice("FakeUsernameIndex", "FakeUsernameIndexLength", tamanho)
	case strings.Contains(nome, "NOMEARQ"), strings.Contains(nome, "ARQPDF"), strings.Contains(nome, "FOTO"):
		return porIndice("FakeFileNameIndex", "FakeFileNameIndexLength", tamanho)
	case ehCodigoDeCidade(nome):
		return localidade("codigo_cidade", tamanho)
	case ehCidade(nome):
		return localidade("cidade", tamanho)
	case ehEstado(nome):
		if tamanho > 2 {
			return localidade("estado", tamanho)
		}
		return localidade("uf", tamanho)
	case strings.Contains(nome, "PAIS"):
		return localidade("nome_pais", tamanho)
	case strings.Contains(nome, "NOME"), strings.Contains(nome, "RAZAO_SOCIAL"):
		return porIndice("FakeNameIndex", "FakeNameIndexLength", tamanho)
	case strings.Contains(nome, "BAIRRO"):
		return porIndice("FakeDistrictIndex", "FakeDistrictIndexLength", tamanho)
	case strings.Contains(nome, "RUA"), strings.Contains(nome, "LOGRADOURO"):
		return porIndice("FakeStreetIndex", "FakeStreetIndexLength", tamanho)
	case strings.Contains(nome, "MATRICULA"):
		return "migrate.FakeMatricula(index)"
	case ehCodigo(nome):
		return fmt.Sprintf("migrate.FakeCode(index, %d)", ouEntao(tamanho, 30))
	}

	padrao := 255
	if column.Type == "text" {
		padrao = 500
	}
	return fmt.Sprintf("migrate.FakeUniqueText(index, %q, %d)", tituloDaColuna(nome), ouEntao(tamanho, padrao))
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
	return fmt.Sprintf("migrate.FakeChoiceIndex(index, %s)", strings.Join(citados, ", "))
}

func comTamanho(semTamanho, comTamanho string, tamanho int) string {
	if tamanho > 0 {
		return fmt.Sprintf("%s(%d)", comTamanho, tamanho)
	}
	return semTamanho + "()"
}

func porIndice(semTamanho, comTamanho string, tamanho int) string {
	if tamanho > 0 {
		return fmt.Sprintf("migrate.%s(index, %d)", comTamanho, tamanho)
	}
	return fmt.Sprintf("migrate.%s(index)", semTamanho)
}

func localidade(campo string, tamanho int) string {
	if tamanho > 0 {
		return fmt.Sprintf("local(index, %q, %d)", campo, tamanho)
	}
	return fmt.Sprintf("local(index, %q)", campo)
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
