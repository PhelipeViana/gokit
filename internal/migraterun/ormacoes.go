package migraterun

// Ações da área ORM, na forma que o menu e a linha de comando compartilham.
//
// Existem porque a composição é o que erra, não cada passo: os dois catálogos têm
// ORDEM entre si, e o mapeamento especial depende do banco enquanto o resto depende
// só do corpus. Deixar cada superfície montar a sua sequência foi exatamente o que
// produziu escritores divergentes do mesmo arquivo.

import (
	"path/filepath"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
)

// AtualizarCatalogos regera os dois catálogos do núcleo a partir do corpus.
//
// A ORDEM não é preferência: o catálogo de colunas resolve o identificador de cada
// tabela consultando o catálogo de TABELAS (`identificadorPorFisico`), que é quem
// decidiu o desempate de apelido. Escrever colunas antes deixaria as duas metades
// discordando — `core.Column.X` apontando para um `core.Table.X` que já não existe.
//
// Não toca no banco: a fonte é a migration.
func AtualizarCatalogos(root, pastaDeMigrate string) error {
	if err := migrationgo.RefreshCatalog(root, pastaDeMigrate); err != nil {
		return err
	}
	return migrationgo.WriteCoreColumnCatalog(root, pastaDeMigrate)
}

// AtualizarCatalogosDoProjeto é o AtualizarCatalogos com a pasta vinda da configuração.
func AtualizarCatalogosDoProjeto(root string, state config.ConfigState) error {
	return AtualizarCatalogos(root, filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
}

// AtualizarEntidades regera catálogos e entidades — o `core` inteiro derivado do corpus.
//
// Devolve o total de entidades e as relações que não viraram atalho de navegação, para
// que quem chamou decida como mostrar.
func AtualizarEntidades(root string, state config.ConfigState) (int, []string, error) {
	if err := AtualizarCatalogosDoProjeto(root, state); err != nil {
		return 0, nil, err
	}
	return GenerateORM(root, state)
}

// ORMTudo roda a área ORM completa: catálogos, entidades e mapeamento especial.
//
// LÊ o banco em dois pontos e não ESCREVE nele em nenhum: a conferência de migration
// pendente e a leitura dos objetos especiais são `SELECT` em catálogo. Diferente do
// reload, que aplica migration — daí não existir aqui a variante "sem banco": o que
// depende de banco aqui é leitura, e sem ela o mapeamento especial não teria fonte.
func ORMTudo(root string, state config.ConfigState, confirmar bool) (int, []string, error) {
	total, semAtalho, err := AtualizarEntidades(root, state)
	if err != nil {
		return total, semAtalho, err
	}
	if err := SpecialMap(root, state, confirmar); err != nil {
		return total, semAtalho, i18n.Errf("orm_all_special_failed", err)
	}
	return total, semAtalho, nil
}
