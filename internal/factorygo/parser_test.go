package factorygo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	migrate "github.com/PhelipeViana/gokit/migration"
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

const cabecalho = "package factories\n\nimport migrate \"github.com/PhelipeViana/gokit/migration\"\n\n"

func factoria(campos string) string {
	return cabecalho + `
func RacasFactory() migrate.Factory {
	return migrate.Factory{
		Table: "RACAS",
		Ruler: migrate.Ruler{Count: 3, Active: true},
		Data: migrate.Fields{
			` + campos + `
		},
	}
}
`
}

func TestLeFactoryCompleta(t *testing.T) {
	caminho := escreve(t, factoria(`"RACA_ID": migrate.FakeInt(1, 100),
				"NOME":    migrate.FakeUniqueText("Raca", 20),`))

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
	caminho := escreve(t, factoria(`"NOME": migrate.FakeName(60),
			"UF":   migrate.FakeUF(),`))

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
	caminho := escreve(t, factoria(`"NOME":      migrate.FakeUniqueText("Raca", 20),
				"CIDADE_ID": migrate.Reference("CIDADES", "CIDADE_ID"),`))

	arquivo, err := ParseArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	if referencias := arquivo.Referencias(); len(referencias) != 1 || referencias[0] != "CIDADES" {
		t.Fatalf("referências vieram %v", referencias)
	}
	// O executor resolve o vínculo contra as linhas já inseridas no pai, então
	// ele não pode aparecer com valor gerado.
	linha, _ := arquivo.Linha(0)
	if _, presente := linha["CIDADE_ID"]; presente {
		t.Fatal("a coluna de vínculo não deveria vir preenchida da factory")
	}
}

func TestNomeForaDoVocabularioSugereOCerto(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": migrate.FakeUniqeText("Raca", 20),`))

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
	caminho := escreve(t, factoria(`"NOME": migrate.FakeUniqueText(5, 20),`))

	_, err := ParseArquivo(caminho)
	if err == nil {
		t.Fatal("texto esperado no lugar de número deveria falhar")
	}
	if !strings.Contains(err.Error(), "texto entre aspas") {
		t.Fatalf("erro pouco claro: %v", err)
	}
}

