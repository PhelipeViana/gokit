package factorygo

// Vocabulário das factories: o conjunto fechado de funções que o corpo de
// Data pode chamar.
//
// O gokit lê as factories por AST e nunca as compila, então não existe
// resolução de símbolo pelo compilador — a ponte entre o nome escrito no
// arquivo e a função de verdade é esta tabela. Uma função nova em
// migration/fake.go só passa a existir para as factories depois de registrada
// aqui.

import (
	"fmt"
	"sort"
	"strings"

	migrate "gokit/migration"
)

// chamadaFake executa uma função do vocabulário com os argumentos já avaliados.
type chamadaFake func(args []any) (any, error)

// vocabulario mapeia o nome escrito na factory para a função correspondente.
var vocabulario = map[string]chamadaFake{
	"FakeChoice":      variadicos(func(values ...string) any { return migrate.FakeChoice(values...) }),
	"FakeChoiceIndex": inteiroVariadicos(func(index int, values ...string) any { return migrate.FakeChoiceIndex(index, values...) }),

	"FakeInt":      doisInteiros(func(min, max int) any { return migrate.FakeInt(min, max) }),
	"FakeIntIndex": tresInteiros(func(index, min, max int) any { return migrate.FakeIntIndex(index, min, max) }),
	"FakeDecimal":  doisInteiros(func(precision, scale int) any { return migrate.FakeDecimal(precision, scale) }),

	"FakeString":     umInteiro(func(length int) any { return migrate.FakeString(length) }),
	"FakeText":       umInteiro(func(length int) any { return migrate.FakeText(length) }),
	"FakeUniqueText": inteiroTextoInteiro(func(index int, prefix string, length int) any { return migrate.FakeUniqueText(index, prefix, length) }),

	"FakeCode":       doisInteiros(func(index, length int) any { return migrate.FakeCode(index, length) }),
	"FakeCodeIndex":  doisInteiros(func(index, length int) any { return migrate.FakeCodeIndex(index, length) }),
	"FakeCodePrefix": inteiroTextoInteiro(func(index int, prefix string, length int) any { return migrate.FakeCodePrefix(index, prefix, length) }),
	"FakeMatricula":  umInteiro(func(index int) any { return migrate.FakeMatricula(index) }),

	"FakeUniqueCPF":        semArgumentos(func() any { return migrate.FakeUniqueCPF() }),
	"FakeUniqueCPFLength":  umInteiro(func(length int) any { return migrate.FakeUniqueCPFLength(length) }),
	"FakeUniqueCNPJ":       semArgumentos(func() any { return migrate.FakeUniqueCNPJ() }),
	"FakeUniqueCNPJLength": umInteiro(func(length int) any { return migrate.FakeUniqueCNPJLength(length) }),

	"FakeCPF":             semArgumentos(func() any { return migrate.FakeCPF() }),
	"FakeCPFIndex":        umInteiro(func(index int) any { return migrate.FakeCPFIndex(index) }),
	"FakeCPFIndexLength":  doisInteiros(func(index, length int) any { return migrate.FakeCPFIndexLength(index, length) }),
	"FakeCNPJ":            semArgumentos(func() any { return migrate.FakeCNPJ() }),
	"FakeCNPJIndex":       umInteiro(func(index int) any { return migrate.FakeCNPJIndex(index) }),
	"FakeCNPJIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeCNPJIndexLength(index, length) }),

	"FakeEmail":            semArgumentos(func() any { return migrate.FakeEmail() }),
	"FakeEmailIndex":       umInteiro(func(index int) any { return migrate.FakeEmailIndex(index) }),
	"FakeEmailIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeEmailIndexLength(index, length) }),

	"FakeCEP":            semArgumentos(func() any { return migrate.FakeCEP() }),
	"FakeCEPIndex":       umInteiro(func(index int) any { return migrate.FakeCEPIndex(index) }),
	"FakeCEPIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeCEPIndexLength(index, length) }),

	"FakePhone":            semArgumentos(func() any { return migrate.FakePhone() }),
	"FakePhoneIndex":       umInteiro(func(index int) any { return migrate.FakePhoneIndex(index) }),
	"FakePhoneIndexLength": doisInteiros(func(index, length int) any { return migrate.FakePhoneIndexLength(index, length) }),

	"FakeUF":      semArgumentos(func() any { return migrate.FakeUF() }),
	"FakeUFIndex": umInteiro(func(index int) any { return migrate.FakeUFIndex(index) }),

	"FakeIPv4":      semArgumentos(func() any { return migrate.FakeIPv4() }),
	"FakeUserAgent": semArgumentos(func() any { return migrate.FakeUserAgent() }),

	"FakeUUID":      semArgumentos(func() any { return migrate.FakeUUID() }),
	"FakeUUIDIndex": umInteiro(func(index int) any { return migrate.FakeUUIDIndex(index) }),

	"FakeHash":            semArgumentos(func() any { return migrate.FakeHash() }),
	"FakeHashIndex":       umInteiro(func(index int) any { return migrate.FakeHashIndex(index) }),
	"FakeHashIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeHashIndexLength(index, length) }),
	"FakeHashPassword":    semArgumentos(func() any { return migrate.FakeHashPassword() }),

	"FakeUsername":            semArgumentos(func() any { return migrate.FakeUsername() }),
	"FakeUsernameIndex":       umInteiro(func(index int) any { return migrate.FakeUsernameIndex(index) }),
	"FakeUsernameIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeUsernameIndexLength(index, length) }),

	"FakeFileName":            semArgumentos(func() any { return migrate.FakeFileName() }),
	"FakeFileNameIndex":       umInteiro(func(index int) any { return migrate.FakeFileNameIndex(index) }),
	"FakeFileNameIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeFileNameIndexLength(index, length) }),

	"FakeDistrict":            semArgumentos(func() any { return migrate.FakeDistrict() }),
	"FakeDistrictIndex":       umInteiro(func(index int) any { return migrate.FakeDistrictIndex(index) }),
	"FakeDistrictIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeDistrictIndexLength(index, length) }),

	"FakeStreet":            semArgumentos(func() any { return migrate.FakeStreet() }),
	"FakeStreetIndex":       umInteiro(func(index int) any { return migrate.FakeStreetIndex(index) }),
	"FakeStreetIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeStreetIndexLength(index, length) }),

	"FakeCity":            semArgumentos(func() any { return migrate.FakeCity() }),
	"FakeCityIndex":       umInteiro(func(index int) any { return migrate.FakeCityIndex(index) }),
	"FakeCityIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeCityIndexLength(index, length) }),

	"FakeCityCode":            semArgumentos(func() any { return migrate.FakeCityCode() }),
	"FakeCityCodeIndex":       umInteiro(func(index int) any { return migrate.FakeCityCodeIndex(index) }),
	"FakeCityCodeIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeCityCodeIndexLength(index, length) }),

	"FakeState":            semArgumentos(func() any { return migrate.FakeState() }),
	"FakeStateIndex":       umInteiro(func(index int) any { return migrate.FakeStateIndex(index) }),
	"FakeStateIndexLength": doisInteiros(func(index, length int) any { return migrate.FakeStateIndexLength(index, length) }),

	"FakeName":      semArgumentos(func() any { return migrate.FakeName() }),
	"FakeNameIndex": inteiroVariadicos(func(index int, genders ...string) any { return migrate.FakeNameIndex(index, genders...) }),
	"FakeNameIndexLength": doisInteirosVariadicos(func(index, length int, genders ...string) any {
		return migrate.FakeNameIndexLength(index, length, genders...)
	}),

	"FakeDate":     semArgumentos(func() any { return migrate.FakeDate() }),
	"FakeDateTime": semArgumentos(func() any { return migrate.FakeDateTime() }),
	"FakeBytes":    umInteiro(func(length int) any { return migrate.FakeBytes(length) }),
	"FakeValue":    semArgumentos(func() any { return migrate.FakeValue() }),
}

