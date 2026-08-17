package migraterun

// Conferência de pendência antes de GERAR artefato tipado.
//
// O problema que isto evita é de correção, não de organização: a camada gerada — entidade,
// coluna, tabela, e no futuro serviço e handler — é derivada do CORPUS, por AST. O corpus
// pode declarar uma tabela `alunos` que ninguém aplicou ainda. Gerar em cima disso produz
// um artefato tipado, que compila, que o editor completa — e que falha no primeiro uso,
// porque a tabela não existe no banco.
//
// O custo desse erro é alto pela DISTÂNCIA entre causa e sintoma: quem sente é o
// programador que consumiu o serviço gerado, e a causa é uma migration não aplicada por
// outra pessoa, possivelmente noutro dia. O sintoma não diz nada disso.
//
// O caso inverso é o mais comum no dia a dia: puxar uma versão em que todo o corpus está
// escrito e o banco local ainda não tem as tabelas. Aí a saída é uma só — rodar o reload,
// que aplica o que falta.
//
// Por que a checagem não é obrigatória: `gokit orm` é deliberadamente independente de
// banco — a fonte é o corpus, e isso permite gerar em CI, offline, ou antes de qualquer
// container subir. Então a regra é: se o banco RESPONDE e há pendência, recusa; se o banco
// não responde, avisa que não conferiu e segue. Nunca transformar ausência de banco em
// impedimento, e nunca deixar pendência conhecida passar em silêncio.

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/config"
)

// pendenciaNoBanco compara o corpus com o histórico do banco.
//
// Devolve os nomes das migrations não aplicadas e se a conferência pôde ser feita. Banco
// inalcançável devolve `conferido = false` sem erro: não é falha, é ausência de resposta.
func pendenciaNoBanco(root string, state config.ConfigState) (naoAplicadas []string, conferido bool) {
	if state.Config == nil {
		return nil, false
	}
	arquivos, err := loadPlans(filepath.Join(root, filepath.FromSlash(state.Config.Output.Migrate)))
	if err != nil || len(arquivos) == 0 {
		return nil, false
	}

	connection := state.Config.Connections[state.ActiveClient]
	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	if driver == "" {
		return nil, false
	}
	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return nil, false
	}
	defer db.Close()

	// Espera curta de propósito: a conferência é uma cortesia, não o trabalho. Se o banco
	// está lento ou fora, quem pediu geração não deve ficar preso por isso.
	ctx, cancelar := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancelar()
	if err := db.PingContext(ctx); err != nil {
		return nil, false
	}

	historico, _, err := loadHistory(ctx, db, dialect, connection.Schema, state.Config.Migrate.Table)
	if err != nil {
		// Sem tabela de histórico o banco é novo: TUDO está pendente, e isso é uma
		// resposta, não uma falha de conferência.
		nomes := make([]string, 0, len(arquivos))
		for _, arquivo := range arquivos {
			nomes = append(nomes, arquivo.Name)
		}
		return nomes, true
	}

	var pendentes []string
	for _, arquivo := range arquivos {
		if !aplicada(arquivo, historico) {
			pendentes = append(pendentes, arquivo.Name)
		}
	}
	return pendentes, true
}

// primeirosNomes recorta a lista para a mensagem.
//
// Num corpus legado a pendência pode ser de centenas de arquivos, e despejar todos afoga
// a instrução que resolve. Cinco nomes bastam para reconhecer o que está faltando.
func primeirosNomes(nomes []string, quantos int) string {
	if len(nomes) <= quantos {
		return strings.Join(nomes, ", ")
	}
	return strings.Join(nomes[:quantos], ", ") + ", …"
}
