package migraterun

// Regressão do teto de linha do MySQL.
//
// O MySQL limita a DEFINIÇÃO da linha a 65.535 bytes somando todas as colunas. É limite de
// formato, não de armazenamento — ROW_FORMAT não resolve. Medido: 4 tabelas de 754 do corpus
// legado estouram, e só as 17 VARCHAR(1000) de `anamnese` dão 68.000 bytes em utf8mb4.

import (
	"testing"

	"github.com/PhelipeViana/gokit/migration/acao"
)

func varchar(nome string, tamanho int) acao.ColunaDefinicao {
	return acao.ColunaDefinicao{Name: nome, Type: "string", Length: tamanho, Nullable: true}
}

// Linha que cabe não é tocada: 750 das 754 tabelas passam por aqui e não podem mudar.
func TestLinhaQueCabeNaoEhPromovida(t *testing.T) {
	colunas := []acao.ColunaDefinicao{
		{Name: "id", Type: "integer"},
		varchar("nome", 200),
		varchar("descricao", 1000),
	}
	ajustadas, promovidas := promoverParaTextNoMySQL(colunas, nil)
	if len(promovidas) != 0 {
		t.Fatalf("nada deveria ser promovido, veio %v", promovidas)
	}
	for i := range colunas {
		if ajustadas[i].Type != colunas[i].Type {
			t.Errorf("coluna %s mudou de tipo sem necessidade", colunas[i].Name)
		}
	}
}

// Promove da mais larga para a mais estreita e PARA quando cabe: é o que troca o menor
// número de colunas.
func TestPromovePelaMaisLargaEParaQuandoCabe(t *testing.T) {
	var colunas []acao.ColunaDefinicao
	for i := 0; i < 20; i++ {
		colunas = append(colunas, varchar("media", 1000))
	}
	colunas = append(colunas, varchar("larguissima", 4000), varchar("estreita", 10))

	ajustadas, promovidas := promoverParaTextNoMySQL(colunas, nil)
	if len(promovidas) == 0 {
		t.Fatal("a linha estoura o teto e deveria promover")
	}
	if promovidas[0] != "larguissima" {
		t.Errorf("a primeira promovida deveria ser a mais larga, veio %q", promovidas[0])
	}
	// A estreita não pode ser tocada — promover tudo seria o caminho fácil e errado.
	for _, nome := range promovidas {
		if nome == "estreita" {
			t.Error("a coluna estreita não deveria ser promovida")
		}
	}
	// E o resultado tem de caber.
	total := 0
	for _, coluna := range ajustadas {
		total += bytesNoMySQL(coluna)
	}
	if total > tetoDeLinhaNoMySQL {
		t.Errorf("depois da promoção a linha ainda tem %d bytes", total)
	}
}

// Coluna com DEFAULT não é candidata: TEXT no MySQL não aceita DEFAULT literal, e promover
// ali trocaria o erro 1118 por um erro de sintaxe — mais difícil de ler.
func TestColunaComDefaultNaoEhPromovida(t *testing.T) {
	comDefault := varchar("com_default", 4000)
	comDefault.Default = "vazio"
	colunas := []acao.ColunaDefinicao{comDefault}
	for i := 0; i < 20; i++ {
		colunas = append(colunas, varchar("media", 1000))
	}
	_, promovidas := promoverParaTextNoMySQL(colunas, nil)
	for _, nome := range promovidas {
		if nome == "com_default" {
			t.Error("coluna com DEFAULT não deveria ser promovida")
		}
	}
}

// Coluna indexada também não: índice sobre TEXT no MySQL exige prefixo.
func TestColunaIndexadaNaoEhPromovida(t *testing.T) {
	colunas := []acao.ColunaDefinicao{varchar("indexada", 4000)}
	for i := 0; i < 20; i++ {
		colunas = append(colunas, varchar("media", 1000))
	}
	_, promovidas := promoverParaTextNoMySQL(colunas, map[string]bool{"indexada": true})
	for _, nome := range promovidas {
		if nome == "indexada" {
			t.Error("coluna indexada não deveria ser promovida")
		}
	}
}

// O mapa de índices tem de cobrir o corpus INTEIRO, não só a migration do create_table: o
// índice pode vir depois, quando a promoção já aconteceu.
func TestIndicesDoCorpusCobremMigrationPosterior(t *testing.T) {
	RegistrarIndicesDoCorpus([]acao.Operacao{
		{Kind: "create_table", Table: "anamnese"},
		{Kind: "create_index", Table: "ANAMNESE", IndexColumns: []string{"Biotipo"}},
	})
	indexadas := colunasIndexadasNoCorpus("anamnese")
	if !indexadas["biotipo"] {
		t.Errorf("a coluna indexada por migration posterior deveria constar: %v", indexadas)
	}
}