// nomesConhecidos devolve o vocabulário em ordem, para mensagens de erro.
func nomesConhecidos() []string {
	nomes := make([]string, 0, len(vocabulario)+2)
	for nome := range vocabulario {
		nomes = append(nomes, nome)
	}
	nomes = append(nomes, "FakeLocation", "Vinculo")
	sort.Strings(nomes)
	return nomes
}

// sugestaoDeNome procura o nome conhecido mais próximo do que foi escrito.
// Erro de digitação em factory é comum e a lista inteira tem 90 entradas.
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

func semArgumentos(f func() any) chamadaFake {
	return func(args []any) (any, error) {
		if err := exigeQuantidade(args, 0); err != nil {
			return nil, err
		}
		return f(), nil
	}
}

func umInteiro(f func(int) any) chamadaFake {
	return func(args []any) (any, error) {
		if err := exigeQuantidade(args, 1); err != nil {
			return nil, err
		}
		primeiro, err := inteiroEm(args, 0)
		if err != nil {
			return nil, err
		}
		return f(primeiro), nil
	}
}

func doisInteiros(f func(int, int) any) chamadaFake {
	return func(args []any) (any, error) {
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
		return f(primeiro, segundo), nil
	}
}

func tresInteiros(f func(int, int, int) any) chamadaFake {
	return func(args []any) (any, error) {
		if err := exigeQuantidade(args, 3); err != nil {
			return nil, err
		}
		valores := make([]int, 3)
		for posicao := range valores {
			valor, err := inteiroEm(args, posicao)
			if err != nil {
				return nil, err
			}
			valores[posicao] = valor
		}
		return f(valores[0], valores[1], valores[2]), nil
	}
}

