package migraterun

import (
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/migration/acao"
)

func seed(id string, first bool, rows ...acao.Linha) seedFile {
	return seedFile{
		Table: "racas", ID: "racas/" + id, Stamp: id,
		Path: "racas/" + id + "_seeder.go", Rows: rows,
		Keys: []string{"raca_id"}, First: first,
	}
}

func TestSeedInicialExigeChaveEmTodasAsLinhas(t *testing.T) {
	seeds := []seedFile{
		seed("2026_01_01_000000", true,
			acao.Linha{"raca_id": int64(1), "nome": "Branca"},
			acao.Linha{"nome": "Sem id"}),
	}
	failures := &LoadError{}
	validateSeeds(seeds, failures)

	if len(failures.Issues) != 1 {
		t.Fatalf("esperava 1 problema, veio %d: %v", len(failures.Issues), failures.Issues)
	}
	if !strings.Contains(failures.Issues[0].Detail, "linha 2") {
		t.Fatalf("o erro deveria apontar a linha 2, veio: %s", failures.Issues[0].Detail)
	}
}

// Seeder de atualização pode inserir sem chave — é o caso "crio sem imputar".
func TestSeedDeAtualizacaoAceitaLinhaSemChave(t *testing.T) {
	seeds := []seedFile{
		seed("2026_01_01_000000", true, acao.Linha{"raca_id": int64(1), "nome": "Branca"}),
		seed("2026_02_01_000000", false, acao.Linha{"nome": "Sem id"}),
	}
	failures := &LoadError{}
	validateSeeds(seeds, failures)

	if len(failures.Issues) != 0 {
		t.Fatalf("não deveria haver problema, veio: %v", failures.Issues)
	}
}

func TestSeederVazioERecusado(t *testing.T) {
	failures := &LoadError{}
	seeds := []seedFile{seed("2026_01_01_000000", true)}
	validateSeeds(seeds, failures)

	if len(failures.Issues) != 1 || !strings.Contains(failures.Issues[0].Detail, "vazio") {
		t.Fatalf("esperava erro de seeder vazio, veio: %v", failures.Issues)
	}
}

// Tabela SEM chave primária é ACEITA: o seeder é soberano sobre os dados e substitui o
// conteúdo inteiro.
//
// Antes era recusada com "não declara chave primária", e a recusa deixava 306 das 754 tabelas
// do corpus legado (41%) fora do seeder — identidade fora da chave é comum no schema legado.
// A troca é decisão do usuário: sem chave não há como casar linha declarada com existente, e a
// saída é o seeder ser dono do conteúdo.
func TestTabelaSemChavePrimariaEAceita(t *testing.T) {
	sem := seed("2026_01_01_000000", true, acao.Linha{"mes": int64(1), "ano": int64(2026)})
	sem.Keys = nil
	failures := &LoadError{}
	validateSeeds([]seedFile{sem}, failures)

	if len(failures.Issues) != 0 {
		t.Fatalf("tabela sem chave não deveria ser recusada, veio: %v", failures.Issues)
	}
}

// Mas seeder VAZIO segue recusado, com ou sem chave: substituir o conteúdo por nada apagaria a
// tabela inteira, e isso não pode acontecer por omissão.
func TestSeederVazioSemChaveSegueRecusado(t *testing.T) {
	sem := seed("2026_01_01_000000", true)
	sem.Keys = nil
	failures := &LoadError{}
	validateSeeds([]seedFile{sem}, failures)

	if len(failures.Issues) != 1 {
		t.Fatalf("seeder vazio sem chave deveria ser recusado, veio: %v", failures.Issues)
	}
}
