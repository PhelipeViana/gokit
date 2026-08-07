package migraterun

import (
	"strings"
	"testing"

	"gokit/migration/acao"
)

func seed(id string, first bool, rows ...acao.Linha) seedFile {
	return seedFile{
		Table: "racas", ID: "racas/" + id, Stamp: id,
		Path: "racas/" + id + "_seeder.go", Rows: rows,
		Keys: []string{"raca_id"}, First: first,
	}
}

// O seed inicial declara os IDs fixos; o seguinte herda todos eles.
func TestPosseAcumulaEntreSeeders(t *testing.T) {
	seeds := []seedFile{
		seed("2026_01_01_000000", true,
			acao.Linha{"raca_id": int64(1), "nome": "Branca"},
			acao.Linha{"raca_id": int64(2), "nome": "Preta"}),
		seed("2026_02_01_000000", false,
			acao.Linha{"raca_id": int64(2), "nome": "Preta corrigida"},
			acao.Linha{"raca_id": int64(9), "nome": "Nova"}),
		seed("2026_03_01_000000", false,
			acao.Linha{"raca_id": int64(9), "nome": "Nova corrigida"}),
	}
	assignOwnedKeys(seeds)

	if len(seeds[0].Owned) != 0 {
		t.Fatalf("o seed inicial não herda nada, veio %v", seeds[0].Owned)
	}
	if !seeds[1].Owned["raca_id=1"] || !seeds[1].Owned["raca_id=2"] {
		t.Fatalf("o segundo deveria herdar 1 e 2, veio %v", seeds[1].Owned)
	}
	if seeds[1].Owned["raca_id=9"] {
		t.Fatal("o id 9 é declarado pelo próprio seed 2; ele não o herda")
	}
	// O terceiro herda o 9, declarado pelo segundo: editar passa a ser válido.
	if !seeds[2].Owned["raca_id=9"] {
		t.Fatalf("o terceiro deveria herdar o id 9, veio %v", seeds[2].Owned)
	}
}

// Linha sem a chave é inserção com ID gerado pelo banco: não vira ID fixo,
// porque o valor muda de ambiente para ambiente.
func TestLinhaSemChaveNaoViraIDFixo(t *testing.T) {
	seeds := []seedFile{
		seed("2026_01_01_000000", true, acao.Linha{"raca_id": int64(1), "nome": "Branca"}),
		seed("2026_02_01_000000", false, acao.Linha{"nome": "Sem id"}),
		seed("2026_03_01_000000", false, acao.Linha{"raca_id": int64(1), "nome": "Editada"}),
	}
	assignOwnedKeys(seeds)

	if len(seeds[2].Owned) != 1 || !seeds[2].Owned["raca_id=1"] {
		t.Fatalf("só o id 1 deveria ser fixo, veio %v", seeds[2].Owned)
	}
}

func TestSeedInicialExigeChaveEmTodasAsLinhas(t *testing.T) {
	seeds := []seedFile{
		seed("2026_01_01_000000", true,
			acao.Linha{"raca_id": int64(1), "nome": "Branca"},
			acao.Linha{"nome": "Sem id"}),
	}
	failures := &LoadError{}
	assignOwnedKeys(seeds)
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
	assignOwnedKeys(seeds)
	validateSeeds(seeds, failures)

	if len(failures.Issues) != 0 {
		t.Fatalf("não deveria haver problema, veio: %v", failures.Issues)
	}
}

func TestTabelaSemChavePrimariaERecusada(t *testing.T) {
	sem := seed("2026_01_01_000000", true, acao.Linha{"nome": "Branca"})
	sem.Keys = nil
	failures := &LoadError{}
	validateSeeds([]seedFile{sem}, failures)

	if len(failures.Issues) != 1 {
		t.Fatalf("esperava 1 problema, veio %d", len(failures.Issues))
	}
	if !strings.Contains(failures.Issues[0].Detail, "chave primária") {
		t.Fatalf("erro deveria citar chave primária, veio: %s", failures.Issues[0].Detail)
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

// Chave composta: a assinatura precisa considerar todas as colunas.
func TestPosseComChaveComposta(t *testing.T) {
	composto := func(id string, first bool, rows ...acao.Linha) seedFile {
		s := seed(id, first, rows...)
		s.Keys = []string{"ano", "mes"}
		return s
	}
	seeds := []seedFile{
		composto("2026_01_01_000000", true,
			acao.Linha{"ano": int64(2026), "mes": int64(1), "valor": int64(10)}),
		composto("2026_02_01_000000", false,
			acao.Linha{"ano": int64(2026), "mes": int64(1), "valor": int64(20)},
			acao.Linha{"ano": int64(2026), "mes": int64(2), "valor": int64(30)}),
	}
	assignOwnedKeys(seeds)

	if !seeds[1].Owned["ano=2026, mes=1"] {
		t.Fatalf("deveria possuir a chave composta, veio %v", seeds[1].Owned)
	}
	if seeds[1].Owned["ano=2026, mes=2"] {
		t.Fatal("ano=2026,mes=2 é declarado pelo próprio seed 2")
	}
}
