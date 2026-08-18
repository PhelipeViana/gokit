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
mostre a saída.

## Ciclo de trabalho

Há **dois** laboratórios, e eles usam contêineres diferentes:

**1. Corpus legado de 754 tabelas** — é onde as divergências entre dialetos aparecem.
Contêineres no compose do `teste/`; corpus vem do `sisprev_homologacao/`.

```bash
cd teste && docker compose up -d mysql postgres oracle mssql
cd ../gokit && GOOS=windows GOARCH=amd64 go build -ldflags "-X main.CommitHash=development" \
  -o "../sisprev_legado/gokit.exe" .
cd ../sisprev_legado
# trocar de dialeto: DB_DIALECT no .env (mysql | sqlserver | oracle | postgres)
./gokit.exe migrate validate && ./gokit.exe migrate run && ./gokit.exe seed run
```

**Zerar o banco entre medições, e CONFERIR que zerou.** Migrate não é destrutivo, então
tabela sobrando da execução anterior faz o `create_table` passar calado (ver invariante 5)
e o run falha adiante, culpando o objeto errado. Já custou três diagnósticos falsos —
todos porque a saída do reset foi para `/dev/null` e o "zerado" foi impresso sem medir.

**2. `testlab/lab.ps1`** — corpus pequeno, regressão de contrato de seed.

⚠ **QUEBRADO hoje, por dois motivos anteriores a qualquer mudança de motor:**

- `prevcontas_test/internal/gokit/gokit.json` tem `"migrate": "internal/gokit/migrations"`,
  pasta que **não existe** — os 271 arquivos do corpus estão em `database/migrations`.
  O gokit não acha o corpus, então nenhum cenário do contrato chega a executar.
- Dentro do `lab.ps1` o nome do contêiner chega vazio ao `docker exec` (a mensagem sai como
  `No such container: mysql`, com o cliente ocupando o lugar do contêiner). `docker exec`
  chamado à mão com os mesmos contêineres funciona.

Os contêineres dele (`prevcontas_test_*`, portas 1599/5499/3399/1499) sobem com
`docker compose up -d oracle postgres mysql sqlserver` dentro de `prevcontas_test/`.
Enquanto os dois pontos acima não forem corrigidos, **medir pelo laboratório 1**.

```powershell
.\testlab\lab.ps1 matrix       # build + validate + limpo + idempotência nos 4
.\testlab\lab.ps1 contract     # contrato de seed: soberania, atomicidade, sequência
.\testlab\lab.ps1 sql all "SELECT ..."
.\testlab\lab.ps1 reset oracle
```

O equivalente manual:
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
| Teto de colunas na chave do índice | 32 | 32 | **16** | 32 |
| Literal booleano (`TRUE`/`FALSE`) | **não** (NUMBER(1)) | sim | sim | **não** (BIT) |
| `boolean DEFAULT 0` | aceita | **recusa** | aceita | aceita |
| `DATE DEFAULT CURRENT_TIMESTAMP` | aceita | aceita | **recusa** | aceita |
| Teto de bytes na definição da linha | — | — | **65.535** | — |
| FK de coluna para ela mesma | aceita | aceita | **recusa** | aceita |

Outros pontos concretos:

- **O apelido é indireção proposital, e é ela que segura a integridade do corpus.**
  `core.Table.X` vale o APELIDO, não o nome físico, e `advanceAliases` move o
  apelido no `RenameTable` — então migration escrita hoje continua correta depois
  de a tabela física ser renomeada. Quem mexer aqui não deve "consertar" isso.
- **Mas o apelido é resolvido por OPERAÇÃO, não por arquivo.** `advanceAliases` roda
  dentro do laço de execução: um `CreateIndex` sobre a tabela que o `CreateTable` do
  mesmo arquivo acabou de criar precisa resolver o apelido. Antes avançava no fim do
  arquivo, e o validador — que já observava por operação — aprovava o que o
  `migrate run` recusava. Fica latente com tabela de nome único, porque aí apelido e
  nome físico coincidem.
