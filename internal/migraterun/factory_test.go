package migraterun

import (
	"os"
	"strings"
	"testing"

	"gokit/internal/config"
	"gokit/internal/factorygo"
	migrate "gokit/migration"
	"gokit/migration/acao"
)

func planoDe(tabela string, pais ...string) plano {
	return plano{
		Arquivo: factorygo.Arquivo{Tabela: tabela},
		Forma:   acao.Operacao{Table: strings.ToLower(tabela)},
		Conhece: true,
		Pais:    pais,
	}
}

func nomes(planos []plano) []string {
	lista := make([]string, len(planos))
	for posicao, atual := range planos {
		lista[posicao] = atual.Tabela()
	}
	return lista
}

func posicaoDe(lista []string, alvo string) int {
	for posicao, nome := range lista {
		if nome == alvo {
			return posicao
		}
	}
	return -1
}

func TestOrdenaPaiAntesDeFilha(t *testing.T) {
	ordenados, ciclos := ordenaFactories([]plano{
		planoDe("PEDIDOS", "CLIENTES"),
		planoDe("ITENS", "PEDIDOS", "PRODUTOS"),
		planoDe("CLIENTES"),
		planoDe("PRODUTOS"),
	})
	if len(ciclos) != 0 {
		t.Fatalf("não há ciclo aqui, veio %v", ciclos)
	}
	ordem := nomes(ordenados)
	for _, par := range [][2]string{{"CLIENTES", "PEDIDOS"}, {"PEDIDOS", "ITENS"}, {"PRODUTOS", "ITENS"}} {
		if posicaoDe(ordem, par[0]) > posicaoDe(ordem, par[1]) {
			t.Fatalf("%s deveria vir antes de %s; ordem: %v", par[0], par[1], ordem)
		}
	}
}

// Schema legado tem ciclo de verdade. Romper e seguir é melhor que abortar: a
// coluna do ciclo aceita nulo, então o INSERT passa.
func TestCicloERompidoSemPerderTabela(t *testing.T) {
	ordenados, ciclos := ordenaFactories([]plano{
		planoDe("A", "B"),
		planoDe("B", "A"),
		planoDe("C"),
	})
	if len(ciclos) == 0 {
		t.Fatal("o ciclo deveria ser reportado")
	}
	if len(ordenados) != 3 {
		t.Fatalf("nenhuma tabela pode se perder no ciclo, vieram %v", nomes(ordenados))
	}
}

// Autorreferência não é dependência de ordem.
func TestAutorreferenciaNaoViraCiclo(t *testing.T) {
	_, ciclos := ordenaFactories([]plano{planoDe("CATEGORIAS", "CATEGORIAS")})
	if len(ciclos) != 0 {
		t.Fatalf("uma tabela que aponta para si mesma não é ciclo, veio %v", ciclos)
	}
}

func TestOrdemEEstavelEntreExecucoes(t *testing.T) {
	entrada := []plano{planoDe("D"), planoDe("A"), planoDe("C"), planoDe("B")}
	primeira := nomes(mustOrdenar(t, entrada))
	for tentativa := 0; tentativa < 5; tentativa++ {
		if atual := nomes(mustOrdenar(t, entrada)); strings.Join(atual, ",") != strings.Join(primeira, ",") {
			t.Fatalf("ordem instável: %v vs %v", atual, primeira)
		}
	}
}

func mustOrdenar(t *testing.T, planos []plano) []plano {
	t.Helper()
	ordenados, _ := ordenaFactories(planos)
	return ordenados
}

// Pedir uma tabela precisa trazer os pais (senão a FK barra o INSERT) e as
// filhas (senão a FK barra o DELETE que limpa a tabela).
func TestSelecaoTrazPaisEFilhas(t *testing.T) {
	planos := []plano{
		planoDe("CLIENTES"),
		planoDe("PEDIDOS", "CLIENTES"),
		planoDe("ITENS", "PEDIDOS"),
		planoDe("AVULSA"),
	}
	selecionados, err := selecionaFactories(planos, []string{"pedidos"})
	if err != nil {
		t.Fatal(err)
	}
	escolhidos := nomes(selecionados)
	for _, esperado := range []string{"CLIENTES", "PEDIDOS", "ITENS"} {
		if posicaoDe(escolhidos, esperado) < 0 {
			t.Fatalf("%s deveria entrar na seleção, veio %v", esperado, escolhidos)
		}
	}
	if posicaoDe(escolhidos, "AVULSA") >= 0 {
		t.Fatalf("AVULSA não tem relação com PEDIDOS, veio %v", escolhidos)
	}
}

