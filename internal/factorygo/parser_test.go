package factorygo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// escreve grava uma factory temporária e devolve o caminho.
func escreve(t *testing.T, corpo string) string {
	t.Helper()
	caminho := filepath.Join(t.TempDir(), "racas_factory.go")
	if err := os.WriteFile(caminho, []byte(corpo), 0o644); err != nil {
		t.Fatal(err)
	}
	return caminho
}

const cabecalho = "package factories\n\nimport migrate \"gokit/migration\"\n\n"

func factoria(campos string, antes ...string) string {
	return cabecalho + `
func RacasFactory() migrate.Factory {
	return migrate.Factory{
		Table: "RACAS",
		Ruler: migrate.Ruler{Count: 3, Update: true, Active: true},
		Data: func(index int) migrate.Fields {
			` + strings.Join(antes, "\n\t\t\t") + `
			return migrate.Fields{
				` + campos + `
			}
		},
	}
}
`
}

func TestLeFactoryCompleta(t *testing.T) {
	caminho := escreve(t, factoria(`"RACA_ID": migrate.FakeIntIndex(index, 1, 100),
				"NOME":    migrate.FakeUniqueText(index, "Raca", 20),`))

	arquivo, err := ParseArquivo(caminho)
	if err != nil {
		t.Fatalf("não deveria falhar: %v", err)
	}
	if arquivo.Tabela != "RACAS" {
		t.Fatalf("tabela veio %q", arquivo.Tabela)
	}
	if arquivo.Ruler.Count != 3 || !arquivo.Ruler.Active {
		t.Fatalf("ruler veio %+v", arquivo.Ruler)
	}

	linha, err := arquivo.Linha(2)
	if err != nil {
		t.Fatal(err)
	}
	if linha["RACA_ID"] != 3 {
		t.Fatalf("RACA_ID na linha de índice 2 deveria ser 3, veio %v", linha["RACA_ID"])
	}
	if linha["NOME"] != "Raca 3" {
		t.Fatalf("NOME veio %q", linha["NOME"])
	}
}

// A ordem das colunas vira a ordem do INSERT: precisa ser a do arquivo.
func TestOrdemDasColunasEPreservada(t *testing.T) {
	caminho := escreve(t, factoria(`"ZZZ": migrate.FakeInt(1, 2),
				"AAA": migrate.FakeInt(1, 2),
				"MMM": migrate.FakeInt(1, 2),`))

	arquivo, err := ParseArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	esperado := []string{"ZZZ", "AAA", "MMM"}
	for posicao, nome := range arquivo.Colunas() {
		if nome != esperado[posicao] {
			t.Fatalf("ordem veio %v, esperava %v", arquivo.Colunas(), esperado)
		}
	}
}

// O mesmo índice tem de gerar o mesmo dado, senão não há como comparar o
// resultado entre os quatro bancos.
func TestGeracaoEDeterministica(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": migrate.FakeNameIndexLength(index, 60),
				"UF":   local(index, "uf", 2),`, `local := migrate.FakeLocation()`))

	arquivo, err := ParseArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	primeira, _ := arquivo.Linha(7)
	segunda, _ := arquivo.Linha(7)
	if primeira["NOME"] != segunda["NOME"] || primeira["UF"] != segunda["UF"] {
		t.Fatalf("mesma linha gerou valores diferentes: %v vs %v", primeira, segunda)
	}
	if primeira["UF"] == "" {
		t.Fatal("o acessor de localidade não produziu valor")
	}
}

func TestVinculoNaoGeraValor(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME":      migrate.FakeUniqueText(index, "Raca", 20),
				"CIDADE_ID": migrate.Vinculo("CIDADES", "CIDADE_ID"),`))

	arquivo, err := ParseArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	if vinculos := arquivo.Vinculos(); len(vinculos) != 1 || vinculos[0] != "CIDADES" {
		t.Fatalf("vínculos vieram %v", vinculos)
	}
	// O executor resolve o vínculo contra as linhas já inseridas no pai, então
	// ele não pode aparecer com valor gerado.
	linha, _ := arquivo.Linha(0)
	if _, presente := linha["CIDADE_ID"]; presente {
		t.Fatal("a coluna de vínculo não deveria vir preenchida da factory")
	}
}

