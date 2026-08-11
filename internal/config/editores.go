package config

// Configuração de editor gerada pelo GoKit.
//
// A intenção é que a árvore de anotações, o executor de .http e o leitor de
// Markdown funcionem sem ninguém configurar nada — e sem obrigar VS Code. Por
// isso há três camadas: as anotações são comentário comum (funcionam em
// qualquer editor), o .editorconfig é padrão de indústria, e só o .vscode/ é
// específico. O guia em docs/editores.md cobre o resto.
//
// Nada aqui sobrescreve trabalho do usuário: arquivo existente é preservado e
// o settings.json recebe apenas as chaves que faltam.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PhelipeViana/gokit/internal/i18n"
)

// TagAnotacao é uma tag canônica mais os sinônimos aceitos na varredura.
//
// A tag canônica fica em inglês porque é o que Go, golangci-lint, gopls e as
// IDEs reconhecem; traduzi-la perderia o realce nativo, a navegação e o
// relatório do linter. Os sinônimos existem para quem escreve o comentário em
// português ou espanhol: o todo-tree os agrupa sob a tag inglesa, então a
// árvore fica igual dos dois jeitos.
type TagAnotacao struct {
	Tag       string
	Sinonimos []string
	Cor       string
	ChaveDoc  string // chave i18n com o "quando usar"
}

// TagsAnotacao é o vocabulário do projeto. GOKIT não tem sinônimo: é marca da
// ferramenta, não frase de quem escreve.
var TagsAnotacao = []TagAnotacao{
	{Tag: "TODO", Sinonimos: []string{"TAREFA", "PENDENTE", "TAREA", "PENDIENTE"}, Cor: "#F1FA8C", ChaveDoc: "edt_tag_todo"},
	{Tag: "FIXME", Sinonimos: []string{"CORRIGIR", "CORREGIR"}, Cor: "#FF5555", ChaveDoc: "edt_tag_fixme"},
	{Tag: "NOTE", Sinonimos: []string{"NOTA"}, Cor: "#8BE9FD", ChaveDoc: "edt_tag_note"},
	{Tag: "WARN", Sinonimos: []string{"ATENCAO", "ATENÇÃO", "ATENCION", "ATENCIÓN"}, Cor: "#FFB86C", ChaveDoc: "edt_tag_warn"},
	{Tag: "HACK", Sinonimos: []string{"GAMBIARRA", "APAÑO"}, Cor: "#BD93F9", ChaveDoc: "edt_tag_hack"},
	{Tag: "GOKIT", Cor: "#50FA7B", ChaveDoc: "edt_tag_gokit"},
}

// globsIgnorados tira da árvore o que não é trabalho de ninguém. O primeiro é o
// que mais importa: anotação em arquivo gerado apareceria como tarefa do
// usuário e sumiria na geração seguinte.
var globsIgnorados = []string{
	"**/*.gen.go",
	"**/vendor/**",
	"**/node_modules/**",
	"**/internal/gokit/.state/**",
	"**/internal/gokit/gokit_local/**",
}

// EscreverConfigEditores grava as configurações e devolve o que foi escrito.
// Arquivo que já existe é preservado; a lista sai vazia quando não havia nada a
// fazer, e o aviso é preenchido quando o settings.json não pôde ser mesclado.
func EscreverConfigEditores(root string) (escritos []string, aviso string, err error) {
	pastaVSCode := filepath.Join(root, ".vscode")
	if err := os.MkdirAll(pastaVSCode, 0o755); err != nil {
		return nil, "", i18n.Errf("edt_mkdir_failed", ".vscode", err)
	}

	feito, err := escreverSeAusente(filepath.Join(pastaVSCode, "extensions.json"), extensionsJSON())
	if err != nil {
		return nil, "", err
	}
	if feito {
		escritos = append(escritos, ".vscode/extensions.json")
	}

	alvo := filepath.Join(pastaVSCode, "settings.json")
	mesclado, aviso, err := mesclarSettings(alvo)
	if err != nil {
		return nil, "", err
	}
	if mesclado != "" {
		// filepath.Rel em vez de recortar o prefixo: o root pode chegar com "/"
		// no Windows, e o recorte deixaria o caminho absoluto no relatório.
		relativo, relErr := filepath.Rel(root, mesclado)
		if relErr != nil {
			relativo = mesclado
		}
		escritos = append(escritos, filepath.ToSlash(relativo))
	}

	feito, err = escreverSeAusente(filepath.Join(root, ".editorconfig"), editorConfig())
	if err != nil {
		return nil, "", err
	}
	if feito {
		escritos = append(escritos, ".editorconfig")
	}

	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		return nil, "", i18n.Errf("edt_mkdir_failed", "docs", err)
	}
	feito, err = escreverSeAusente(filepath.Join(docs, "editores.md"), guiaEditores())
	if err != nil {
		return nil, "", err
	}
	if feito {
		escritos = append(escritos, "docs/editores.md")
	}

	return escritos, aviso, nil
}

