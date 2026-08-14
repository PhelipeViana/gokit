package migraterun

// Regressão do escopo da conciliação de FK.
//
// A conciliação existe para recuperar FK ausente em banco criado por versão antiga
// do gokit. O escopo dela é o que estava errado: varrendo o corpus INTEIRO, ela
// tentava criar a FK de migration ainda PENDENTE — cuja tabela não existe — e
// abortava o `migrate run` em qualquer banco novo, em qualquer dialeto, com qualquer
// corpus que tivesse um `.References()`.
//
// O teste é sobre o filtro, e não sobre o SQL: sem banco, `aplicada` é o predicado
// que decide se a FK daquele arquivo é sequer considerada. Se ele voltar a dizer
// "sim" para migration pendente, o bug volta, e volta em silêncio — porque o projeto
// de teste de sempre já está migrado e a conciliação não faz nada nele.

import "testing"

func TestConciliacaoSoConsideraMigrationJaAplicada(t *testing.T) {
	aplicadaPorID := migrationFile{ID: "2026_01_01_000001", Name: "2026_01_01_000001_create_pai.go"}
	// Histórico antigo era indexado pelo NOME do arquivo, não pelo ID. Os dois têm de
	// contar como aplicada, senão a atualização do gokit refaria FK em banco velho.
	aplicadaPorNome := migrationFile{ID: "2026_01_01_000003", Name: "2026_01_01_000003_create_neto.go"}
	pendente := migrationFile{ID: "2026_01_01_000002", Name: "2026_01_01_000002_create_filho.go"}

	history := map[string]string{
		aplicadaPorID.ID:     "checksum-a",
		aplicadaPorNome.Name: "checksum-c",
	}

	casos := []struct {
		nome     string
		arquivo  migrationFile
		esperado bool
	}{
		{"aplicada, no histórico por ID", aplicadaPorID, true},
		{"aplicada, no histórico por nome (formato antigo)", aplicadaPorNome, true},
		{"pendente: a tabela dela ainda não existe", pendente, false},
	}
	for _, caso := range casos {
		if obtido := aplicada(caso.arquivo, history); obtido != caso.esperado {
			t.Errorf("%s: esperado %v, obtido %v", caso.nome, caso.esperado, obtido)
		}
	}
}

// Histórico vazio é o banco NOVO — o caso que quebrava. Nenhuma FK pode ser
// conciliada ali: tudo está pendente e cada migration cria a própria.
func TestConciliacaoNaoConsideraNadaEmBancoNovo(t *testing.T) {
	vazio := map[string]string{}
	arquivos := []migrationFile{
		{ID: "2026_01_01_000001", Name: "a.go"},
		{ID: "2026_01_01_000002", Name: "b.go"},
	}
	for _, arquivo := range arquivos {
		if aplicada(arquivo, vazio) {
			t.Errorf("%s não deveria contar como aplicada em banco novo", arquivo.Name)
		}
	}
}
