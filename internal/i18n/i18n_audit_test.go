package i18n

import (
	"regexp"
	"strings"
	"testing"
)

// Com o catálogo passando de 400 chaves espalhadas por arquivo de domínio, o
// erro que escapa não é o de compilação — é a chave sem um dos idiomas e a
// tradução que perdeu um %s. Os dois quebram só na hora em que o usuário lê a
// mensagem, então ficam guardados aqui.

func TestTodaChaveTemOsTresIdiomas(t *testing.T) {
	for _, chave := range Keys() {
		idiomas := translations[chave]
		for _, idioma := range []Language{PT, ES, EN} {
			if strings.TrimSpace(idiomas[idioma]) == "" {
				t.Errorf("chave %q sem tradução em %s", chave, idioma)
			}
		}
	}
}

// verbo captura os marcadores de formato do fmt, inclusive largura e sinal
// (%-42s, %4d, %.2f), porque a tradução precisa manter o mesmo tipo na mesma
// ordem — trocar %s por %d faz o fmt imprimir %!d(string=...).
// O %% vem primeiro na alternância para ser consumido antes: sem isso, um
// "%%s" seria lido como o verbo %s a partir do segundo caractere.
var verbo = regexp.MustCompile(`%%|%[-+# 0-9.*]*[a-zA-Z]`)

// tipoDoVerbo reduz o marcador ao que importa para a comparação: o tipo. A
// largura pode mudar de idioma para idioma (uma coluna alinhada em português
// pode precisar de mais espaço em inglês), o tipo não.
func tipoDoVerbo(marca string) string {
	return marca[len(marca)-1:]
}

func TestVerbosDeFormatoBatemEntreIdiomas(t *testing.T) {
	for _, chave := range Keys() {
		idiomas := translations[chave]
		esperado := tiposDe(idiomas[PT])
		for _, idioma := range []Language{ES, EN} {
			obtido := tiposDe(idiomas[idioma])
			if strings.Join(obtido, "") != strings.Join(esperado, "") {
				t.Errorf("chave %q: verbos %v em PT, %v em %s", chave, esperado, obtido, idioma)
			}
		}
	}
}

func tiposDe(frase string) []string {
	var tipos []string
	for _, marca := range verbo.FindAllString(frase, -1) {
		// %% é um literal de porcentagem, não um argumento.
		if marca == "%%" {
			continue
		}
		tipos = append(tipos, tipoDoVerbo(marca))
	}
	return tipos
}
