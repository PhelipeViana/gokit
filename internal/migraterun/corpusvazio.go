package migraterun

// Conferência de corpus vazio para a leitura do banco.
//
// Regra do projeto: `migrate import` lê o banco e escreve APENAS as migrations, e só em
// projeto em branco. Fora disso a escrita seria por cima de um corpus já existente —
// possivelmente gerado por uma versão anterior do leitor —, e as duas gerações ficariam
// indistinguíveis no mesmo lugar.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PhelipeViana/gokit/internal/config"
)

// migrationsNoCorpus devolve os nomes dos arquivos de migration já escritos.
//
// Conta ARQUIVO, não tabela declarada: um corpus pode ter migration que não cria tabela
// nenhuma (raw_sql, todo, drop), e ela também é escrita que já existe. Contar tabelas
// deixaria essas passarem.
func migrationsNoCorpus(root string, state config.ConfigState) ([]string, error) {
	if state.Config == nil {
		return nil, nil
	}
	base := pastaDeMigrations(root, state.Config.Output.Migrate)
	var nomes []string
	err := filepath.WalkDir(base, func(caminho string, entrada os.DirEntry, err error) error {
		if err != nil {
			// Pasta ainda não existe é o caso NORMAL do projeto em branco, não falha.
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entrada.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entrada.Name()), ".go") {
			return nil
		}
		nomes = append(nomes, entrada.Name())
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Strings(nomes)
	return nomes, nil
}
