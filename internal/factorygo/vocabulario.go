package factorygo

// Vocabulário das factories: o conjunto fechado de funções que o corpo de
// Data pode chamar.
//
// O gokit lê as factories por AST e nunca as compila, então não existe
// resolução de símbolo pelo compilador — a ponte entre o nome escrito no
// arquivo e a função de verdade é esta tabela. Uma função nova em
// migration/fake.go só passa a existir para as factories depois de registrada
// aqui.
//
// O ÍNDICE DA LINHA É IMPLÍCITO. Ele não aparece na chamada escrita: o motor já
// sabe em que linha está e o injeta ao executar. Antes cada nome vinha em dois
// sabores — `FakeName()` e `FakeNameIndex(index, ...)` — e o primeiro devolvia
// constante, o que fazia dez linhas saírem idênticas sem avisar. Agora existe um
// nome por conceito, e ele sempre varia por linha.
//
// A função pública correspondente em migration/fake.go, chamada de verdade fora
// do gokit, devolve o valor da PRIMEIRA linha. É a mesma regra para todas.

import (
	"sort"
	"strings"

	"github.com/PhelipeViana/gokit/internal/i18n"
	migrate "github.com/PhelipeViana/gokit/migration"
)

// chamadaFake executa uma função do vocabulário. `index` é a linha corrente, que
// o motor injeta; `args` são só os argumentos realmente escritos no arquivo.
type chamadaFake func(index int, args []any) (any, error)

