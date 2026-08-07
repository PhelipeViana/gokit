---
name: gokit-migrations
description: Como trabalhar no motor de migrations, seeds e factories do GoKit e validar qualquer mudança nos quatro dialetos (Oracle, Postgres, MySQL, SQL Server). Use ao alterar o DSL (migration/, acao/), os parsers AST (internal/migrationgo/, internal/factorygo/), o executor (internal/migraterun/), o seed ou as factories; ao investigar erro de migration/seed/factory; ao adicionar operação ou função Fake* nova; ou quando aparecer ORA-, SQLSTATE, "Could not create constraint", drift de checksum ou divergência de sequência/identity.
---

# Motor de migrations do GoKit

Plugin em Go que lê migrations declarativas **por AST** (nunca compila nem executa
o arquivo) e aplica em Oracle, Postgres, MySQL e SQL Server a partir de um único
corpus.

## Regra inegociável: medir, não supor

Comportamento de banco é contraintuitivo e diverge entre os quatro. **Toda
afirmação sobre um dialeto precisa de evidência de execução.** Nesta base já
custou caro supor errado — e a suposição sempre parecia razoável.

Se a resposta depende de "como o Oracle trata X", rode contra o container e
mostre a saída. Os quatro bancos estão de pé no `prevcontas_test`.

## Ciclo de trabalho

```powershell
.\testlab\lab.ps1 matrix       # build + validate + limpo + idempotência nos 4
.\testlab\lab.ps1 contract     # regressão do contrato de seed/ID fixo
.\testlab\lab.ps1 sql all "SELECT ..."
.\testlab\lab.ps1 reset oracle
```

Sem o testlab, o equivalente manual:

```bash
GOOS=windows GOARCH=amd64 go build -ldflags "-X main.CommitHash=development" \
  -o "<projeto-teste>/gokit-windows-amd64.exe" .
cd <projeto-teste> && ./gokit-windows-amd64.exe migrate validate && ./gokit-windows-amd64.exe migrate run
```

