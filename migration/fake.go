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
}

// Vinculo declara que a coluna recebe o valor de uma coluna da tabela pai.
//
//	"CIDADE_ID": migrate.Vinculo("CIDADES", "CIDADE_ID")
func Vinculo(table string, column string) Link {
	return Link{Table: table, Column: column}
}

// FakeChoice devolve sempre o primeiro valor. Use FakeChoiceIndex quando
// quiser variar entre as linhas.
func FakeChoice(values ...string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// FakeChoiceIndex percorre os valores de forma circular conforme o índice da
// linha, de modo que um lote de 10 linhas exercita todas as opções de um CHECK.
func FakeChoiceIndex(index int, values ...string) string {
	if len(values) == 0 {
		return ""
	}
	return values[positiveIndex(index)%len(values)]
}

func FakeInt(min int, max int) int {
	if max < min {
		return max
	}
	return min
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

func FakeDecimal(precision int, scale int) float64 {
	if precision <= 0 {
		precision = 10
	}
	if scale <= 0 {
		return float64(FakeInt(1, 9))
	}
	integerDigits := precision - scale
	if integerDigits < 1 {
		integerDigits = 1
	}
	value := 1 + 1/math.Pow10(scale)
	maxValue := math.Pow10(integerDigits) - (1 / math.Pow10(scale))
	if value > maxValue {
		value = maxValue
	}
	return math.Round(value*math.Pow10(scale)) / math.Pow10(scale)
}

func FakeString(length int) string {
	return limitFakeText("Texto fake para teste", length)
}

func FakeText(length int) string {
	return limitFakeText("Texto gerado automaticamente para validar a factory durante testes de desenvolvimento.", length)
}

func FakeUniqueText(index int, prefix string, length int) string {
	return limitFakeText(fmt.Sprintf("%s %d", prefix, positiveIndex(index)+1), length)
}

func FakeCode(index int, length int) string {
	return tailFakeText(fmt.Sprintf("COD-%d", positiveIndex(index)+1), length)
}

func FakeCodePrefix(index int, prefix string, length int) string {
	return tailFakeText(fmt.Sprintf("%s%d", prefix, positiveIndex(index)+1), length)
}

// FakeCodeIndex gera um código único por índice. É o padrão para chave
// primária de texto sem identity.
func FakeCodeIndex(index int, length int) string {
	return tailFakeText(fmt.Sprintf("COD-%d", positiveIndex(index)+1), length)
}

func FakeMatricula(index int) string {
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

// FakeUniqueCPF gera um CPF válido e único durante a execução atual.
func FakeUniqueCPF() string {
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

// FakeUniqueCPFLength gera um CPF único respeitando o tamanho da coluna.
func FakeUniqueCPFLength(length int) string {
	return formatDocumentLength(FakeUniqueCPF(), length)
}

// FakeUniqueCNPJ gera um CNPJ válido e único durante a execução atual.
func FakeUniqueCNPJ() string {
	root := uniqueCNPJSequence.Add(1) % 100_000_000
	digits := append(digitsFromNumber(int(root), 8), 0, 0, 0, 1)
	first := cnpjDigit(digits, []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	second := cnpjDigit(append(digits, first), []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	return formatCNPJ(digits, first, second)
}

// FakeUniqueCNPJLength gera um CNPJ único respeitando o tamanho da coluna.
func FakeUniqueCNPJLength(length int) string {
	return formatDocumentLength(FakeUniqueCNPJ(), length)
}

func FakeCPF() string { return FakeCPFIndex(0) }

func FakeCPFIndex(index int) string {
	digits := digitsFromNumber(100000000+positiveIndex(index), 9)
	first := cpfDigit(digits, 10)
	second := cpfDigit(append(digits, first), 11)
	return formatCPF(digits, first, second)
}

func FakeCPFIndexLength(index int, length int) string {
	return formatDocumentLength(FakeCPFIndex(index), length)
}

func FakeCNPJ() string { return FakeCNPJIndex(0) }

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

func FakeEmail() string { return FakeEmailIndex(0) }

func FakeEmailIndex(index int) string {
	return fmt.Sprintf("usuario.%03d@example.com", positiveIndex(index)+1)
}

func FakeEmailIndexLength(index int, length int) string {
	return limitFakeText(FakeEmailIndex(index), length)
}

func FakeCEP() string { return FakeCEPIndex(0) }

func FakeCEPIndex(index int) string {
	return fmt.Sprintf("78%03d-%03d", positiveIndex(index)%1000, (positiveIndex(index)+100)%1000)
}

func FakeCEPIndexLength(index int, length int) string {
	return formatDocumentLength(FakeCEPIndex(index), length)
}

func FakeUF() string { return FakeUFIndex(0) }

func FakeUFIndex(index int) string { return fakeLocationIndex(index).UF }

func FakePhone() string { return FakePhoneIndex(0) }

func FakePhoneIndex(index int) string {
	number := 900000000 + (positiveIndex(index) % 99999999)
	return fmt.Sprintf("(65) %05d-%04d", number/10000, number%10000)
}

func FakePhoneIndexLength(index int, length int) string {
	return formatDocumentLength(FakePhoneIndex(index), length)
}

func FakeIPv4() string { return "127.0.0.1" }

func FakeUserAgent() string { return "GoKitFactory/1.0" }

func FakeUUID() string { return FakeUUIDIndex(0) }

func FakeUUIDIndex(index int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", positiveIndex(index)+1)
}

// FakeHash varia a cada execução: serve para colunas com índice único onde
// repetir o valor de uma rodada anterior causaria violação.
func FakeHash() string { return FakeHashIndex(0) }

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

func FakeUsername() string { return FakeUsernameIndex(0) }

func FakeUsernameIndex(index int) string {
	return fmt.Sprintf("usuario.teste.%03d", positiveIndex(index)+1)
}

func FakeUsernameIndexLength(index int, length int) string {
	return limitFakeText(FakeUsernameIndex(index), length)
}

func FakeFileName() string { return FakeFileNameIndex(0) }

func FakeFileNameIndex(index int) string {
	return fmt.Sprintf("doc_teste_%03d.pdf", positiveIndex(index)+1)
}

func FakeFileNameIndexLength(index int, length int) string {
	return limitFakeText(FakeFileNameIndex(index), length)
}

func FakeDate() time.Time {
	return time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)
}

func FakeDateTime() time.Time {
	return time.Date(2024, 1, 1, 9, 30, 0, 0, time.Local)
}

func FakeBytes(length int) []byte {
	if length <= 0 {
		return []byte{}
	}
	return []byte(FakeString(length))
}

func FakeValue() any { return nil }

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

// FakeLocation devolve um acessor que mantém cidade, estado, UF e código
// coerentes entre si na mesma linha — sem isso a factory gera "Cuiabá/SP".
//
//	local := migrate.FakeLocation()
//	"CIDADE": local(index, "cidade", 60),
//	"UF":     local(index, "uf", 2),
func FakeLocation() func(index int, field string, length ...int) string {
	return func(index int, field string, length ...int) string {
		location := fakeLocationIndex(index)
		var value string
		switch strings.ToLower(field) {
		case "cidade", "city", "nome_cidade":
			value = location.City
		case "estado", "state", "nome_estado":
			value = location.State
		case "uf", "sigla", "uf_sigla":
			value = location.UF
		case "codigo_cidade", "cod_cidade", "codigo_municipio", "cod_municipio":
			value = location.CityCode
		case "pais", "nome_pais", "country":
			value = location.Country
		default:
			value = location.City
		}
		if len(length) == 0 {
			return value
		}
		limit := length[0]
		// "Mato Grosso" não cabe em VARCHAR(2), mas "MT" cabe e continua correto.
		if strings.EqualFold(field, "estado") && len(value) > limit && limit >= len(location.UF) {
			return location.UF
		}
		return limitFakeText(value, limit)
	}
}

func fakeLocationIndex(index int) fakeLocation {
	if len(fakeLocations) == 0 {
		return fakeLocation{}
	}
	return fakeLocations[positiveIndex(index)%len(fakeLocations)]
}

func FakeDistrict() string { return FakeDistrictIndex(0) }

func FakeDistrictIndex(index int) string {
	return pickFake(index, []string{"Centro", "Jardim das Americas", "Boa Esperanca", "Santa Rosa", "Morada do Ouro"})
}

func FakeDistrictIndexLength(index int, length int) string {
	return limitFakeText(FakeDistrictIndex(index), length)
}

func FakeStreet() string { return FakeStreetIndex(0) }

func FakeStreetIndex(index int) string {
	return pickFake(index, []string{"Rua das Flores", "Avenida Brasil", "Rua Sao Jose", "Avenida Mato Grosso", "Rua das Palmeiras"})
}

func FakeStreetIndexLength(index int, length int) string {
	return limitFakeText(FakeStreetIndex(index), length)
}

func FakeCity() string { return FakeCityIndex(0) }

func FakeCityIndex(index int) string { return fakeLocationIndex(index).City }

func FakeCityIndexLength(index int, length int) string {
	return limitFakeText(FakeCityIndex(index), length)
}

func FakeCityCode() string { return FakeCityCodeIndex(0) }

func FakeCityCodeIndex(index int) string { return fakeLocationIndex(index).CityCode }

func FakeCityCodeIndexLength(index int, length int) string {
	return limitFakeText(FakeCityCodeIndex(index), length)
}

func FakeState() string { return FakeStateIndex(0) }

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

func FakeName() string { return FakeNameIndex(0) }

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
	if length <= 0 {
		return ""
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