// vocabulario mapeia o nome escrito na factory para a função correspondente.
//
// Onde há `length`, 0 significa "sem limite" — o gerador sempre passa o tamanho
// declarado na migration, e quem escreve à mão pode passar 0.
var vocabulario = map[string]chamadaFake{
	// ---------------------------------------------------------------- genéricos
	"FakeChoice":     textos(func(index int, valores ...string) any { return migrate.FakeChoiceIndex(index, valores...) }),
	"FakeInt":        doisInteiros(func(index, min, max int) any { return migrate.FakeIntIndex(index, min, max) }),
	"FakeDecimal":    doisInteiros(func(index, precision, scale int) any { return migrate.FakeDecimalIndex(index, precision, scale) }),
	"FakeString":     umInteiro(func(index, length int) any { return migrate.FakeStringIndex(index, length) }),
	"FakeText":       umInteiro(func(index, length int) any { return migrate.FakeTextIndex(index, length) }),
	"FakeBytes":      umInteiro(func(index, length int) any { return migrate.FakeBytesIndex(index, length) }),
	"FakeValue":      semArgumentos(func(index int) any { return migrate.FakeValue() }),
	"FakeUniqueText": textoInteiro(func(index int, prefix string, length int) any { return migrate.FakeUniqueTextIndex(index, prefix, length) }),

	// ------------------------------------------------------------------ códigos
	"FakeCode":       umInteiro(func(index, length int) any { return migrate.FakeCodeIndex(index, length) }),
	"FakeCodePrefix": textoInteiro(func(index int, prefix string, length int) any { return migrate.FakeCodePrefixIndex(index, prefix, length) }),
	"FakeMatricula":  semArgumentos(func(index int) any { return migrate.FakeMatriculaIndex(index) }),

	// ---------------------------------------------------------------- documentos
	// A família Unique é a única que NÃO é dirigida pelo índice: ela precisa
	// variar entre EXECUÇÕES para não colidir com documento que uma rodada
	// anterior já gravou. Por isso o índice recebido aqui é ignorado.
	"FakeUniqueCPF":  umInteiro(func(index, length int) any { return migrate.FakeUniqueCPF(length) }),
	"FakeUniqueCNPJ": umInteiro(func(index, length int) any { return migrate.FakeUniqueCNPJ(length) }),
	"FakeCPF":        umInteiro(func(index, length int) any { return migrate.FakeCPFIndexLength(index, length) }),
	"FakeCNPJ":       umInteiro(func(index, length int) any { return migrate.FakeCNPJIndexLength(index, length) }),

	// ---------------------------------------------------------------- identidade
	"FakeName":     inteiroTextos(func(index, length int, genders ...string) any { return migrate.FakeNameIndexLength(index, length, genders...) }),
	"FakeUsername": umInteiro(func(index, length int) any { return migrate.FakeUsernameIndexLength(index, length) }),
	"FakeEmail":    umInteiro(func(index, length int) any { return migrate.FakeEmailIndexLength(index, length) }),
	"FakePhone":    umInteiro(func(index, length int) any { return migrate.FakePhoneIndexLength(index, length) }),

	// ------------------------------------------------------------------ endereço
	"FakeCEP":      umInteiro(func(index, length int) any { return migrate.FakeCEPIndexLength(index, length) }),
	"FakeUF":       semArgumentos(func(index int) any { return migrate.FakeUFIndex(index) }),
	"FakeDistrict": umInteiro(func(index, length int) any { return migrate.FakeDistrictIndexLength(index, length) }),
	"FakeStreet":   umInteiro(func(index, length int) any { return migrate.FakeStreetIndexLength(index, length) }),
	"FakeCity":     umInteiro(func(index, length int) any { return migrate.FakeCityIndexLength(index, length) }),
	"FakeCityCode": umInteiro(func(index, length int) any { return migrate.FakeCityCodeIndexLength(index, length) }),
	"FakeState":    umInteiro(func(index, length int) any { return migrate.FakeStateIndexLength(index, length) }),
	"FakeCountry":  umInteiro(func(index, length int) any { return migrate.FakeCountryIndexLength(index, length) }),

	// --------------------------------------------------------------------- datas
	"FakeDate":     semArgumentos(func(index int) any { return migrate.FakeDateIndex(index) }),
	"FakeDateTime": semArgumentos(func(index int) any { return migrate.FakeDateTimeIndex(index) }),

	// -------------------------------------------------------------------- técnicos
	"FakeUUID":         semArgumentos(func(index int) any { return migrate.FakeUUIDIndex(index) }),
	"FakeHash":         umInteiro(func(index, length int) any { return migrate.FakeHashIndexLength(index, length) }),
	"FakeFileName":     umInteiro(func(index, length int) any { return migrate.FakeFileNameIndexLength(index, length) }),
	"FakeIPv4":         semArgumentos(func(index int) any { return migrate.FakeIPv4Index(index) }),
	"FakeUserAgent":    semArgumentos(func(index int) any { return migrate.FakeUserAgent() }),
	"FakeHashPassword": semArgumentos(func(index int) any { return migrate.FakeHashPassword() }),
}

// nomesConhecidos devolve o vocabulário em ordem, para mensagens de erro.
func nomesConhecidos() []string {
	nomes := make([]string, 0, len(vocabulario)+3)
	for nome := range vocabulario {
		nomes = append(nomes, nome)
	}
	nomes = append(nomes, "FakeLocation", "Reference", "Seeder")
	sort.Strings(nomes)
	return nomes
}

// sugestaoDeNome procura o nome conhecido mais próximo do que foi escrito.
// Erro de digitação em factory é comum, e a lista inteira não cabe na mensagem.
//
// É também o que orienta quem escreveu o nome antigo: `FakeNameIndexLength` cai
// perto de `FakeName`, então a mensagem já aponta o substituto.
func sugestaoDeNome(escrito string) string {
	melhor := ""
	menor := len(escrito)/2 + 2
	for _, nome := range nomesConhecidos() {
		if distancia := distanciaEntre(strings.ToLower(escrito), strings.ToLower(nome)); distancia < menor {
			menor = distancia
			melhor = nome
		}
	}
	return melhor
}

