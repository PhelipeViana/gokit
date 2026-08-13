package migrate

// Biblioteca de valores fake das factories.
//
// Toda função aqui é determinística em relação ao índice da linha: a mesma
// factory rodada duas vezes produz os mesmos dados. Isso é proposital — sem
// isso não há como comparar o resultado entre os quatro bancos, que é o teste
// que garante que a factory é portável.
//
// As exceções são FakeUnique* e FakeHash*, que precisam variar entre execuções
// para não colidir com dados já gravados, e estão marcadas individualmente.
//
// Estas funções são chamadas por nome pelo avaliador de AST (o plugin nunca
// compila as factories). Renomear uma delas quebra os arquivos existentes;
// veja o catálogo em internal/factoryrun/vocabulario.go.

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Link é uma referência a uma coluna de outra tabela. O executor resolve o
// valor a partir das linhas realmente inseridas na tabela pai, então a FK
// sempre aponta para algo que existe.
type Link struct {
	Table  string
	Column string
	// Modo restringe quais valores do pai podem ser usados: "" percorre todos,
	// "fixed" e "random" usam só os listados em Valores.
	Modo    string
	Valores []any
}

// Seeder aponta a referência para valores ESPECÍFICOS da tabela pai — tipicamente
// os IDs de seed, que são estáveis entre ambientes por contrato.
//
//	migrate.Seeder(core.Table.Users, 1)          // sempre o ID 1
//	migrate.Seeder(core.Table.Users, 1, 2, 15)   // varia entre esses, na ordem
//
// Um valor é fixo por construção; vários são percorridos na ordem declarada,
// conforme o índice da linha. Não há sorteio: a mesma factory produz o mesmo
// resultado, e é isso que permite comparar os quatro bancos.
//
// Os valores podem ser escritos como número ou como texto — 1 e "1" são o mesmo
// alvo, e a comparação é feita pelo texto.
//
// Se nenhum dos valores existir na tabela pai no momento da execução, a restrição
// é IGNORADA em silêncio e o fluxo segue com o comportamento padrão. Uma FK
// apontando para outra linha válida é melhor que uma execução interrompida — e
// pior que as duas seria gravar referência para linha inexistente.
func Seeder(table Table, valores ...any) Link {
	return Link{Table: string(table), Modo: "seeder", Valores: valores}
}

// Reference declara que a coluna recebe um valor que JÁ EXISTE na coluna da
// tabela pai — das linhas geradas na mesma execução, ou do banco.
//
//	core.Column.Users.CidadeId: migrate.Reference(core.Table.Cidades, core.Column.Cidades.Id)
//
// Não é dado fake: por isso não se chama Fake*. As funções Fake* são
// determinísticas no índice; esta depende do que a tabela pai tem.
//
// Cuidado para não confundir com o References() da coluna, que é outra coisa:
// aquele DECLARA a integridade no banco; este ESCOLHE um valor para a factory.
// A coluna é opcional: quando a FK está declarada na migration, o gokit já sabe
// para qual coluna ela aponta. Escrevê-la é o caso de schema legado, sem FK.
//
//	migrate.Reference(core.Table.Users)                       // qualquer linha do pai
//	migrate.Reference(core.Table.Users, core.Column.Users.Id) // coluna explícita
//
// Para apontar valores específicos, use Seeder.
func Reference(table Table, column ...ColumnName) Link {
	link := Link{Table: string(table)}
	if len(column) > 0 {
		link.Column = string(column[0])
	}
	return link
}

// ---------------------------------------------------------------------------
// Camada pública: um nome por conceito, sem índice na assinatura.
//
// É esta a lista que as factories escrevem. O índice da linha é IMPLÍCITO: o
// motor sabe em que linha está e injeta na hora de avaliar, então escrevê-lo
// seria repetir para o motor algo que ele já sabe.
//
// Chamada de verdade, fora do gokit, cada uma devolve o valor da PRIMEIRA linha.
// É uma regra só, válida para todas — diferente do desenho anterior, em que cada
// conceito tinha dois nomes e o mais curto devolvia constante, fazendo dez linhas
// saírem idênticas sem avisar.
//
// `length` é o limite da coluna; 0 é sem limite. As funções *Index abaixo são a
// camada de implementação: elas continuam existindo e recebem o índice de
// verdade, mas não estão no vocabulário e não podem ser escritas numa factory.
// ---------------------------------------------------------------------------

