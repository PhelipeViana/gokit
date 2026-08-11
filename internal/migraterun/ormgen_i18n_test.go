package migraterun

import (
	"os"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/i18n"
)

// Os templates de geração usam {{chave}} para o texto traduzido. Chave errada
// não quebra a compilação do gokit: ela sai literal no arquivo gerado do
// cliente, e só aparece quando alguém lê o response.gen.go. Este teste lê o
// próprio fonte e cobra que toda marca tenha tradução.
func TestPlaceholdersDosTemplatesTemTraducao(t *testing.T) {
	for _, arquivo := range []string{"ormgen.go"} {
		fonte, err := os.ReadFile(arquivo)
		if err != nil {
			t.Fatalf("ler %s: %v", arquivo, err)
		}
		for numero, linha := range strings.Split(string(fonte), "\n") {
			// A doc de traduzirPlaceholders cita {{chave}} como exemplo; só as
			// marcas dentro do template interessam.
			if strings.HasPrefix(strings.TrimSpace(linha), "//") {
				continue
			}
			for _, marca := range placeholderChave.FindAllStringSubmatch(linha, -1) {
				chave := marca[1]
				if i18n.T(chave) == chave {
					t.Errorf("%s:%d: {{%s}} não tem tradução registrada", arquivo, numero+1, chave)
				}
			}
		}
	}
}
