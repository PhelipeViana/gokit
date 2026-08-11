# 🧭 Editores e anotações

> [!NOTE]
> Gerado pelo GoKit no reload. Editar este arquivo é seguro: o GoKit não o sobrescreve depois de criado. As chaves em `.vscode/settings.json` também são preservadas — o gokit só acrescenta as que faltam.

Este guia foi gerado pelo GoKit. Ele descreve as anotações do projeto e como cada editor as apresenta. Nada aqui é obrigatório: as anotações são comentários de código comum e funcionam sem plugin nenhum — o plugin só as organiza em árvore.

## Tags

A tag fica **em inglês** e o texto no seu idioma. O motivo é prático: `TODO` e `FIXME` são reconhecidos pelo Go, pelo golangci-lint, pelo gopls e por qualquer IDE; uma tag traduzida perderia tudo isso. Para quem prefere escrever em português ou espanhol, os sinônimos abaixo são varridos e agrupados na tag inglesa.

| Tag | Quando usar | Sinônimos varridos |
|:---|:---|:---|
| `TODO:` | trabalho combinado que ainda falta | `TAREFA`, `PENDENTE`, `TAREA`, `PENDIENTE` |
| `FIXME:` | código errado ou frágil que precisa de correção | `CORRIGIR`, `CORREGIR` |
| `NOTE:` | explicação de uma decisão que não é óbvia no código | `NOTA` |
| `WARN:` | armadilha: mexer aqui quebra algo em outro lugar | `ATENCAO`, `ATENÇÃO`, `ATENCION`, `ATENCIÓN` |
| `HACK:` | solução provisória que só existe por uma limitação externa | `GAMBIARRA`, `APAÑO` |
| `GOKIT:` | ponto de contato com o GoKit (regerar, revalidar, migrar) | — |

## Arquivos gerados não recebem anotação

Nenhum `*.gen.go` contém tag de anotação, e a varredura ignora esses arquivos por glob. São duas travas para o mesmo problema: uma tag num arquivo gerado apareceria na sua lista como se fosse trabalho seu, e desapareceria na próxima geração sem que ninguém tivesse feito nada.

## Por editor

**VS Code / Cursor / Windsurf** — o `.vscode/extensions.json` gerado sugere as extensões e o `.vscode/settings.json` já traz as tags, os sinônimos, as cores e o glob de exclusão. As extensões sugeridas são: Go, todo-tree (árvore de anotações), REST Client (executa o `api.gen.http` direto do editor), Markdown All in One com Mermaid (lê a documentação gerada) e Docker.

**GoLand / IntelliJ** — não precisa de plugin: a janela **TODO** já lista as anotações, arquivos `.http` são executados nativamente pelo HTTP Client e `.md` tem visualização embutida. Para varrer os sinônimos em português, acrescente os padrões em *Settings → Editor → TODO*. Para ocultar os gerados, use o escopo *Project Files sem `*.gen.go`* no filtro da janela TODO.

**Neovim / Helix / Emacs** — as anotações são texto, então `grep`, `ripgrep` ou a lista de quickfix já resolvem. No Neovim, `todo-comments.nvim` faz o mesmo papel do todo-tree; aponte os sinônimos na configuração dele e exclua `*.gen.go` do `rg`.

**Qualquer terminal** — a varredura sem editor nenhum:

```bash
rg -n --glob '!**/*.gen.go' '\b(APAÑO|ATENCAO|ATENCION|ATENCIÓN|ATENÇÃO|CORREGIR|CORRIGIR|FIXME|GAMBIARRA|GOKIT|HACK|NOTA|NOTE|PENDENTE|PENDIENTE|TAREA|TAREFA|TODO|WARN):'
```

## .editorconfig

O `.editorconfig` na raiz fixa fim de linha, codificação e indentação. Go usa tabulação por decisão do gofmt, e o resto do projeto usa espaço. Praticamente todo editor respeita esse arquivo — alguns nativamente, outros por extensão.