func inteiroTextoInteiro(f func(int, string, int) any) chamadaFake {
	return func(args []any) (any, error) {
		if err := exigeQuantidade(args, 3); err != nil {
			return nil, err
		}
		primeiro, err := inteiroEm(args, 0)
		if err != nil {
			return nil, err
		}
		texto, err := textoEm(args, 1)
		if err != nil {
			return nil, err
		}
		terceiro, err := inteiroEm(args, 2)
		if err != nil {
			return nil, err
		}
		return f(primeiro, texto, terceiro), nil
	}
}

func variadicos(f func(...string) any) chamadaFake {
	return func(args []any) (any, error) {
		textos, err := textosDe(args, 0)
		if err != nil {
			return nil, err
		}
		return f(textos...), nil
	}
}

func inteiroVariadicos(f func(int, ...string) any) chamadaFake {
	return func(args []any) (any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("exige ao menos o índice")
		}
		primeiro, err := inteiroEm(args, 0)
		if err != nil {
			return nil, err
		}
		textos, err := textosDe(args, 1)
		if err != nil {
			return nil, err
		}
		return f(primeiro, textos...), nil
	}
}

func doisInteirosVariadicos(f func(int, int, ...string) any) chamadaFake {
	return func(args []any) (any, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("exige ao menos índice e tamanho")
		}
		primeiro, err := inteiroEm(args, 0)
		if err != nil {
			return nil, err
		}
		segundo, err := inteiroEm(args, 1)
		if err != nil {
			return nil, err
		}
		textos, err := textosDe(args, 2)
		if err != nil {
			return nil, err
		}
		return f(primeiro, segundo, textos...), nil
	}
}

func exigeQuantidade(args []any, esperado int) error {
	if len(args) != esperado {
		return fmt.Errorf("exige %d argumento(s), recebeu %d", esperado, len(args))
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
	return 0, fmt.Errorf("o argumento %d precisa ser um número inteiro, veio %T", posicao+1, args[posicao])
}

func textoEm(args []any, posicao int) (string, error) {
	if texto, ok := args[posicao].(string); ok {
		return texto, nil
	}
	return "", fmt.Errorf("o argumento %d precisa ser um texto entre aspas, veio %T", posicao+1, args[posicao])
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