func TestQuantidadeDeArgumentosErrada(t *testing.T) {
	caminho := escreve(t, factoria(`"NOME": migrate.FakeInt(1),`))

	_, err := ParseArquivo(caminho)
	if err == nil || !strings.Contains(err.Error(), "2 argumento") {
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

// O acessor de localidade (`local("uf", 2)`) e o FakeLocation que o produzia
// saíram junto com a closure. Escrever qualquer um deve falhar com a sugestão do
// substituto, não com um erro de gramática.
func TestAcessorDeLocalidadeNaoExisteMais(t *testing.T) {
	for _, escrito := range []string{`"UF": local("uf", 2),`, `"UF": migrate.FakeLocation(),`} {
		caminho := escreve(t, factoria(escrito))
		_, err := ParseArquivo(caminho)
		if err == nil {
			t.Fatalf("%s deveria ser recusado", escrito)
		}
		if !strings.Contains(err.Error(), "vocabulário") && !strings.Contains(err.Error(), "não reconhecida") {
			t.Fatalf("%s: erro pouco claro: %v", escrito, err)
		}
	}
}

// A coerência que o acessor garantia — cidade, UF e estado da mesma localidade na
// mesma linha — precisa sobreviver às funções independentes, que é o que justifica
// ter tirado o acessor.
func TestLocalidadeContinuaCoerenteSemAcessor(t *testing.T) {
	caminho := escreve(t, factoria(`"CIDADE": migrate.FakeCity(60),
			"UF":     migrate.FakeUF(),
			"ESTADO": migrate.FakeState(60),`))

	arquivo, err := ParseArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 5; index++ {
		linha, err := arquivo.Linha(index)
		if err != nil {
			t.Fatal(err)
		}
		esperado := migrate.FakeUFIndex(index)
		if linha["UF"] != esperado {
			t.Fatalf("índice %d: UF veio %v, esperava %v", index, linha["UF"], esperado)
		}
		if linha["CIDADE"] != migrate.FakeCityIndex(index) || linha["ESTADO"] != migrate.FakeStateIndexLength(index, 60) {
			t.Fatalf("índice %d: localidade incoerente: %v", index, linha)
		}
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
	caminho := escreve(t, factoria(`"NOME": migrate.FakeUniqueText("Raca", 20),`))

	arquivo, err := ParseArquivo(caminho)
	if err != nil {
		t.Fatal(err)
	}
	if arquivo.Campos[0].Origem != `migrate.FakeUniqueText("Raca", 20)` {
		t.Fatalf("origem veio %q", arquivo.Campos[0].Origem)
	}
}

// A closure aceitava um statement antes do return e por isso precisava recusar os
// demais. Com Data virando mapa, não existe lugar para statement nenhum: escrever
// a forma antiga tem de falhar dizendo qual é a forma certa.
func TestClosureNoDataERecusada(t *testing.T) {
	corpo := cabecalho + `
func RacasFactory() migrate.Factory {
	return migrate.Factory{
		Table: "RACAS",
		Ruler: migrate.Ruler{Count: 3, Active: true},
		Data: func(index int) migrate.Fields {
			return migrate.Fields{
				"NOME": migrate.FakeInt(1, 2),
			}
		},
	}
}
`
	caminho := escreve(t, corpo)
	_, err := ParseArquivo(caminho)
	if err == nil || !strings.Contains(err.Error(), "migrate.Fields{...}") {
		t.Fatalf("deveria apontar a forma certa de Data, veio: %v", err)
	}
}

// Um arquivo pode declarar várias factories: não há regra implícita que justifique
// fatiar por tabela, então a organização é escolha de quem escreve.
func TestVariasFactoriesNoMesmoArquivo(t *testing.T) {
	corpo := cabecalho + `
func RacasFactory() migrate.Factory {
	return migrate.Factory{
		Table: "RACAS",
		Ruler: migrate.Ruler{Count: 3, Active: true},
		Data: migrate.Fields{
			"NOME": migrate.FakeName(60),
		},
	}
}

// O Active é por FUNÇÃO, mesmo morando no mesmo arquivo.
func CoresFactory() migrate.Factory {
	return migrate.Factory{
		Table: "CORES",
		Ruler: migrate.Ruler{Count: 7, Active: false},
		Data: migrate.Fields{
			"TOM": migrate.FakeName(30),
		},
	}
}
`
	caminho := filepath.Join(t.TempDir(), "factories.go")
	if err := os.WriteFile(caminho, []byte(corpo), 0o644); err != nil {
		t.Fatal(err)
	}

	arquivos, err := ParseArquivos(caminho)
	if err != nil {
		t.Fatal(err)
	}
	if len(arquivos) != 2 {
		t.Fatalf("esperava 2 factories, obteve %d", len(arquivos))
	}
	if arquivos[0].Tabela != "RACAS" || arquivos[1].Tabela != "CORES" {
		t.Fatalf("tabelas erradas: %s, %s", arquivos[0].Tabela, arquivos[1].Tabela)
	}
	if !arquivos[0].Ruler.Active || arquivos[1].Ruler.Active {
		t.Fatal("o Active de cada função deveria ser independente")
	}
	if arquivos[1].Ruler.Count != 7 {
		t.Fatalf("o Count da segunda deveria ser 7, obteve %d", arquivos[1].Ruler.Count)
	}
}

// Um helper na mesma pasta não pode virar erro de factory.
func TestArquivoSemFactoryEIgnorado(t *testing.T) {
	pasta := t.TempDir()
	if err := os.WriteFile(filepath.Join(pasta, "ajuda.go"), []byte("package factories\n\nfunc Ajuda() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	arquivos, err := CarregarPasta(pasta)
	if err != nil {
		t.Fatalf("não deveria falhar: %v", err)
	}
	if len(arquivos) != 0 {
		t.Fatalf("esperava nenhuma factory, obteve %d", len(arquivos))
	}
}