**Sempre compile o binário de teste com `CommitHash=development`.** Com o hash
real, o updater busca a versão do GitHub e sobrescreve o build local por um mais
antigo — apagando o trabalho em silêncio ([internal/updater/updater.go:37](internal/updater/updater.go#L37)).

Ordem que economiza tempo: `validate` (não toca no banco, ~0,5 s) antes de
qualquer `run`. Ele acumula **todos** os problemas do corpus agrupados por causa,
em vez de parar no primeiro.

## Diferenças entre dialetos que já morderam

| | Oracle | Postgres | MySQL | SQL Server |
|---|---|---|---|---|
| DDL transacional | **não** (auto-commit) | sim | **não** | sim |
| DML transacional | sim | sim | sim | sim |
| Identity aceita valor explícito | sim (`BY DEFAULT`) | sim | sim | só com `SET IDENTITY_INSERT` |
| Adicionar identity a coluna existente | sim | sim | sim | **impossível** |
| Identity avança em transação revertida | não | não | não | **sim** |
| Desligar FK globalmente | não (uma a uma) | `session_replication_role` | `FOREIGN_KEY_CHECKS=0` | `NOCHECK CONSTRAINT ALL` |
| PK promove coluna a NOT NULL | sim | sim | sim | **não** |
| Nome de constraint único por | schema | tabela | tabela | schema |

Outros pontos concretos:

- **`DEFAULT` antes das constraints inline.** `PRIMARY KEY DEFAULT x` dá ORA-03076.
  Ordem que passa nos quatro: tipo → `DEFAULT` → nulidade → constraint.
- **PK composta** vira constraint de tabela; `PRIMARY KEY` inline em cada coluna
  dá ORA-02260.
- **MySQL exige que coluna `AUTO_INCREMENT` seja chave**; os outros três não.
- **Nome de constraint truncado em 30 chars** colide (ORA-02264) — use
  `shortenIdentifier`, que anexa hash determinístico.
- **`/` sozinho numa linha** é diretiva do SQL*Plus, não SQL. Blocos PL/SQL
  múltiplos precisam ser divididos antes do `ExecContext`.
- **MySQL em Linux é case-sensitive** para nome de tabela; SQL escrito em
  maiúsculas quebra contra tabelas criadas em minúsculas.
- **`SET IDENTITY_INSERT` é por sessão** — tem que estar na mesma conexão dos
  inserts, ou seja, dentro de uma transação. Com pool, o `SET` e o `INSERT` caem
  em conexões diferentes.

## Invariantes do motor

Quebrar qualquer um destes é regressão, mesmo que o teste passe:

1. **Sequência nunca anda para trás.** `resyncIdentity` usa
   `max(MAX(id)+1, próximo_atual)`. Se recuar, o banco reemite IDs já entregues e
   FKs passam a apontar para outra linha, sem erro nenhum. Gap é inofensivo;
   reemissão não.
2. **Seed nunca sobrescreve linha que não é dele.** Linha idêntica = no-op
   (reexecução segura); linha divergente = **erro**, porque pode ter vindo da
   aplicação.
3. **ID explícito no `Seeder()` é ID fixo** — estável em todos os ambientes e o
   único alvo válido de edição. ID gerado automaticamente é folha: não se edita
   nem se referencia, porque muda de ambiente para ambiente.
4. **Seed é atômico.** Transação única; falha no meio não deixa linha gravada.
5. **`create_table`/`add_column` não podem mentir.** Se o objeto já existe mas
   diverge do declarado, é erro — não `return nil`. O no-op silencioso legitima
   divergência permanente e o checksum passa a bater com o arquivo novo.
6. **Rollback falha alto no irreversível.** Nunca apagar histórico sem desfazer.

## Armadilhas do codebase

- **As migrations não compilam.** Todo arquivo declara `func Migration()` no mesmo
  package. O parser é AST puro, sem type-check — método inexistente no DSL só
  aparece no `migrate run`. Foi a causa de 214 de 267 arquivos quebrados.
  Ao adicionar método ao DSL, adicione **nos dois lugares**:
  [migration/acao/acao.go](migration/acao/acao.go) e o `switch` em
  [internal/migrationgo/parser.go](internal/migrationgo/parser.go).
- **Ordem de execução é só o timestamp do nome do arquivo.** Subpasta por método
  é organização, não ordem.
- **Editar migration já aplicada** dispara drift de checksum. Em ambiente de
  teste, resete. Em produção, migration nova — Oracle e MySQL não têm rollback
  de DDL, então não existe "force seguro" para schema.
- **Cache de schema** (`schemaCache`) substitui ~440 consultas ao catálogo por
  uma. Precisa ser invalidado depois de `raw_sql`, que cria objetos fora do DSL.
- **`loadCatalog` devolve mapa compartilhado em cache** — copie antes de mutar.
- **`Expandir` move a coluna de `Columns` para o campo singular `Column`**, uma
  operação por coluna. Só `create_table` mantém `Columns` preenchido. Ler apenas
  `Columns` faz `add_column` sumir da forma da tabela — foi o que escondeu 103
  colunas do validador das factories e deixava seeds sem coerção de tipo.
- **`tableShapes` é a forma final da tabela**, não o `CreateTable`: aplica
  add/drop/rename de coluna e de tabela na ordem do corpus.
- **Nome físico vem da migration, nunca da factory.** O Postgres guarda
  minúsculas, o MySQL diferencia caixa no nome da tabela, o Oracle dobra para
  maiúsculas. `plano.Fisica()` e `plano.colunaFisica()` existem por isso.
- **`migrate.SQL` é opaco ao AST.** Um rename por SQL cru deixa a forma
  declarada divergente do banco. Onde isso importa (resync de identity), o
  motor confere se a coluna existe antes de usá-la.

## Factories

Mesmo princípio das migrations: o corpo de `Data` é lido por AST e **nunca
compilado**. O motor antigo do projeto de implementação gerava um arquivo com
build tag e chamava `go run` para conseguir executar as closures; isso não
existe mais.

```go
func CidadesFactory() migrate.Factory {
	return migrate.Factory{
		Table: "CIDADES",
		Ruler: migrate.Ruler{Count: 10, Update: true, Active: true},
		Data: func(index int) migrate.Fields {
			local := migrate.FakeLocation()
			return migrate.Fields{
				"cidade_id": migrate.FakeIntIndex(index, 1, 999999999),
				"uf":        local(index, "uf", 2),
				"orgao_id":  migrate.Vinculo("ORGAOS", "ORGAO_ID"),
			}
		},
	}
}
```

O que o avaliador aceita dentro de `Data`, e nada além disso:

- literais (texto, número, `true`, `false`, `nil`, negativos)
- o identificador do parâmetro de índice
- as funções de [migration/fake.go](migration/fake.go) **registradas** em
  [internal/factorygo/vocabulario.go](internal/factorygo/vocabulario.go)
- `migrate.Vinculo("TABELA", "COLUNA")`
- um único `nome := migrate.FakeLocation()` antes do `return`

A closure existe só para dar significado a `index` — sem ela o arquivo não
compilaria, e ele precisa continuar sendo Go válido para o editor e o
`go build ./...` do projeto.

Invariantes:

1. **Toda função `Fake*` é determinística no índice.** É o que permite comparar
   o resultado entre os quatro bancos. As exceções são `FakeUnique*` e
   `FakeHash*`, que precisam variar entre execuções para não colidir com dado
   já gravado.
2. **Função nova em `fake.go` só existe para as factories depois de registrada
   no vocabulário.** Não há resolução de símbolo pelo compilador.
3. **Factory limpa a tabela antes de inserir** — é dado descartável, ao
   contrário do seed. Por isso pedir uma tabela traz os **pais** (senão a FK
   barra o INSERT) e as **filhas** (senão a FK barra o DELETE).
4. **A ordem vem do grafo de FK do corpus em AST**, não do catálogo do banco:
   é a mesma nos quatro dialetos e funciona antes de o banco existir. Ciclo em
   schema legado é rompido no ponto de menor dependência, não aborta.
5. **`factory create` preserva o que já está escrito.** Mantém a expressão de
   cada coluna existente e o `Ruler`; só acerta a lista de colunas contra a
   migration. `Update: false` congela o arquivo inteiro.
6. **Regra específica do projeto vai em `factory.expressions.mappers`** no
   `gokit.json`, não em código do plugin.

```bash
gokit factory validate          # confere contra as migrations, sem tocar no banco
gokit factory create [tabela]   # gera/atualiza a partir do corpus
gokit factory run [tabela...]   # popula; sem argumento, todas as ativas
```

## Onde as coisas ficam

| | |
|---|---|
| DSL público | [migration/migrate.go](migration/migrate.go) |
| Tipos e validação | [migration/acao/acao.go](migration/acao/acao.go) |
| Parser AST de migration | [internal/migrationgo/parser.go](internal/migrationgo/parser.go) |
| Executor, validate, rollback | [internal/migraterun/runner.go](internal/migraterun/runner.go) |
| Motor de seed | [internal/migraterun/seed.go](internal/migraterun/seed.go) |
| Biblioteca de dados fake | [migration/fake.go](migration/fake.go) |
| Tipos de factory | [migration/factory.go](migration/factory.go) |
| Avaliador AST de factory | [internal/factorygo/parser.go](internal/factorygo/parser.go) |
| Vocabulário aceito | [internal/factorygo/vocabulario.go](internal/factorygo/vocabulario.go) |
| Executor de factory | [internal/migraterun/factory.go](internal/migraterun/factory.go) |
| Gerador de factory | [internal/migraterun/factorycreate.go](internal/migraterun/factorycreate.go) |
| Menu TUI | [internal/tui/tui.go](internal/tui/tui.go) |
| Config e conexões | [internal/config/config.go](internal/config/config.go) |
| Laboratório de teste | [testlab/lab.ps1](testlab/lab.ps1) |

Adicionar uma operação nova toca, no mínimo: constante em `acao`, caso em
`Validar`, construtor em `migrate.go`, caso no `switch` do parser, caso em
`executeOperation`, caso em `rollbackSQL`. Esquecer o `rollbackSQL` faz a
operação cair no default e virar erro no rollback.

## Convenções

- Código, comentários e mensagens de erro em **português**.
- Mensagem de erro diz o que aconteceu **e** como resolver — o usuário lê no
  meio de uma migration de 267 arquivos.
- Toda mudança roda `go test ./...` e `.\testlab\lab.ps1 matrix` antes de fechar.


Projeto de teste em implementacao: C:\Users\phelipe.viana\Desktop\prevcontas_test\
Projeto do plugin gokit: C:\Users\phelipe.viana\Desktop\gokit\
Referencia do projeto em producao: C:\Users\phelipe.viana\Documents\TCE_MT\back