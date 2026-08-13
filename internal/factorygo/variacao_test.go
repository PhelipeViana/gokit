package factorygo

import (
	"fmt"
	"testing"
)

// argumentosDeTeste é uma chamada plausível de cada nome do vocabulário, escrita
// como a factory escreveria — sem índice. Serve de lista de presença: nome novo no
// vocabulário sem entrada aqui faz o teste falhar, e é isso que garante que
// ninguém acrescente um gerador constante por descuido.
var argumentosDeTeste = map[string][]any{
	"FakeChoice":       {"A", "B", "C"},
	"FakeInt":          {int64(1), int64(999)},
	"FakeDecimal":      {int64(10), int64(2)},
	"FakeString":       {int64(60)},
	"FakeText":         {int64(200)},
	"FakeBytes":        {int64(32)},
	"FakeValue":        {},
	"FakeUniqueText":   {"Item", int64(60)},
	"FakeCode":         {int64(30)},
	"FakeCodePrefix":   {"PRE", int64(30)},
	"FakeMatricula":    {},
	"FakeUniqueCPF":    {int64(14)},
	"FakeUniqueCNPJ":   {int64(18)},
	"FakeCPF":          {int64(14)},
	"FakeCNPJ":         {int64(18)},
	"FakeName":         {int64(60)},
	"FakeUsername":     {int64(40)},
	"FakeEmail":        {int64(80)},
	"FakePhone":        {int64(20)},
	"FakeCEP":          {int64(9)},
	"FakeUF":           {},
	"FakeDistrict":     {int64(60)},
	"FakeStreet":       {int64(60)},
	"FakeCity":         {int64(60)},
	"FakeCityCode":     {int64(10)},
	"FakeState":        {int64(60)},
	"FakeCountry":      {int64(60)},
	"FakeDate":         {},
	"FakeDateTime":     {},
	"FakeUUID":         {},
	"FakeHash":         {int64(64)},
	"FakeFileName":     {int64(60)},
	"FakeIPv4":         {},
	"FakeUserAgent":    {},
	"FakeHashPassword": {},
}

// constantesPorMerito são os nomes que NÃO variam, com o motivo de cada um.
// Nenhum deles cai em coluna única.
//
//	FakeUserAgent    user-agent repetido em dez linhas é dado plausível
//	FakeHashPassword hash de senha idêntico é o normal em ambiente de teste
//	FakeValue        é o nulo
//	FakeCountry      constante pelos DADOS, não por decisão: a tabela de
//	                 localidades de fake.go é toda brasileira. Acrescentar
//	                 localidade de outro país faz esta entrada sair daqui.
var constantesPorMerito = map[string]bool{
	"FakeUserAgent":    true,
	"FakeHashPassword": true,
	"FakeValue":        true,
	"FakeCountry":      true,
}

// O motivo de o índice ter saído da assinatura foi este: antes, metade dos nomes
// devolvia constante e dez linhas saíam idênticas sem avisar. Agora todo nome do
// vocabulário varia por linha, e este teste é o que impede a regressão.
func TestTodoNomeDoVocabularioVariaPorLinha(t *testing.T) {
	for nome, funcao := range vocabulario {
		args, listado := argumentosDeTeste[nome]
		if !listado {
			t.Errorf("%s está no vocabulário mas não tem chamada de teste: acrescente em argumentosDeTeste", nome)
			continue
		}

		distintos := map[string]bool{}
		for index := 0; index < 10; index++ {
			valor, err := funcao(index, args)
			if err != nil {
				t.Errorf("%s: %v", nome, err)
				break
			}
			distintos[fmt.Sprintf("%v", valor)] = true
		}

		if constantesPorMerito[nome] {
			if len(distintos) != 1 {
				t.Errorf("%s deveria ser constante, veio %d valores", nome, len(distintos))
			}
			continue
		}
		if len(distintos) < 2 {
			t.Errorf("%s devolveu o mesmo valor nas 10 linhas: %v", nome, distintos)
		}
	}
}

// A recíproca: chamada de teste para nome que não está no vocabulário significa
// que o nome foi removido e a lista ficou para trás.
func TestNaoSobraChamadaDeTesteOrfa(t *testing.T) {
	for nome := range argumentosDeTeste {
		if _, existe := vocabulario[nome]; !existe {
			t.Errorf("%s tem chamada de teste mas não está no vocabulário", nome)
		}
	}
}