func escreverSeAusente(caminho, conteudo string) (bool, error) {
	if _, err := os.Stat(caminho); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, i18n.Errf("edt_write_failed", filepath.Base(caminho), err)
	}
	if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
		return false, i18n.Errf("edt_write_failed", filepath.Base(caminho), err)
	}
	return true, nil
}

// mesclarSettings acrescenta ao settings.json apenas as chaves que faltam.
//
// Se o arquivo existe mas não é JSON estrito — o VS Code aceita comentários e
// vírgula sobrando, e muita gente usa —, ele não é tocado: as chaves do gokit
// vão para um arquivo ao lado e o aviso explica. Reescrever perderia os
// comentários do usuário, e adivinhar JSONC aqui seria pior que avisar.
func mesclarSettings(caminho string) (escrito string, aviso string, err error) {
	nossas := settingsGoKit()

	atual, leErr := os.ReadFile(caminho)
	if os.IsNotExist(leErr) {
		return caminho, "", gravarJSON(caminho, nossas)
	}
	if leErr != nil {
		return "", "", i18n.Errf("edt_write_failed", "settings.json", leErr)
	}

	var existente map[string]json.RawMessage
	if json.Unmarshal(atual, &existente) != nil {
		lado := filepath.Join(filepath.Dir(caminho), "gokit.settings.json")
		if err := gravarJSON(lado, nossas); err != nil {
			return "", "", err
		}
		return lado, i18n.Tf("edt_settings_unparseable", ".vscode/settings.json", ".vscode/gokit.settings.json"), nil
	}

	faltando := map[string]any{}
	for chave, valor := range nossas {
		if _, tem := existente[chave]; !tem {
			faltando[chave] = valor
		}
	}
	if len(faltando) == 0 {
		return "", "", nil
	}
	for chave, valor := range faltando {
		bruto, err := json.Marshal(valor)
		if err != nil {
			return "", "", i18n.Errf("edt_write_failed", "settings.json", err)
		}
		existente[chave] = bruto
	}
	return caminho, "", gravarJSON(caminho, existente)
}

func gravarJSON(caminho string, valor any) error {
	// Chave ordenada e indentação de 2 espaços: o arquivo entra no Git, então
	// duas gerações seguidas não podem produzir diff diferente.
	corpo, err := json.MarshalIndent(valor, "", "  ")
	if err != nil {
		return i18n.Errf("edt_write_failed", filepath.Base(caminho), err)
	}
	if err := os.WriteFile(caminho, append(corpo, '\n'), 0o644); err != nil {
		return i18n.Errf("edt_write_failed", filepath.Base(caminho), err)
	}
	return nil
}

func extensionsJSON() string {
	corpo, _ := json.MarshalIndent(map[string][]string{
		"recommendations": {
			"golang.go",
			"Gruntfuggly.todo-tree",
			"humao.rest-client",
			"yzhang.markdown-all-in-one",
			"bierner.markdown-mermaid",
			"ms-azuretools.vscode-docker",
			"editorconfig.editorconfig",
		},
	}, "", "  ")
	return string(corpo) + "\n"
}

