package migraterun

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/config"
)

// Arquivo gerado não leva tag de anotação. A regra tem uma razão concreta: uma
// tag num *.gen.go apareceria na árvore do usuário como tarefa dele, e sumiria
// na geração seguinte sem que ninguém tivesse mexido. O glob de exclusão do
// todo-tree é a segunda trava; esta é a primeira, e vale para quem não usa o
// plugin.
//
// A varredura é nos GERADORES: se a tag não existe no template, não pode
// aparecer na saída.
func TestGeradoresNaoEmitemTagDeAnotacao(t *testing.T) {
	var vocabulario []string
	for _, entrada := range config.TagsAnotacao {
		vocabulario = append(vocabulario, regexp.QuoteMeta(entrada.Tag))
		for _, sinonimo := range entrada.Sinonimos {
			vocabulario = append(vocabulario, regexp.QuoteMeta(sinonimo))
		}
	}
	// Só a forma anotação (TAG seguida de dois-pontos) importa. migrate.TODO()
	// é uma operação do DSL, não um comentário.
	tag := regexp.MustCompile(`\b(` + strings.Join(vocabulario, "|") + `)\s*:`)

	geradores := []string{"ormgen.go", "factorycreate.go", "seed.go", "runner.go", "documentation.go"}
	for _, gerador := range geradores {
		fonte, err := os.ReadFile(gerador)
		if err != nil {
			t.Fatalf("ler %s: %v", gerador, err)
		}
		for numero, linha := range strings.Split(string(fonte), "\n") {
			// Comentário do próprio gokit pode falar de TODO à vontade; o que
			// não pode é a tag entrar no texto emitido.
			if strings.HasPrefix(strings.TrimSpace(linha), "//") {
				continue
			}
			if !strings.Contains(linha, `"`) && !strings.Contains(linha, "`") {
				continue
			}
			if achado := tag.FindString(linha); achado != "" {
				t.Errorf("%s:%d emite a tag %q num template de arquivo gerado", gerador, numero+1, achado)
			}
		}
	}
}