func TestSelecaoSemAlvoPegaSoAsAtivas(t *testing.T) {
	ativa := planoDe("A")
	ativa.Arquivo.Ruler = migrate.Ruler{Count: 1, Active: true}
	inativa := planoDe("B")
	inativa.Arquivo.Ruler = migrate.Ruler{Count: 1, Active: false}

	selecionados, err := selecionaFactories([]plano{ativa, inativa}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(selecionados) != 1 || selecionados[0].Tabela() != "A" {
		t.Fatalf("só a ativa deveria entrar, veio %v", nomes(selecionados))
	}
}

func TestTabelaSemFactoryOrientaACriar(t *testing.T) {
	_, err := selecionaFactories([]plano{planoDe("A")}, []string{"inexistente"})
	if err == nil || !strings.Contains(err.Error(), "factory create") {
		t.Fatalf("o erro deveria orientar a criar a factory, veio: %v", err)
	}
}

func TestValoresDoCheck(t *testing.T) {
	casos := []struct {
		expressao string
		coluna    string
		valores   []string
	}{
		{"status IN ('A', 'B', 'C')", "status", []string{"A", "B", "C"}},
		{"TIPO IN (1,2,3)", "TIPO", []string{"1", "2", "3"}},
		{"valor > 0", "", nil},
		{"sexo IN ('M','F')", "sexo", []string{"M", "F"}},
	}
	for _, caso := range casos {
		coluna, valores := valoresDoCheck(caso.expressao)
		if coluna != caso.coluna {
			t.Fatalf("%q: coluna veio %q, esperava %q", caso.expressao, coluna, caso.coluna)
		}
		if strings.Join(valores, ",") != strings.Join(caso.valores, ",") {
			t.Fatalf("%q: valores vieram %v, esperava %v", caso.expressao, valores, caso.valores)
		}
	}
}

// Faixa numérica contígua vira FakeInt; o resto percorre os valores um a um.
func TestExpressaoDeDominio(t *testing.T) {
	if got := expressaoDeDominio([]string{"1", "2", "3"}); got != "migrate.FakeInt(1, 3)" {
		t.Fatalf("faixa contígua veio %q", got)
	}
	if got := expressaoDeDominio([]string{"1", "5", "9"}); !strings.HasPrefix(got, "migrate.FakeChoiceIndex(index,") {
		t.Fatalf("faixa esparsa veio %q", got)
	}
	if got := expressaoDeDominio([]string{"S", "N"}); got != `migrate.FakeChoiceIndex(index, "S", "N")` {
		t.Fatalf("texto veio %q", got)
	}
}

func TestExpressaoParaColuna(t *testing.T) {
	vazio := config.ConfigState{}
	casos := []struct {
		nome    string
		coluna  acao.ColunaDefinicao
		contem  string
		semChar string
	}{
		{"chave de texto é única por índice",
			acao.ColunaDefinicao{Name: "codigo", Type: "string", Length: 10, PrimaryKey: true}, "FakeCodeIndex", ""},
		{"chave numérica é única por índice",
			acao.ColunaDefinicao{Name: "id", Type: "integer", Precision: 9, PrimaryKey: true}, "FakeIntIndex", ""},
		{"chave estrangeira vira vínculo",
			acao.ColunaDefinicao{Name: "cidade_id", Type: "integer", ReferenceTable: "cidades", ReferenceColumn: "cidade_id"}, "Vinculo", ""},
		{"data usa o tipo, não o nome",
			acao.ColunaDefinicao{Name: "criado_em", Type: "date"}, "FakeDate()", ""},
		{"cnpj é detectado pelo nome",
			acao.ColunaDefinicao{Name: "cnpj", Type: "string", Length: 20}, "FakeUniqueCNPJLength", ""},
		{"decimal respeita a escala",
			acao.ColunaDefinicao{Name: "valor", Type: "decimal", Precision: 10, Scale: 2}, "FakeDecimal(10, 2)", ""},
		{"coluna de um caractere vira flag",
			acao.ColunaDefinicao{Name: "ativo", Type: "char", Length: 1}, "FakeChoiceIndex", ""},
		{"sexo não vira flag S/N",
			acao.ColunaDefinicao{Name: "sexo", Type: "char", Length: 1}, `"M", "F"`, ""},
		{"texto genérico leva o tamanho da coluna",
			acao.ColunaDefinicao{Name: "observacao", Type: "string", Length: 80}, "80", ""},
	}
	for _, caso := range casos {
		got := expressaoParaColuna("TESTE", caso.coluna, nil, vazio)
		if !strings.Contains(got, caso.contem) {
			t.Fatalf("%s: veio %q, esperava conter %q", caso.nome, got, caso.contem)
		}
	}
}

// CHECK vence a heurística: gerar um valor que a própria migration proíbe
// derrubaria o INSERT.
func TestCheckVenceAHeuristica(t *testing.T) {
	coluna := acao.ColunaDefinicao{Name: "status", Type: "string", Length: 20}
	got := expressaoParaColuna("TESTE", coluna, []string{"ABERTO", "FECHADO"}, config.ConfigState{})
	if !strings.Contains(got, "ABERTO") {
		t.Fatalf("o domínio do CHECK deveria mandar, veio %q", got)
	}
}

// Regra específica do projeto mora na configuração dele, não no plugin.
func TestOverrideDoProjetoVenceTudo(t *testing.T) {
	state := config.ConfigState{Config: &config.Config{}}
	state.Config.Factory.Expressions.Mappers = map[string]string{
		"TESTE.status": `migrate.FakeChoice("X")`,
		"hash":         "migrate.FakeHashPassword()",
	}

	got := expressaoParaColuna("TESTE", acao.ColunaDefinicao{Name: "status", Type: "string", Length: 5}, []string{"A", "B"}, state)
	if got != `migrate.FakeChoice("X")` {
		t.Fatalf("override por tabela.coluna deveria vencer até o CHECK, veio %q", got)
	}

	got = expressaoParaColuna("OUTRA", acao.ColunaDefinicao{Name: "hash", Type: "string", Length: 60}, nil, state)
	if got != "migrate.FakeHashPassword()" {
		t.Fatalf("override por coluna deveria valer em qualquer tabela, veio %q", got)
	}
}

func TestRenderizaFactoryPreservaExpressoes(t *testing.T) {
	forma := acao.Operacao{Table: "racas", Columns: []acao.ColunaDefinicao{
		{Name: "raca_id", Type: "integer", Precision: 9, PrimaryKey: true},
		{Name: "nome", Type: "string", Length: 60},
		{Name: "sis_data", Type: "date"},
	}}
	existentes := map[string]string{"NOME": `migrate.FakeChoice("Ajustado à mão")`}

	texto := renderizaFactory(forma, nil, config.ConfigState{}, existentes, &migrate.Ruler{Count: 42, Update: true, Active: false})

	if !strings.Contains(texto, `migrate.FakeChoice("Ajustado à mão")`) {
		t.Fatal("a expressão existente deveria ser mantida")
	}
	if !strings.Contains(texto, "Count: 42, Update: true, Active: false") {
		t.Fatal("o Ruler existente deveria ser mantido")
	}
	// A coluna nova, que não estava no arquivo, entra gerada.
	if !strings.Contains(texto, "FakeDate()") {
		t.Fatal("a coluna ausente deveria ser gerada")
	}
	if !strings.Contains(texto, "func RacasFactory() migrate.Factory") {
		t.Fatalf("assinatura errada:\n%s", texto)
	}
}

// A coluna de identidade é preenchida pelo banco; escrever nela obrigaria a
// ligar IDENTITY_INSERT à toa.
func TestIdentidadeFicaDeForaDoArquivoGerado(t *testing.T) {
	forma := acao.Operacao{Table: "racas", Columns: []acao.ColunaDefinicao{
		{Name: "raca_id", Type: "integer", PrimaryKey: true, AutoIncrement: true},
		{Name: "nome", Type: "string", Length: 60},
	}}
	texto := renderizaFactory(forma, nil, config.ConfigState{}, nil, nil)
	if strings.Contains(texto, "raca_id") {
		t.Fatalf("a coluna de identidade não deveria aparecer:\n%s", texto)
	}
}

// O arquivo gerado precisa ser Go válido e legível pelo próprio avaliador.
func TestArquivoGeradoEValidoParaOAvaliador(t *testing.T) {
	forma := acao.Operacao{Table: "racas", Columns: []acao.ColunaDefinicao{
		{Name: "raca_id", Type: "integer", Precision: 9, PrimaryKey: true},
		{Name: "nome", Type: "string", Length: 60},
		{Name: "uf", Type: "char", Length: 2},
		{Name: "cnpj", Type: "string", Length: 20},
	}}
	texto := renderizaFactory(forma, nil, config.ConfigState{}, nil, nil)

	caminho := t.TempDir() + "/racas_factory.go"
	if err := writeFile(caminho, texto); err != nil {
		t.Fatal(err)
	}
	arquivo, err := factorygo.ParseArquivo(caminho)
	if err != nil {
		t.Fatalf("o avaliador não conseguiu ler o arquivo gerado: %v\n%s", err, texto)
	}
	if len(arquivo.Campos) != 4 {
		t.Fatalf("esperava 4 campos, veio %d", len(arquivo.Campos))
	}
	if _, err := arquivo.Linha(0); err != nil {
		t.Fatalf("o arquivo gerado não produz linha: %v", err)
	}
}

func writeFile(caminho, conteudo string) error {
	return os.WriteFile(caminho, []byte(conteudo), 0o644)
}