// settingsGoKit são as chaves que o gokit conhece. Só entram no settings.json
// as que ainda não existirem lá.
func settingsGoKit() map[string]any {
	var tags []string
	grupos := map[string][]string{}
	realces := map[string]any{}

	for _, entrada := range TagsAnotacao {
		tags = append(tags, entrada.Tag)
		tags = append(tags, entrada.Sinonimos...)
		if len(entrada.Sinonimos) > 0 {
			grupos[entrada.Tag] = append([]string{entrada.Tag}, entrada.Sinonimos...)
		}
		realces[entrada.Tag] = map[string]any{
			"foreground": entrada.Cor,
			"type":       "tag",
		}
	}

	return map[string]any{
		"todo-tree.general.tags":               tags,
		"todo-tree.general.tagGroups":          grupos,
		"todo-tree.highlights.customHighlight": realces,
		"todo-tree.filtering.excludeGlobs":     globsIgnorados,
		"todo-tree.tree.showCountsInTree":      true,
		"todo-tree.tree.groupedByTag":          true,
		"files.associations":                   map[string]string{"*.http": "http"},
		"files.eol":                            "\n",
		"[go]": map[string]any{
			"editor.formatOnSave":     true,
			"editor.insertSpaces":     false,
			"editor.defaultFormatter": "golang.go",
		},
		"[markdown]": map[string]any{
			"editor.wordWrap": "on",
		},
	}
}

func editorConfig() string {
	return `# Gerado pelo GoKit. Fixa fim de linha, codificação e indentação para que o
# projeto fique igual em qualquer editor. Go usa tabulação por decisão do gofmt.
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
indent_style = space
indent_size = 2

[*.go]
indent_style = tab
indent_size = 4

[*.{md,markdown}]
# Duas quebras no fim da linha são sintaxe de Markdown; aparar mataria o <br>.
trim_trailing_whitespace = false

[*.{yml,yaml,json}]
indent_size = 2

[Makefile]
indent_style = tab
`
}

// guiaEditores monta o docs/editores.md no idioma ativo. A tabela de tags sai
// do mesmo TagsAnotacao que alimenta o settings.json, então documentação e
// configuração não conseguem divergir.
func guiaEditores() string {
	var b strings.Builder

	b.WriteString(i18n.T("edt_doc_title") + "\n\n")
	b.WriteString(i18n.T("edt_doc_regen") + "\n\n")
	b.WriteString(i18n.T("edt_doc_intro") + "\n\n")

	b.WriteString(i18n.T("edt_doc_tags_title") + "\n\n")
	b.WriteString(i18n.T("edt_doc_tags_intro") + "\n\n")
	b.WriteString(i18n.T("edt_doc_tags_header") + "\n")
	for _, entrada := range TagsAnotacao {
		sinonimos := "—"
		if len(entrada.Sinonimos) > 0 {
			citados := make([]string, 0, len(entrada.Sinonimos))
			for _, sinonimo := range entrada.Sinonimos {
				citados = append(citados, "`"+sinonimo+"`")
			}
			sinonimos = strings.Join(citados, ", ")
		}
		b.WriteString("| `" + entrada.Tag + ":` | " + i18n.T(entrada.ChaveDoc) + " | " + sinonimos + " |\n")
	}
	b.WriteString("\n")

	b.WriteString(i18n.T("edt_doc_gen_title") + "\n\n")
	b.WriteString(i18n.T("edt_doc_gen_body") + "\n\n")

	b.WriteString(i18n.T("edt_doc_editors_title") + "\n\n")
	for _, chave := range []string{"edt_doc_vscode", "edt_doc_jetbrains", "edt_doc_neovim"} {
		b.WriteString(i18n.T(chave) + "\n\n")
	}
	b.WriteString(i18n.T("edt_doc_terminal") + "\n\n")
	b.WriteString("```bash\n" + comandoRipgrep() + "\n```\n\n")

	b.WriteString(i18n.T("edt_doc_editorconfig_title") + "\n\n")
	b.WriteString(i18n.T("edt_doc_editorconfig_body") + "\n")

	return b.String()
}

// comandoRipgrep escreve a busca equivalente à árvore do editor, com as mesmas
// tags e a mesma exclusão dos gerados.
func comandoRipgrep() string {
	var alternativas []string
	for _, entrada := range TagsAnotacao {
		alternativas = append(alternativas, entrada.Tag)
		alternativas = append(alternativas, entrada.Sinonimos...)
	}
	sort.Strings(alternativas)
	return "rg -n --glob '!**/*.gen.go' '\\b(" + strings.Join(alternativas, "|") + "):'"
}
