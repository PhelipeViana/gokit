package migraterun

import (
	"testing"

	"github.com/PhelipeViana/gokit/migration/acao"
)

// O apelido tem de ser resolvível na MESMA migration que o declara.
//
// O registro de alias avançava por ARQUIVO: `advanceAliases` rodava depois de
// executar todas as operações, então uma operação seguinte no mesmo arquivo recebia
// o apelido cru e o banco procurava tabela inexistente. E o validador já observava
// por operação, então `migrate validate` aprovava o que o `migrate run` recusava —
// medido nos quatro dialetos com CreateTable + CreateIndex no mesmo arquivo.
func TestAliasResolveNoMesmoArquivo(t *testing.T) {
	operacoes := []acao.Operacao{
		{Kind: string(acao.CreateTable), Table: "minha_tabela", AliasName: "apelido"},
		{Kind: string(acao.CreateIndex), Table: "apelido", Name: "ix_teste", IndexColumns: []string{"id"}},
		{Kind: string(acao.AddColumn), Table: "apelido", Column: &acao.ColunaDefinicao{Name: "extra", Type: "integer"}},
	}

	aliases := map[string]string{}
	var alvos []string
	for _, operacao := range operacoes {
		resolvida := resolveAlias(operacao, aliases)
		alvos = append(alvos, resolvida.Table)
		advanceAliases(aliases, []acao.Operacao{operacao})
	}

	// A primeira é o próprio CreateTable: ela usa o nome físico e não se resolve.
	if alvos[0] != "minha_tabela" {
		t.Fatalf("CreateTable deveria manter o nome físico, veio %q", alvos[0])
	}
	for posicao, alvo := range alvos[1:] {
		if alvo != "minha_tabela" {
			t.Errorf("operação %d recebeu %q; o apelido do mesmo arquivo deveria resolver para minha_tabela",
				posicao+1, alvo)
		}
	}
}

// RenameTable move o apelido para o nome novo, e registrar por operação não pode
// aplicar o rename duas vezes.
func TestRenameMoveOApelidoUmaVez(t *testing.T) {
	aliases := map[string]string{}
	operacoes := []acao.Operacao{
		{Kind: string(acao.CreateTable), Table: "antiga", AliasName: "apelido"},
		{Kind: string(acao.RenameTable), Table: "antiga", NewName: "nova"},
	}
	for _, operacao := range operacoes {
		advanceAliases(aliases, []acao.Operacao{operacao})
	}
	if aliases["apelido"] != "nova" {
		t.Fatalf("o apelido deveria apontar para nova, aponta para %q", aliases["apelido"])
	}

	// De novo, simulando o registro repetido: o resultado não pode mudar.
	advanceAliases(aliases, operacoes)
	if aliases["apelido"] != "nova" {
		t.Fatalf("registro repetido mudou o destino para %q", aliases["apelido"])
	}
}
