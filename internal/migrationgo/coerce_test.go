package migrationgo

import (
	"testing"
	"time"

	"gokit/migration/acao"
)

func colunas() []acao.ColunaDefinicao {
	return []acao.ColunaDefinicao{
		{Name: "id", Type: "integer"},
		{Name: "codigo", Type: "int"},
		{Name: "nome", Type: "string"},
		{Name: "ativo", Type: "char"},
		{Name: "criado_em", Type: "timestamp"},
		{Name: "nascimento", Type: "date"},
		{Name: "valor", Type: "decimal"},
	}
}

func TestCoerceDataVemComoTextoEViraTime(t *testing.T) {
	linhas := []acao.Linha{{"criado_em": "2026-07-01 18:06:57"}}
	if err := coerceSeedRows(linhas, colunas()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	momento, ok := linhas[0]["criado_em"].(time.Time)
	if !ok {
		t.Fatalf("esperava time.Time, veio %T", linhas[0]["criado_em"])
	}
	if got := momento.Format("2006-01-02 15:04:05"); got != "2026-07-01 18:06:57" {
		t.Fatalf("data errada: %s", got)
	}
}

func TestCoerceAceitaOsFormatosDeDumpMaisComuns(t *testing.T) {
	casos := map[string]string{
		"2026-07-01 18:06:57":        "2026-07-01 18:06:57",
		"2026-07-01T18:06:57":        "2026-07-01 18:06:57",
		"2026-07-01 18:06:57.123456": "2026-07-01 18:06:57",
		"2026-07-01":                 "2026-07-01 00:00:00",
	}
	for entrada, esperado := range casos {
		linhas := []acao.Linha{{"criado_em": entrada}}
		if err := coerceSeedRows(linhas, colunas()); err != nil {
			t.Fatalf("%q: %v", entrada, err)
		}
		got := linhas[0]["criado_em"].(time.Time).Format("2006-01-02 15:04:05")
		if got != esperado {
			t.Fatalf("%q virou %q, esperava %q", entrada, got, esperado)
		}
	}
}

func TestCoerceDataInvalidaFalhaComOrientacao(t *testing.T) {
	linhas := []acao.Linha{{"criado_em": "01/07/2026"}}
	err := coerceSeedRows(linhas, colunas())
	if err == nil {
		t.Fatal("esperava erro para data em formato brasileiro")
	}
	if want := "2006-01-02 15:04:05"; !contains(err.Error(), want) {
		t.Fatalf("erro deveria ensinar o formato, veio: %v", err)
	}
}

// O pgx recusa número em coluna de texto; os outros três convertem sozinhos.
func TestCoerceNumeroEmColunaDeTextoViraTexto(t *testing.T) {
	linhas := []acao.Linha{{"nome": int64(6), "ativo": int64(1), "valor": float64(1.5)}}
	if err := coerceSeedRows(linhas, colunas()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if linhas[0]["nome"] != "6" {
		t.Fatalf("nome: esperava \"6\", veio %#v", linhas[0]["nome"])
	}
	if linhas[0]["ativo"] != "1" {
		t.Fatalf("ativo: esperava \"1\", veio %#v", linhas[0]["ativo"])
	}
	if linhas[0]["valor"] != float64(1.5) {
		t.Fatalf("valor decimal não deveria mudar, veio %#v", linhas[0]["valor"])
	}
}

func TestCoerceTextoNumericoEmColunaNumericaViraNumero(t *testing.T) {
	linhas := []acao.Linha{{"id": "42", "codigo": "7"}}
	if err := coerceSeedRows(linhas, colunas()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if linhas[0]["id"] != int64(42) {
		t.Fatalf("id: esperava 42, veio %#v", linhas[0]["id"])
	}
	if linhas[0]["codigo"] != int64(7) {
		t.Fatalf("codigo: esperava 7, veio %#v", linhas[0]["codigo"])
	}
}

func TestCoercePreservaNilEValorJaTipado(t *testing.T) {
	momento := time.Date(2026, 7, 1, 18, 6, 57, 0, time.UTC)
	linhas := []acao.Linha{{"nome": nil, "criado_em": momento, "id": int64(1)}}
	if err := coerceSeedRows(linhas, colunas()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if linhas[0]["nome"] != nil {
		t.Fatalf("nil deveria continuar nil, veio %#v", linhas[0]["nome"])
	}
	if !linhas[0]["criado_em"].(time.Time).Equal(momento) {
		t.Fatal("time.Time já tipado não deveria mudar")
	}
	if linhas[0]["id"] != int64(1) {
		t.Fatalf("id deveria continuar 1, veio %#v", linhas[0]["id"])
	}
}

// Coluna que não está no CreateTable não tem tipo conhecido: o valor passa
// intacto e quem barra é a validação de coluna inexistente.
func TestCoerceIgnoraColunaDesconhecida(t *testing.T) {
	linhas := []acao.Linha{{"inexistente": "abc"}}
	if err := coerceSeedRows(linhas, colunas()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if linhas[0]["inexistente"] != "abc" {
		t.Fatalf("valor deveria passar intacto, veio %#v", linhas[0]["inexistente"])
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