func FakeChoice(values ...string) string           { return FakeChoiceIndex(0, values...) }
func FakeInt(min int, max int) int                 { return FakeIntIndex(0, min, max) }
func FakeDecimal(precision int, scale int) float64 { return FakeDecimalIndex(0, precision, scale) }
func FakeString(length int) string                 { return FakeStringIndex(0, length) }
func FakeText(length int) string                   { return FakeTextIndex(0, length) }
func FakeBytes(length int) []byte                  { return FakeBytesIndex(0, length) }
func FakeValue() any                               { return nil }

func FakeUniqueText(prefix string, length int) string { return FakeUniqueTextIndex(0, prefix, length) }
func FakeCode(length int) string                      { return FakeCodeIndex(0, length) }
func FakeCodePrefix(prefix string, length int) string { return FakeCodePrefixIndex(0, prefix, length) }
func FakeMatricula() string                           { return FakeMatriculaIndex(0) }

// FakeUniqueCPF e FakeUniqueCNPJ são a exceção à regra do índice: elas variam
// entre EXECUÇÕES, não entre linhas, para não colidir com documento que uma
// rodada anterior já gravou. Por isso ignoram o índice.
func FakeUniqueCPF(length int) string  { return formatDocumentLength(uniqueCPF(), length) }
func FakeUniqueCNPJ(length int) string { return formatDocumentLength(uniqueCNPJ(), length) }

func FakeCPF(length int) string  { return FakeCPFIndexLength(0, length) }
func FakeCNPJ(length int) string { return FakeCNPJIndexLength(0, length) }

func FakeName(length int, genders ...string) string {
	return FakeNameIndexLength(0, length, genders...)
}
func FakeUsername(length int) string { return FakeUsernameIndexLength(0, length) }
func FakeEmail(length int) string    { return FakeEmailIndexLength(0, length) }
func FakePhone(length int) string    { return FakePhoneIndexLength(0, length) }

func FakeCEP(length int) string      { return FakeCEPIndexLength(0, length) }
func FakeUF() string                 { return FakeUFIndex(0) }
func FakeDistrict(length int) string { return FakeDistrictIndexLength(0, length) }
func FakeStreet(length int) string   { return FakeStreetIndexLength(0, length) }
func FakeCity(length int) string     { return FakeCityIndexLength(0, length) }
func FakeCityCode(length int) string { return FakeCityCodeIndexLength(0, length) }
func FakeState(length int) string    { return FakeStateIndexLength(0, length) }
func FakeCountry(length int) string  { return FakeCountryIndexLength(0, length) }

func FakeDate() time.Time     { return FakeDateIndex(0) }
func FakeDateTime() time.Time { return FakeDateTimeIndex(0) }

func FakeUUID() string               { return FakeUUIDIndex(0) }
func FakeHash(length int) string     { return FakeHashIndexLength(0, length) }
func FakeFileName(length int) string { return FakeFileNameIndexLength(0, length) }
func FakeIPv4() string               { return FakeIPv4Index(0) }

// ------------------------------------------------- camada de implementação

// FakeDecimalIndex caminha dentro da precisão declarada, começando no menor
// valor representável com aquela escala.
func FakeDecimalIndex(index int, precision int, scale int) float64 {
	if precision <= 0 {
		precision = 10
	}
	if scale < 0 {
		scale = 0
	}
	inteiras := precision - scale
	if inteiras < 1 {
		inteiras = 1
	}
	maximo := math.Pow10(inteiras) - 1/math.Pow10(scale)
	if scale == 0 {
		maximo = math.Pow10(inteiras) - 1
		return float64((positiveIndex(index) % int(maximo+1)))
	}
	passo := 1 / math.Pow10(scale)
	valor := passo + float64(positiveIndex(index))*(1+passo)
	if valor > maximo {
		// Volta ao começo em vez de estourar a precisão: DECIMAL(4,2) não aceita
		// 100.00, e o erro viria do banco, não da factory.
		valor = passo + math.Mod(valor, maximo)
	}
	return math.Round(valor*math.Pow10(scale)) / math.Pow10(scale)
}

func FakeStringIndex(index int, length int) string {
	return limitFakeText(fmt.Sprintf("Texto fake %d", positiveIndex(index)+1), length)
}

func FakeTextIndex(index int, length int) string {
	return limitFakeText(fmt.Sprintf("Texto gerado automaticamente para validar a factory na linha %d.", positiveIndex(index)+1), length)
}