func TestNomeForaDoVocabularioSugereOCerto(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": migrate.FakeUniqeText(index, "Raca", 20),`))

	_, err := ParseArquivo(caminho)
	if err == nil {
		t.Fatal("um nome inexistente deveria falhar na leitura")
	}
	if !strings.Contains(err.Error(), "FakeUniqueText") {
		t.Fatalf("o erro deveria sugerir FakeUniqueText, veio: %v", err)
	}
}

// O erro precisa aparecer ao ler o arquivo, não no meio de um lote de INSERT.
func TestArgumentoDeTipoErradoFalhaNaLeitura(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": migrate.FakeUniqueText(index, 5, 20),`))

	_, err := ParseArquivo(caminho)
	if err == nil {
		t.Fatal("texto esperado no lugar de número deveria falhar")
	}
	if !strings.Contains(err.Error(), "texto entre aspas") {
		t.Fatalf("erro pouco claro: %v", err)
	}
}

func TestQuantidadeDeArgumentosErrada(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": migrate.FakeIntIndex(index, 1),`))

	_, err := ParseArquivo(caminho)
	if err == nil || !strings.Contains(err.Error(), "3 argumento") {
		t.Fatalf("esperava erro de aridade, veio: %v", err)
	}
}

// Expressão arbitrária é justamente o que o AST não sabe avaliar: precisa ser
// recusada com clareza, não aceita e ignorada.
func TestExpressaoArbitrariaERecusada(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": strings.ToUpper("x"),`))

	_, err := ParseArquivo(caminho)
	if err == nil {
		t.Fatal("chamada fora do vocabulário deveria falhar")
	}
}

func TestAcessorNaoDeclaradoERecusado(t *testing.T) {
	caminho := escreve(t, factoria(`"UF": local(index, "uf", 2),`))

	_, err := ParseArquivo(caminho)
	if err == nil || !strings.Contains(err.Error(), "FakeLocation") {
		t.Fatalf("deveria orientar a declarar o acessor, veio: %v", err)
	}
}

func TestFakeLocationSoltaOrientaOUso(t *testing.T) {
	caminho := escreve(t, factoria(`"UF": migrate.FakeLocation(),`))

	_, err := ParseArquivo(caminho)
	if err == nil || !strings.Contains(err.Error(), "local :=") {
		t.Fatalf("deveria explicar como usar FakeLocation, veio: %v", err)
	}
}

func TestColunaRepetidaERecusada(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": migrate.FakeInt(1, 2),
				"NOME": migrate.FakeInt(3, 4),`))

	_, err := ParseArquivo(caminho)
	if err == nil || !strings.Contains(err.Error(), "duas vezes") {
		t.Fatalf("esperava erro de coluna repetida, veio: %v", err)
	}
}

func TestLiteraisSaoAceitos(t *testing.T) {
	caminho := escreve(t, factoria(`"A": nil,
				"B": 42,
				"C": "texto",
				"D": true,
				"E": -7,`))

	arquivo, err := ParseArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	linha, err := arquivo.Linha(0)
	if err != nil {
		t.Fatal(err)
	}
	if linha["A"] != nil || linha["B"] != int64(42) || linha["C"] != "texto" || linha["D"] != true || linha["E"] != int64(-7) {
		t.Fatalf("literais vieram %v", linha)
	}
}

// Origem é o que permite ao gerador reescrever o arquivo sem perder ajustes.
func TestOrigemGuardaAExpressaoOriginal(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": migrate.FakeUniqueText(index, "Raca", 20),`))

	arquivo, err := ParseArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	if arquivo.Campos[0].Origem != `migrate.FakeUniqueText(index, "Raca", 20)` {
		t.Fatalf("origem veio %q", arquivo.Campos[0].Origem)
	}
}

func TestStatementInesperadoDentroDeData(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": migrate.FakeInt(1, 2),`, `for i := 0; i < 3; i++ {}`))

	_, err := ParseArquivo(caminho)
	if err == nil || !strings.Contains(err.Error(), "FakeLocation") {
		t.Fatalf("deveria recusar statement arbitrário, veio: %v", err)
	}
}