- **Índice se declara por operação, não na coluna.** `migrate.CreateIndex(tabela,
  nome, colunas...)` é a única forma; o `.Index()` da coluna foi removido porque
  era lido num lugar só e produzia índice **apenas no MySQL** — nos outros três não
  fazia nada. O nome explícito também importa: o Oracle trunca identificador em 30
  e o nome de índice é único por schema nele e no SQL Server.
- **`.Unique()` na coluna, ao contrário, vale nos quatro** — vira `UNIQUE` inline no
  `columnDefinition`, e alimenta a ORM, a documentação e a validação de factory.
- **Coluna `AUTO_INCREMENT` fora da chave não precisa de cuidado no corpus:** o
  executor emite uma `UNIQUE KEY` própria para ela quando o dialeto é MySQL, que é
  o único que exige a identidade indexada. O mesmo corpus vale nos quatro.

- **Teto de colunas na CHAVE do índice existe nos QUATRO**, com tetos diferentes:
  MySQL **16**, os outros três 32. Isto já foi escrito como "só o Oracle recusa" —
  era suposição, e o efeito era o corpus morrer no primeiro índice largo do próprio
  dialeto de origem. Índice comum acima do teto é tolerado (é desempenho perdido);
  índice **único** não, porque ali a unicidade é integridade.
- **`DEFAULT 0` em coluna `boolean` quebra o Postgres** — ele não faz a coerção de
  integer para boolean. Normalizado pelo TIPO da coluna, nunca pela expressão: em
  coluna numérica o 0 é um número. E cuidado: **T-SQL não tem literal booleano**,
  então `BIT DEFAULT FALSE` é "Invalid column name 'FALSE'" — no SQL Server tem de
  voltar a 0/1, como no Oracle.
- **`DATE DEFAULT CURRENT_TIMESTAMP` só o MySQL recusa** ("Invalid default value").
  A intenção é a data de hoje, então a forma só-data é a fiel nos quatro. Mesma
  regra do booleano: decide pelo tipo da coluna.
- **MySQL limita a DEFINIÇÃO da linha a 65.535 bytes**, somando todas as colunas —
  é limite de formato, `ROW_FORMAT` não resolve. Quando não cabe, o executor promove
  as colunas de texto mais largas para TEXT, da mais larga para a mais estreita, e
  para quando cabe. Não promove coluna com `DEFAULT` (TEXT não aceita) nem coluna
  **indexada** (índice em TEXT exige prefixo) — e a lista de indexadas vem do corpus
  INTEIRO, porque o índice pode estar numa migration posterior ao `create_table`.
- **FK de uma coluna para ELA MESMA na própria tabela é pulada nos quatro.** Não
  restringe nada; o SQL Server aceita e guarda a constraint inútil, o MySQL recusa
  com "Missing unique key for constraint" — mensagem que manda procurar uma chave
  única que existe. Auto-referência legítima (`pai_id → id`) **não** entra nisso.
- **Ler o banco só funciona em projeto EM BRANCO.** `migrate import` recusa se o
  corpus já tem qualquer `.go`: importar por cima mistura duas gerações do leitor no
  mesmo lugar e depois não há como atribuir uma divergência à origem ou à versão que
  escreveu. Reimportar = projeto novo + diff entre os dois corpora.
- **Coluna de `INCLUDE` do SQL Server NÃO é chave de índice.** A consulta precisa de
  `ic.is_included_column = 0`. Sem o filtro elas entram na lista e, pior, vêm
  **primeiro** (`key_ordinal = 0`), então a chave real vai para o fim ou desaparece.
  Custou 375 colunas de chave fantasmas em 29 tabelas do corpus legado.
- **Método removido do DSL precisa dizer o que fazer.** O reconhecimento do método
  vem ANTES de resolver a referência de tabela; sem isso, `migrate.CreateView(...)`
  de um corpus antigo era diagnosticado como "exige core.Table.*" — mandando
  corrigir o primeiro argumento de um método que não existe mais.

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
   reemissão não. Medido nos quatro: depois de semear IDs 1–10, o próximo é 11.