func distanciaEntre(a, b string) int {
	anterior := make([]int, len(b)+1)
	atual := make([]int, len(b)+1)
	for j := range anterior {
		anterior[j] = j
	}
	for i := 1; i <= len(a); i++ {
		atual[0] = i
		for j := 1; j <= len(b); j++ {
			custo := 1
			if a[i-1] == b[j-1] {
				custo = 0
			}
			atual[j] = minimo(atual[j-1]+1, anterior[j]+1, anterior[j-1]+custo)
		}
		copy(anterior, atual)
	}
	return anterior[len(b)]
}

func minimo(valores ...int) int {
	menor := valores[0]
	for _, valor := range valores[1:] {
		if valor < menor {
			menor = valor
		}
	}
	return menor
}

// ------------------------------------------------- adaptadores de assinatura
//
// Cada adaptador confere a quantidade de argumentos ESCRITOS — o índice não
// conta, porque não é escrito.

func semArgumentos(f func(index int) any) chamadaFake {
	return func(index int, args []any) (any, error) {
		if err := exigeQuantidade(args, 0); err != nil {
			return nil, err
		}
		return f(index), nil
	}
}

func umInteiro(f func(index, a int) any) chamadaFake {
	return func(index int, args []any) (any, error) {
		if err := exigeQuantidade(args, 1); err != nil {
			return nil, err
		}
		primeiro, err := inteiroEm(args, 0)
		if err != nil {
			return nil, err
		}
		return f(index, primeiro), nil
	}
}

func doisInteiros(f func(index, a, b int) any) chamadaFake {
	return func(index int, args []any) (any, error) {
		if err := exigeQuantidade(args, 2); err != nil {
			return nil, err
		}
		primeiro, err := inteiroEm(args, 0)
		if err != nil {
			return nil, err
		}
		segundo, err := inteiroEm(args, 1)
		if err != nil {
			return nil, err
		}
		return f(index, primeiro, segundo), nil
	}
}

func textoInteiro(f func(index int, texto string, a int) any) chamadaFake {
	return func(index int, args []any) (any, error) {
		if err := exigeQuantidade(args, 2); err != nil {
			return nil, err
		}
		texto, err := textoEm(args, 0)
		if err != nil {
			return nil, err
		}
		segundo, err := inteiroEm(args, 1)
		if err != nil {
			return nil, err
		}
		return f(index, texto, segundo), nil
	}
}

func textos(f func(index int, valores ...string) any) chamadaFake {
	return func(index int, args []any) (any, error) {
		lista, err := textosDe(args, 0)
		if err != nil {
			return nil, err
		}
		if len(lista) == 0 {
			return nil, i18n.Errf("fcp_needs_values")
		}
		return f(index, lista...), nil
	}
}

func inteiroTextos(f func(index, a int, valores ...string) any) chamadaFake {
	return func(index int, args []any) (any, error) {
		if len(args) < 1 {
			return nil, i18n.Errf("fcp_needs_size")
		}
		primeiro, err := inteiroEm(args, 0)
		if err != nil {
			return nil, err
		}
		lista, err := textosDe(args, 1)
		if err != nil {
			return nil, err
		}
		return f(index, primeiro, lista...), nil
	}
}

func exigeQuantidade(args []any, esperado int) error {
	if len(args) != esperado {
		return i18n.Errf("fcp_needs_n_got", esperado, len(args))
	}
	return nil
}

func inteiroEm(args []any, posicao int) (int, error) {
	switch valor := args[posicao].(type) {
	case int64:
		return int(valor), nil
	case int:
		return valor, nil
	}
	return 0, i18n.Errf("fcp_arg_int", posicao+1, args[posicao])
}

func textoEm(args []any, posicao int) (string, error) {
	if texto, ok := args[posicao].(string); ok {
		return texto, nil
	}
	return "", i18n.Errf("fcp_arg_text", posicao+1, args[posicao])
}

func textosDe(args []any, inicio int) ([]string, error) {
	textos := make([]string, 0, len(args)-inicio)
	for posicao := inicio; posicao < len(args); posicao++ {
		texto, err := textoEm(args, posicao)
		if err != nil {
			return nil, err
		}
		textos = append(textos, texto)
	}
	return textos, nil
}