func FakeBytesIndex(index int, length int) []byte {
	if length <= 0 {
		return []byte{}
	}
	return []byte(FakeStringIndex(index, length))
}

// FakeChoiceIndex percorre os valores de forma circular conforme o índice da
// linha, de modo que um lote de 10 linhas exercita todas as opções de um CHECK.
func FakeChoiceIndex(index int, values ...string) string {
	if len(values) == 0 {
		return ""
	}
	return values[positiveIndex(index)%len(values)]
}

func FakeIntIndex(index int, min int, max int) int {
	if max < min {
		return max
	}
	value := min + positiveIndex(index)
	if value <= max {
		return value
	}
	span := max - min + 1
	if span <= 0 {
		return min
	}
	return min + (positiveIndex(index) % span)
}

func FakeUniqueTextIndex(index int, prefix string, length int) string {
	return limitFakeText(fmt.Sprintf("%s %d", prefix, positiveIndex(index)+1), length)
}

func FakeCodePrefixIndex(index int, prefix string, length int) string {
	return tailFakeText(fmt.Sprintf("%s%d", prefix, positiveIndex(index)+1), length)
}

// FakeCodeIndex gera um código único por índice. É o padrão para chave
// primária de texto sem identity.
func FakeCodeIndex(index int, length int) string {
	return tailFakeText(fmt.Sprintf("COD-%d", positiveIndex(index)+1), length)
}

func FakeMatriculaIndex(index int) string {
	return fmt.Sprintf("MAT%06d", positiveIndex(index)+1)
}

// As sequências de documento variam a cada execução para não colidir com CPFs
// e CNPJs já gravados no banco por uma rodada anterior.
var (
	uniqueCPFSequence  atomic.Uint64
	uniqueCNPJSequence atomic.Uint64
)

func init() {
	seed := uint64(time.Now().UnixNano()) ^ uint64(os.Getpid())*0x9e3779b97f4a7c15
	uniqueCPFSequence.Store(seed % 1_000_000_000)
	uniqueCNPJSequence.Store((seed >> 17) % 100_000_000)
}

// uniqueCPF gera um CPF válido e único durante a execução atual.
func uniqueCPF() string {
	for {
		base := uniqueCPFSequence.Add(1) % 1_000_000_000
		digits := digitsFromNumber(int(base), 9)
		if allDocumentDigitsEqual(digits) {
			continue
		}
		first := cpfDigit(digits, 10)
		second := cpfDigit(append(digits, first), 11)
		return formatCPF(digits, first, second)
	}
}