2. **O seeder é SOBERANO sobre os dados.** Onde existe seeder, o seeder manda:
   linha idêntica = no-op; linha divergente = **sobrescrita**, com ou sem chave
   primária. Tabela **sem chave** tem o conteúdo substituído por inteiro — sem
   chave não há como casar linha declarada com existente.
   Isto **substituiu** o invariante anterior ("seed nunca sobrescreve linha que não
   é dele; divergente é erro"), por decisão do usuário e com o custo à vista:
   dado da aplicação naquela linha é perdido. Daí a regra que sobra —
3. **Nenhuma perda em silêncio.** Toda sobrescrita sai com tabela, chave, coluna e
   os DOIS valores; toda substituição de tabela sem chave sai com quantas linhas
   foram apagadas e inseridas. As duas listas viram notificação `warning`. Um
   seeder **vazio** continua sendo recusado: substituir por nada apagaria a tabela,
   e isso não pode acontecer por omissão.
4. **Seed é atômico.** Transação única; falha no meio não deixa linha gravada.
   Medido: seed que insere 6 e falha na 7ª deixa a tabela como estava. Por isso a
   limpeza da tabela sem chave usa `DELETE` e não `TRUNCATE` — TRUNCATE é DDL em
   três dos quatro bancos e sairia da transação.
5. **`create_table`/`add_column` não podem mentir.** Objeto que já existe é CONFERIDO
   contra a forma declarada, não ignorado. Divergência é erro alto, com as duas
   formas na mensagem.
   O que é comparado, e o que NÃO é, tem razão medida:
   - **presença** da coluna declarada — a checagem com dentes, sem ambiguidade;
   - **tipo em bucket grosso** (`numerico`/`textual`/`data`/`temporal`/`binario`),
     porque a ida DSL → tipo físico não é injetiva: `boolean` vira NUMBER no Oracle e
     `tinyint` no catálogo do MySQL, `text` vira CLOB no Oracle e NVARCHAR(MAX) no SQL
     Server. Os DOIS lados passam por `columnTypeSQL` + `familiaDoTipo`, senão a
     comparação acusaria toda tabela que o próprio gokit criou;
   - **nulidade não é comparada:** o SQL Server não promove coluna de PK a NOT NULL, e
     o cache de schema não guarda nulidade — errar aqui barraria migration correta;
   - **tamanho menor no banco é aviso**, porque o leitor promove texto acima de 4000
     para `Text()` e o tamanho difere legitimamente após uma volta pelo import;
   - **coluna a mais no banco não é divergência:** a declaração não mentiu sobre ela.
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
		Table: core.Table.Cidades,
		Ruler: migrate.Ruler{Count: 10, Active: true},
		Data: migrate.Fields{
			core.Column.Cidades.CidadeId: migrate.FakeInt(1, 999999999),
			core.Column.Cidades.Uf:       migrate.FakeUF(),
			core.Column.Cidades.OrgaoId:  migrate.Reference(core.Table.Orgaos),
		},
	}
}
```

O que o avaliador aceita dentro de `Data`, e nada além disso:

- literais (texto, número, `true`, `false`, `nil`, negativos)
- as funções de [migration/fake.go](migration/fake.go) **registradas** em
  [internal/factorygo/vocabulario.go](internal/factorygo/vocabulario.go) —
  **sem índice na chamada**, ver abaixo
- `migrate.Reference(tabela)` — pega um valor que já existe na coluna do pai; a
  coluna é opcional porque a FK já a declara
- `migrate.Seeder(tabela, 1, 2, 15)` — restringe a IDs específicos, percorridos na
  ordem declarada pelo índice. Número ou texto dá no mesmo. Se nenhum existir no
  pai, a restrição é ignorada em silêncio e vale o padrão

`Data` é um **mapa**, não uma closure. Era `func(index int) migrate.Fields`, porque
o arquivo escrevia `index` dentro do mapa e o parâmetro precisava existir para o
arquivo ser Go válido; com o índice implícito ninguém mais o escreve, e o invólucro
deixou de servir ao compilador e ao motor. Não existe statement dentro de `Data` —
o acessor `local := migrate.FakeLocation()` era o único, e saiu com ele.

Coerência de localidade **não** se perdeu: `FakeCity`, `FakeUF`, `FakeState`,
`FakeCityCode`, `FakeDistrict`, `FakeStreet` e `FakeCountry` derivam todas do mesmo
`fakeLocationIndex(index)`, então a linha continua saindo "Cuiaba/MT" em vez de
"Cuiabá/SP". Medido antes de remover o acessor, e travado por teste.

**O índice da linha é implícito.** Ele não aparece na chamada: o motor sabe em que
linha está e injeta ao avaliar. São 34 nomes, um por conceito, e `length` é sempre
escrito (0 é sem limite). O desenho anterior tinha 72 nomes, cada conceito em dois
sabores — `FakeName()` e `FakeNameIndex(index, …)` —, e o curto devolvia
**constante**: três das sete colunas de `pedidos` saíam idênticas nas dez linhas
sem nenhum aviso.

Correspondência para quem lê arquivo antigo: `FakeXIndex`/`FakeXIndexLength` →
`FakeX(length)`. Os nomes longos continuam existindo em `fake.go` como camada de
implementação, mas **não estão no vocabulário** — escrevê-los numa factory dá erro
com a sugestão do nome novo. Chamada de verdade, fora do gokit, a função pública
devolve o valor da PRIMEIRA linha; é uma regra só, válida para todas.

Invariantes:

1. **Toda função `Fake*` é determinística no índice E varia por linha.** O
   determinismo é o que permite comparar o resultado entre os quatro bancos; a
   variação é o que faz o dado servir de teste.
   [variacao_test.go](internal/factorygo/variacao_test.go) exercita os 34 nomes e
   falha se algum devolver o mesmo valor nas dez linhas — ou se um nome novo entrar
   no vocabulário sem chamada de teste.
   Duas exceções, ambas documentadas no arquivo: `FakeUnique*` varia entre
   EXECUÇÕES em vez de por linha (senão colide com documento já gravado), e
   `FakeUserAgent`/`FakeHashPassword`/`FakeValue` são constantes por mérito.
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
   migration. Não há como congelar o arquivo: acertar a lista de colunas é
   obrigatório, senão a factory cita coluna que a tabela não tem mais.
6. **Factory não popula tabela que já tem dados.** Ela é simulador de dados de
   teste; dado real (ou de seed) não é sobrescrito sem `--force`. É também o que
   mantém os IDs do seed vivos para o `Seeder` encontrar.
7. **Regra específica do projeto vai em `factory.expressions.mappers`** no
   `gokit.json`, não em código do plugin.

```bash
gokit factory validate          # confere contra as migrations, sem tocar no banco
gokit factory create [tabela]   # gera/atualiza a partir do corpus
gokit factory run [tabela...]   # popula; sem argumento, todas as ativas
```

Todas as factories moram num **arquivo único**, `internal/gokit/factory/factories.go`,
uma função por tabela. Não há regra implícita no fatiamento por tabela, e o
`Active` é por FUNÇÃO — conviver no mesmo arquivo não acopla nada. A leitura
aceita qualquer `.go` da pasta com várias factories dentro, então uma organização
manual continua funcionando; o gerador é que consolida (e apaga os
`<tabela>_factory.go` que consolidou, senão seria função duplicada no pacote).

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
| Leitura do banco (catálogos) | [internal/migraterun/dbscan.go](internal/migraterun/dbscan.go) |
| Escrita banco → migrations | [internal/migraterun/dbimport.go](internal/migraterun/dbimport.go) |
| Tipo do banco → método do DSL | [internal/migraterun/dbtipo.go](internal/migraterun/dbtipo.go) |
| Guarda "só projeto em branco" | [internal/migraterun/corpusvazio.go](internal/migraterun/corpusvazio.go) |
| Conferência de forma (inv. 5) | [internal/migraterun/conferirforma.go](internal/migraterun/conferirforma.go) |
| FK e tolerâncias de índice | [internal/migraterun/foreignkey.go](internal/migraterun/foreignkey.go) |
| Teto de linha do MySQL | [internal/migraterun/mysqllinha.go](internal/migraterun/mysqllinha.go) |
| Rastro da soberania do seeder | [internal/migraterun/seedsoberano.go](internal/migraterun/seedsoberano.go) |
| Mapper de view/func/proc | [internal/migraterun/specialmap.go](internal/migraterun/specialmap.go) |
| Catálogos core.Table/core.Column | [internal/migrationgo/catalog.go](internal/migrationgo/catalog.go) |
| Ações da área ORM | [internal/migraterun/ormacoes.go](internal/migraterun/ormacoes.go) |
| Menu TUI | [internal/tui/tui.go](internal/tui/tui.go) |
| Config e conexões | [internal/config/config.go](internal/config/config.go) |
| Laboratório de teste | [testlab/lab.ps1](testlab/lab.ps1) |

Adicionar uma operação nova toca, no mínimo: constante em `acao`, caso em
`Validar`, construtor em `migrate.go`, caso no `switch` do parser, **entrada em
`metodoDeTabela`** (`internal/migrationgo/parser.go`), caso em `executeOperation`,
caso em `rollbackSQL`.

- Esquecer o `rollbackSQL` faz a operação cair no default e virar erro no rollback.
- Esquecer o `metodoDeTabela` faz a operação ser recusada como **inexistente**: esse
  mapa é conferido ANTES de resolver a referência de tabela, justamente para que
  método que não existe não seja diagnosticado como "exige core.Table.*".

**View, function e procedure NÃO estão no corpus de migrations.** Saíram do DSL — o
gokit não as cria, ele as MAPEIA do banco com `gokit special`, e o banco é a verdade.
Um corpus antigo com `migrate.CreateView(...)` recebe uma mensagem que aponta o
caminho novo, via `metodoRemovido`.

## Convenções

- Código, comentários e mensagens de erro em **português**.
- Mensagem de erro diz o que aconteceu **e** como resolver — o usuário lê no
  meio de uma migration de 267 arquivos.
- Toda mudança roda `go build ./...`, `go vet ./...` e `go test ./...` antes de fechar,
  mais a medição contra o corpus legado nos quatro dialetos quando toca o executor.
  O `lab.ps1 matrix` está fora de serviço — ver o Ciclo de trabalho.
- **Medir, não supor** — vale também para o próprio ferramental. `echo "ok"` encadeado
  depois de um `head`, ou um reset com a saída em `/dev/null`, imprime sucesso com o
  comando tendo falhado. Leia o código de saída do comando que importa, e confira o
  estado do banco em vez de confiar na mensagem que você mesmo imprimiu.
- **Não deixar máquina morta.** Função que ninguém chama, chave i18n órfã e parâmetro
  ignorado voltam a ser usados por engano — e `go vet` não pega nenhum dos três.


## Onde os projetos ficam

Tudo sob `C:\Users\phelipe.viana\Documents\PROJETO_GO_KIT\`:

| pasta | papel |
|---|---|
| `gokit/` | o plugin |
| `sisprev_homologacao/` | **oficial de teste** — 754 tabelas do SISPREV legado, importado do banco |
| `sisprev_legado/` | laboratório dos 4 dialetos sobre o corpus oficial (não copia o corpus) |
| `teste/` | projeto pequeno e descartável; **os 4 contêineres vivem no compose dele** |
| `prevcontas_test/`, `normaliza_lab/`, `aplica_oracle/` | labs antigos, descartáveis |

Referência de produção, **não trabalhar dentro**:
`C:\Users\phelipe.viana\Documents\TCE_MT\back`

Os quatro bancos sobem com `docker compose up -d mysql postgres oracle mssql` **dentro
de `teste/`** (mysql 20590, postgres 24443, oracle 22889, mssql 28401). Trocar de dialeto
é `DB_DIALECT` no `.env` do projeto — variável de ambiente do host não passa.

**Nunca rodar `migrate run` ou `seed run` contra a conexão `proxima`**: é o PROXIMA-BD, um
servidor de produção com ~300 bancos, e o acesso ali é somente leitura de catálogo.