// uniqueCNPJ gera um CNPJ válido e único durante a execução atual.
func uniqueCNPJ() string {
	root := uniqueCNPJSequence.Add(1) % 100_000_000
	digits := append(digitsFromNumber(int(root), 8), 0, 0, 0, 1)
	first := cnpjDigit(digits, []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	second := cnpjDigit(append(digits, first), []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	return formatCNPJ(digits, first, second)
}

func FakeCPFIndex(index int) string {
	digits := digitsFromNumber(100000000+positiveIndex(index), 9)
	first := cpfDigit(digits, 10)
	second := cpfDigit(append(digits, first), 11)
	return formatCPF(digits, first, second)
}

func FakeCPFIndexLength(index int, length int) string {
	return formatDocumentLength(FakeCPFIndex(index), length)
}

func FakeCNPJIndex(index int) string {
	digits := digitsFromNumber(112223330001+positiveIndex(index), 12)
	first := cnpjDigit(digits, []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	second := cnpjDigit(append(digits, first), []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	return formatCNPJ(digits, first, second)
}

func FakeCNPJIndexLength(index int, length int) string {
	return formatDocumentLength(FakeCNPJIndex(index), length)
}

func formatCPF(digits []int, first int, second int) string {
	return fmt.Sprintf(
		"%d%d%d.%d%d%d.%d%d%d-%d%d",
		digits[0], digits[1], digits[2],
		digits[3], digits[4], digits[5],
		digits[6], digits[7], digits[8],
		first, second,
	)
}

func formatCNPJ(digits []int, first int, second int) string {
	return fmt.Sprintf(
		"%d%d.%d%d%d.%d%d%d/%d%d%d%d-%d%d",
		digits[0], digits[1],
		digits[2], digits[3], digits[4],
		digits[5], digits[6], digits[7],
		digits[8], digits[9], digits[10], digits[11],
		first, second,
	)
}

// formatDocumentLength tira a máscara quando ela não cabe: uma coluna
// CHAR(11) recebe os dígitos puros em vez de um CPF cortado no meio.
func formatDocumentLength(value string, length int) string {
	if length <= 0 || length >= len(value) {
		return value
	}
	return limitFakeText(onlyDigits(value), length)
}

func allDocumentDigitsEqual(digits []int) bool {
	if len(digits) == 0 {
		return true
	}
	for _, digit := range digits[1:] {
		if digit != digits[0] {
			return false
		}
	}
	return true
}

func FakeEmailIndex(index int) string {
	return fmt.Sprintf("usuario.%03d@example.com", positiveIndex(index)+1)
}

func FakeEmailIndexLength(index int, length int) string {
	return limitFakeText(FakeEmailIndex(index), length)
}

func FakeCEPIndex(index int) string {
	return fmt.Sprintf("78%03d-%03d", positiveIndex(index)%1000, (positiveIndex(index)+100)%1000)
}

func FakeCEPIndexLength(index int, length int) string {
	return formatDocumentLength(FakeCEPIndex(index), length)
}

func FakeUFIndex(index int) string { return fakeLocationIndex(index).UF }

func FakePhoneIndex(index int) string {
	number := 900000000 + (positiveIndex(index) % 99999999)
	return fmt.Sprintf("(65) %05d-%04d", number/10000, number%10000)
}

func FakePhoneIndexLength(index int, length int) string {
	return formatDocumentLength(FakePhoneIndex(index), length)
}

// FakeIPv4Index caminha no último octeto, começando em 127.0.0.1.
func FakeIPv4Index(index int) string {
	return fmt.Sprintf("127.0.0.%d", 1+positiveIndex(index)%254)
}

// FakeUserAgent é constante de propósito: user-agent repetido em dez linhas é
// dado plausível, e nenhuma coluna de user-agent é única.
func FakeUserAgent() string { return "GoKitFactory/1.0" }

func FakeUUIDIndex(index int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", positiveIndex(index)+1)
}

// FakeHash varia a cada execução: serve para colunas com índice único onde
// repetir o valor de uma rodada anterior causaria violação.
func FakeHashIndex(index int) string { return fakeHashIndexValue(index, 64) }

func FakeHashIndexLength(index int, length int) string {
	if length <= 0 {
		return FakeHashIndex(index)
	}
	return fakeHashIndexValue(index, length)
}

func fakeHashIndexValue(index int, length int) string {
	if length <= 0 {
		return ""
	}
	positive := positiveIndex(index) + 1
	now := time.Now().UTC()
	seed := now.UnixNano() + int64(positive*1_000_003)
	value := "h" + now.Format("060102150405") + strconv.FormatInt(int64(positive), 36) + strconv.FormatInt(seed, 36)
	for len(value) < length {
		seed += int64(len(value)*7_919 + positive)
		value += strconv.FormatInt(seed, 36)
	}
	return limitFakeText(value, length)
}

// FakeHashPassword é o bcrypt de uma senha conhecida, para permitir login nos
// ambientes de teste.
func FakeHashPassword() string {
	return "$2y$12$3YZte70BSGA0rDmtnRH1t.8M696/MOUR940JfvjeanBfGY/TTI6Ve"
}

func FakeUsernameIndex(index int) string {
	return fmt.Sprintf("usuario.teste.%03d", positiveIndex(index)+1)
}

func FakeUsernameIndexLength(index int, length int) string {
	return limitFakeText(FakeUsernameIndex(index), length)
}

func FakeFileNameIndex(index int) string {
	return fmt.Sprintf("doc_teste_%03d.pdf", positiveIndex(index)+1)
}

func FakeFileNameIndexLength(index int, length int) string {
	return limitFakeText(FakeFileNameIndex(index), length)
}

// dataBase é a origem das datas fake. Ficava embutida em FakeDate(), mas depois
// que FakeDate passou a ser a amostra da primeira linha — ou seja,
// FakeDateIndex(0) — usá-la como base fecharia um ciclo infinito.
var (
	dataBase     = time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
	dataHoraBase = time.Date(2024, 1, 1, 9, 30, 0, 0, time.Local)
)

// FakeDateIndex devolve data que VARIA com o índice: um mês por índice, a partir
// de 2024-01-01. É o que faz intervalo, ordenação e MIN/MAX por data terem o que
// exercitar.
func FakeDateIndex(index int) time.Time {
	return dataBase.AddDate(0, normalizaIndiceData(index), 0)
}

// FakeDateTimeIndex é a versão com hora: avança mês e hora conforme o índice.
func FakeDateTimeIndex(index int) time.Time {
	i := normalizaIndiceData(index)
	return dataHoraBase.AddDate(0, i, 0).Add(time.Duration(i) * time.Hour)
}

// normalizaIndiceData mantém o índice em 0..11 para as datas ficarem no mesmo
// ano-base, mesmo com factories de contagem alta.
func normalizaIndiceData(index int) int {
	if index < 0 {
		index = -index
	}
	return index % 12
}

// ---------------------------------------------------------------- localidade

type fakeLocation struct {
	City     string
	State    string
	UF       string
	CityCode string
	Country  string
}

var fakeLocations = []fakeLocation{
	{City: "Cuiaba", State: "Mato Grosso", UF: "MT", CityCode: "01", Country: "Brasil"},
	{City: "Varzea Grande", State: "Mato Grosso", UF: "MT", CityCode: "02", Country: "Brasil"},
	{City: "Rondonopolis", State: "Mato Grosso", UF: "MT", CityCode: "03", Country: "Brasil"},
	{City: "Sinop", State: "Mato Grosso", UF: "MT", CityCode: "04", Country: "Brasil"},
	{City: "Sao Paulo", State: "Sao Paulo", UF: "SP", CityCode: "05", Country: "Brasil"},
	{City: "Campinas", State: "Sao Paulo", UF: "SP", CityCode: "06", Country: "Brasil"},
	{City: "Santos", State: "Sao Paulo", UF: "SP", CityCode: "07", Country: "Brasil"},
	{City: "Ribeirao Preto", State: "Sao Paulo", UF: "SP", CityCode: "08", Country: "Brasil"},
	{City: "Rio de Janeiro", State: "Rio de Janeiro", UF: "RJ", CityCode: "09", Country: "Brasil"},
	{City: "Niteroi", State: "Rio de Janeiro", UF: "RJ", CityCode: "10", Country: "Brasil"},
	{City: "Petropolis", State: "Rio de Janeiro", UF: "RJ", CityCode: "11", Country: "Brasil"},
	{City: "Volta Redonda", State: "Rio de Janeiro", UF: "RJ", CityCode: "12", Country: "Brasil"},
	{City: "Goiania", State: "Goias", UF: "GO", CityCode: "13", Country: "Brasil"},
	{City: "Anapolis", State: "Goias", UF: "GO", CityCode: "14", Country: "Brasil"},
	{City: "Rio Verde", State: "Goias", UF: "GO", CityCode: "15", Country: "Brasil"},
	{City: "Luziania", State: "Goias", UF: "GO", CityCode: "16", Country: "Brasil"},
}

func fakeLocationIndex(index int) fakeLocation {
	if len(fakeLocations) == 0 {
		return fakeLocation{}
	}
	return fakeLocations[positiveIndex(index)%len(fakeLocations)]
}

func FakeDistrictIndex(index int) string {
	return pickFake(index, []string{"Centro", "Jardim das Americas", "Boa Esperanca", "Santa Rosa", "Morada do Ouro"})
}

func FakeDistrictIndexLength(index int, length int) string {
	return limitFakeText(FakeDistrictIndex(index), length)
}

func FakeStreetIndex(index int) string {
	return pickFake(index, []string{"Rua das Flores", "Avenida Brasil", "Rua Sao Jose", "Avenida Mato Grosso", "Rua das Palmeiras"})
}

func FakeStreetIndexLength(index int, length int) string {
	return limitFakeText(FakeStreetIndex(index), length)
}

func FakeCityIndex(index int) string { return fakeLocationIndex(index).City }

func FakeCityIndexLength(index int, length int) string {
	return limitFakeText(FakeCityIndex(index), length)
}

func FakeCityCodeIndex(index int) string { return fakeLocationIndex(index).CityCode }

func FakeCityCodeIndexLength(index int, length int) string {
	return limitFakeText(FakeCityCodeIndex(index), length)
}

func FakeStateIndex(index int) string { return fakeLocationIndex(index).State }

// FakeStateIndexLength cai para a sigla quando o nome do estado não cabe.
func FakeStateIndexLength(index int, length int) string {
	location := fakeLocationIndex(index)
	if length >= len(location.State) {
		return location.State
	}
	if length >= len(location.UF) {
		return location.UF
	}
	return limitFakeText(location.UF, length)
}

// FakeCountryIndex é o país da mesma localidade da linha. Existia só como campo
// do acessor de localidade; virou função própria quando o acessor saiu.
func FakeCountryIndex(index int) string { return fakeLocationIndex(index).Country }

func FakeCountryIndexLength(index int, length int) string {
	return limitFakeText(FakeCountryIndex(index), length)
}

// -------------------------------------------------------------------- nomes

const (
	FakeNameMasculino = "masculino"
	FakeNameFeminino  = "feminino"
)

// Estas listas podem ser ampliadas manualmente sem alterar a composição dos nomes.
var (
	fakeMaleFirstNames = []string{
		"Carlos", "Jose", "Caio", "Joao", "Pedro", "Lucas", "Mateus", "Gabriel", "Rafael", "Bruno",
		"Felipe", "Gustavo", "Leonardo", "Vinicius", "Marcos", "Andre", "Thiago", "Daniel", "Eduardo", "Fernando",
		"Rodrigo", "Ricardo", "Marcelo", "Alexandre", "Diego", "Henrique", "Murilo", "Vitor", "Arthur", "Miguel",
		"Davi", "Samuel", "Matheus", "Antonio", "Luiz", "Paulo", "Renato", "Leandro", "Igor", "Cesar",
	}

	fakeFemaleFirstNames = []string{
		"Ana", "Beatriz", "Joana", "Maria", "Julia", "Mariana", "Camila", "Larissa", "Amanda", "Fernanda",
		"Patricia", "Juliana", "Carolina", "Leticia", "Isabela", "Gabriela", "Rafaela", "Luana", "Bruna", "Aline",
		"Bianca", "Vanessa", "Priscila", "Renata", "Tatiane", "Cristina", "Daniela", "Eduarda", "Sofia", "Laura",
		"Helena", "Valentina", "Clara", "Manuela", "Livia", "Lorena", "Vitoria", "Yasmin", "Rebeca", "Natalia",
	}

	fakeLastNames = []string{
		"da Silva", "dos Santos", "Oliveira", "Souza", "Rodrigues",
		"Ferreira", "Alves", "Pereira", "Lima", "Gomes",
		"Ribeiro", "Carvalho", "Almeida", "Lopes", "Soares",
		"Fernandes", "Vieira", "Barbosa", "Rocha", "Dias",
		"Nascimento", "Andrade", "Moreira", "Nunes", "Marques",
		"Machado", "Mendes", "Freitas", "Cardoso", "Ramos",
		"Goncalves", "Santana", "Teixeira", "Correia", "Moura",
		"Batista", "Campos", "Monteiro", "Araujo", "Cavalcanti",
		"Rezende", "Borges", "Medeiros", "Farias", "Pinto",
		"Castro", "Duarte", "Melo", "Barros", "Neves",
		"Peixoto", "Tavares", "Amaral", "Cunha", "Sales",
		"Antunes", "Bezerra", "Coelho", "Leal", "Brito",
		"Aguiar", "Assis", "Queiroz", "Siqueira", "Xavier",
		"Figueiredo", "Pacheco", "Prado", "Bittencourt", "Garcia",
		"Guimaraes", "Moraes", "Miranda", "Azevedo", "Santos",
		"Vargas", "Valente", "Pinheiro", "Bandeira", "Cordeiro",
		"Esteves", "Furtado", "Macedo", "Magalhaes", "Navarro",
		"Paiva", "Porto", "Rangel", "Sampaio", "Seixas",
		"Toledo", "Vasconcelos", "Viana", "Bastos", "Caldeira",
		"Drummond", "Franco", "Godoy", "Junqueira", "Lacerda",
	}
)

func FakeNameIndex(index int, genders ...string) string {
	firstNames := fakeFirstNames(genders)
	firstNameCount := len(firstNames)
	lastNameCount := len(fakeLastNames)
	combinationCount := firstNameCount * lastNameCount * (lastNameCount - 1)
	combinationIndex := fakeNameShuffledIndex(positiveIndex(index), combinationCount)

	firstName := firstNames[combinationIndex%firstNameCount]

	// Cada sobrenome pode ser seguido por qualquer outro, exceto ele mesmo.
	lastNamePairIndex := combinationIndex / firstNameCount
	firstLastNameIndex := lastNamePairIndex / (lastNameCount - 1)
	secondLastNameIndex := lastNamePairIndex % (lastNameCount - 1)
	if secondLastNameIndex >= firstLastNameIndex {
		secondLastNameIndex++
	}

	return fmt.Sprintf(
		"%s %s %s",
		firstName,
		fakeLastNames[firstLastNameIndex],
		fakeLastNames[secondLastNameIndex],
	)
}

func FakeNameIndexLength(index int, length int, genders ...string) string {
	return limitFakeText(FakeNameIndex(index, genders...), length)
}

// fakeNameShuffledIndex aplica uma permutação determinística ao índice. O
// multiplicador coprimo ao total garante que nenhum valor se repita antes de
// todo o espaço de combinações ser percorrido.
func fakeNameShuffledIndex(index, total int) int {
	const (
		shuffleMultiplier = 104729
		shuffleOffset     = 7919
	)

	multiplier := shuffleMultiplier
	for greatestCommonDivisor(multiplier, total) != 1 {
		multiplier++
	}

	return int((int64(index%total)*int64(multiplier) + shuffleOffset) % int64(total))
}

func greatestCommonDivisor(first, second int) int {
	for second != 0 {
		first, second = second, first%second
	}
	return first
}

func fakeFirstNames(genders []string) []string {
	useMale := len(genders) == 0
	useFemale := len(genders) == 0

	for _, gender := range genders {
		switch strings.ToLower(strings.TrimSpace(gender)) {
		case FakeNameMasculino, "m":
			useMale = true
		case FakeNameFeminino, "f":
			useFemale = true
		}
	}

	firstNames := make([]string, 0, len(fakeMaleFirstNames)+len(fakeFemaleFirstNames))
	if useMale {
		firstNames = append(firstNames, fakeMaleFirstNames...)
	}
	if useFemale {
		firstNames = append(firstNames, fakeFemaleFirstNames...)
	}
	if len(firstNames) == 0 {
		firstNames = append(firstNames, fakeMaleFirstNames...)
		firstNames = append(firstNames, fakeFemaleFirstNames...)
	}

	return firstNames
}

// ------------------------------------------------------------------ apoio

// limitFakeText corta pelo limite de bytes da coluna sem partir um caractere
// multibyte no meio: VARCHAR2(5) conta bytes no Oracle, e meio caractere
// gravado vira lixo na leitura.
func limitFakeText(value string, length int) string {
	// 0 é "sem limite", como já era em tailFakeText e formatDocumentLength. Antes
	// esta devolvia vazio, então coluna de texto sem tamanho declarado nascia em
	// branco — o oposto de dado de teste útil.
	if length <= 0 {
		return value
	}
	if len(value) <= length {
		return value
	}
	cut := length
	for cut > 0 && !utf8Boundary(value, cut) {
		cut--
	}
	return value[:cut]
}

// tailFakeText preserva o final do texto, onde fica a parte que varia por
// índice. Cortar o começo mantém o valor único; cortar o fim não.
func tailFakeText(value string, length int) string {
	if length <= 0 || len(value) <= length {
		return value
	}
	start := len(value) - length
	for start < len(value) && !utf8Boundary(value, start) {
		start++
	}
	return value[start:]
}

// utf8Boundary informa se o offset cai no início de um caractere.
func utf8Boundary(value string, offset int) bool {
	if offset <= 0 || offset >= len(value) {
		return true
	}
	return value[offset]&0xC0 != 0x80
}

func onlyDigits(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func pickFake(index int, values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[positiveIndex(index)%len(values)]
}

func positiveIndex(index int) int {
	if index < 0 {
		return 0
	}
	return index
}

func digitsFromNumber(number int, size int) []int {
	if number < 0 {
		number = -number
	}
	digits := make([]int, size)
	for i := size - 1; i >= 0; i-- {
		digits[i] = number % 10
		number /= 10
	}
	return digits
}

func cpfDigit(digits []int, initialWeight int) int {
	sum := 0
	weight := initialWeight
	for _, digit := range digits {
		sum += digit * weight
		weight--
	}
	if remainder := sum % 11; remainder >= 2 {
		return 11 - remainder
	}
	return 0
}

func cnpjDigit(digits []int, weights []int) int {
	sum := 0
	for i, digit := range digits {
		sum += digit * weights[i]
	}
	if remainder := sum % 11; remainder >= 2 {
		return 11 - remainder
	}
	return 0
}
