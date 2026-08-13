# GoKit

Plugin em Go para **migrations declarativas e ORM cross-dialect**. O mesmo corpus
roda em **Oracle, PostgreSQL, MySQL e SQL Server** — a autoria é uma só, e o
dialeto entra apenas na compilação.

As migrations são lidas por **AST**: o gokit nunca compila nem executa o arquivo
de migration. A camada de consulta é **gerada** a partir delas, então o schema
que o banco tem e o que o Go conhece saem da mesma fonte.

---

## Índice

- [Estado e validação](#estado-e-validação)
- [A camada gerada: um import](#a-camada-gerada-um-import)
- [Casos de uso](#casos-de-uso)
  - [Leitura](#leitura)
  - [Escrita](#escrita)
  - [Erros tipados](#erros-tipados)
  - [Transação](#transação)
  - [Agrupamento e relatório](#agrupamento-e-relatório)
  - [Junções e subconsultas](#junções-e-subconsultas)
  - [Escala e concorrência](#escala-e-concorrência)
  - [Upsert](#upsert)
  - [Expressões cross-dialect](#expressões-cross-dialect)
  - [Raw contido](#raw-contido)
  - [A mesma regra no banco e em memória](#a-mesma-regra-no-banco-e-em-memória)
- [Modificações](#modificações)
- [Convenções](#convenções)
- [Comandos](#comandos)

---

## Estado e validação

A ORM está **completa** nos seis blocos do escopo: escrita e chave, leitura
avançada, junções e subconsultas, escala e concorrência, expressões
cross-dialect e `Raw` contido.

Nada entra sem passar pelas duas baterias, que rodam a **mesma autoria** contra
os quatro bancos reais e comparam o resultado:

```bash
cd teste && go run ./cmd/conform    # 55 casos de leitura  · 0 divergências
cd teste && go run ./cmd/escrita    # 78 casos de escrita  · 0 divergências + 1 esperada
cd teste && go run ./cmd/ormdemo    # catálogo executável  · 22 seções
```

A divergência “esperada” é uma só, decidida e documentada: o `LIKE` cru segue a
*collation* da coluna (sensível à caixa no Postgres e no Oracle, insensível no
MySQL e no SQL Server). Quem precisa da mesma resposta nos quatro usa
`orm.Lower(col)`. A bateria a marca com `≈`, não derruba o status de saída, e
uma divergência **nova** continua marcando `✗` — se toda divergência conhecida
contasse como falha, o sinal seria sempre vermelho e o problema novo passaria
batido.

---

## A camada gerada: um import

Tudo que o gokit gera para a aplicação mora em **`internal/gokit/core`**, num
pacote só:

| arquivo | o que é | de onde vem |
|---|---|---|
| `entities.gen.go` | entidades: `core.Users` com `Model`, `Column`, `Relation` | schema atual das migrations |
| `table.gen.go` | `core.Table.Users` — identidade **física**, para as migrations | catálogo acumulado |
| `view.gen.go` | `core.View.X` — o mesmo, para views | catálogo acumulado |
| `response.gen.go` | envelope JSON (`data`/`meta`/`error`) | template |

Os dois lados da mesma tabela têm nomes distintos porque são coisas distintas:

```go
// migration — identidade física da tabela
migrate.AddColumn(core.Table.Users,
    migrate.Col("cidade_id").Integer().Nullable().References("cidades", "id"),
)

// aplicação — entidade de consulta
linhas, err := core.Users.Where(core.Users.Column.Ativo.IsTrue()).Get(ctx)
```

> **Por que dois geradores e não um.** O catálogo **acumula e nunca remove**; as
> entidades refletem o schema **atual** e desaparecem no `DropTable`. Como a
> migration de drop cita o nome da tabela que derrubou, derivar o lado físico das
> entidades faria essa própria migration parar de compilar. A separação é
> obrigatória — o pacote único é que é conveniência.

---

## Casos de uso

Os exemplos assumem o handle gerado e o filtro da entidade:

```go
import (
    core "seu-modulo/internal/gokit/core"
    "github.com/PhelipeViana/gokit/orm"
)

u, f := core.Users, core.Users.Column
```

A conexão é resolvida uma vez, no boot, e vira a conexão padrão — a autoria
nunca passa dialeto nem schema:

```go
sess, err := gokitdb.Connect(ctx)
defer sess.Close()
```

Todo terminal tem a variante `*With`, que recebe um `Runner` explícito
(transação, outro banco): `Get`/`GetWith`, `Insert`/`InsertWith`, e assim por
diante.

### Leitura

```go
total, _   := u.Count(ctx)
existe, _  := u.Where(f.Email.EndsWith("@example.com")).Exists(ctx)
linha, err := u.Find(ctx, 7)                       // por chave primária
ids, _     := u.Where(f.Id.LessOrEqual(5)).PluckInt(ctx, f.Id)

pagina, _ := u.OrderBy(f.Id).Paginate(ctx, 2, 15)  // Items, Total, Page, Size, Pages
```

Condições combinam num `Where` só, com a precedência explícita nos combinadores:

```go
u.Where(
    f.Ativo.IsTrue(),
    orm.Or(f.Uf.Equal("MT"), f.Uf.Equal("SP")),
)
// WHERE ativo = ? AND (uf = ? OR uf = ?)
```

Relações são carregadas por `With`, com aninhamento e projeção no mesmo
parêntese — sem N+1:

```go
users, _ := u.With(u.Relation.Cidade(core.Cidades.Relation.Estado())).Get(ctx)
```

O catálogo completo da leitura está em
[docs/orm-casos-de-uso.md](docs/orm-casos-de-uso.md).

### Escrita

O valor é checado pelo compilador: o tipo da coluna decide o que o método aceita.

```go
res, err := u.Insert(ctx, orm.Set(
    f.Nome.Is("Ana"),                 // só string
    f.Email.Is("ana@exemplo.com"),
    f.Saldo.Is(10.5),                 // número (int ou float)
    f.Ativo.Is(true),                 // só bool
    f.Nascimento.Is("1990-05-17"),    // data em qualquer forma conhecida
    f.CidadeId.SetNull(),             // NULL explícito
))
res.Affected  // linhas afetadas
res.LastID    // chave gerada, quando o banco a fornece
```

```go
f.Nome.Is(123)      // não compila
f.Ativo.Is("talvez") // não compila
```

A tipagem é a mesma que os **filtros** já adotaram, e isso é deliberado: texto e
booleano são tipados; número aceita `int` e `float` sem conversão na autoria;
data passa pelo `DateValue`. Divergir disso deixaria a escrita mais restrita que
o filtro, sem ganho — e Go não permite parâmetro de tipo em método, então uma
restrição numérica genérica não está disponível.

Duas guardas que só a forma tipada consegue dar:

- **coluna atribuída duas vezes** falha. Numa literal de mapa com chave não
  constante, o Go aceita a repetição e fica com a última **em silêncio** —
  `orm.Values{f.Nome: "a", f.Nome: "b"}` compila e grava `"b"`.
- **`SetNull()` em coluna `NOT NULL`** falha na chamada, dizendo para omitir a
  coluna se a intenção é o padrão do banco. Antes só quebrava no `INSERT`, com o
  erro do driver.

`orm.Values` continua existindo para o caso **dinâmico**, em que a lista de
colunas é decidida em runtime (importação, formulário genérico) — mesmo papel do
`Record.ByName`: a saída existe, e o nome diz que ali se abre mão da checagem.

```go
u.Insert(ctx, orm.Values{f.Nome: valorVindoDeFora})
```

Nos dois casos a compilação **ordena pela posição da coluna na entidade** — mapa
em Go itera aleatório, e SQL instável impede comparar dialetos.

```go
u.Where(f.Id.Equal(7)).Update(ctx, orm.Set(f.Nome.Is("Ana Maria")))
u.UpdateByKey(ctx, orm.Set(f.Nome.Is("Ana Maria")), 7)

u.Where(f.Id.Equal(7)).Delete(ctx)
u.DeleteByKey(ctx, 7)

u.InsertMany(ctx, []orm.Values{
    orm.Set(f.Nome.Is("Ana")),
    orm.Set(f.Nome.Is("Bia")),
})
```

`UPDATE` e `DELETE` **sem `Where` falham** com `orm.ErrMissingWhere`. Quem quer
mesmo a tabela inteira diz isso em voz alta:

```go
u.Unrestricted().Delete(ctx)
```

A chave gerada é o ponto em que os quatro bancos mais divergem, e quem escreve
não vê nada disso: MySQL usa `LastInsertId`, Postgres `RETURNING`, SQL Server
`OUTPUT INSERTED` (que precisa ficar entre a lista de colunas e o `VALUES`) e
Oracle `RETURNING INTO` com parâmetro de saída.

Chave primária **composta** e com nome diferente de `id` são o caso normal em
schema legado, e funcionam: `u.Find(ctx, 10, "MAT1")`.

### Erros tipados

É o que permite a camada HTTP responder 409 ou 422 sem inspecionar texto de
driver:

```go
_, err := u.Insert(ctx, orm.Values{f.Email: emailJaExistente})

switch {
case errors.Is(err, orm.ErrDuplicate):  // 409
case errors.Is(err, orm.ErrForeignKey): // 422
case errors.Is(err, orm.ErrNotNull):
case errors.Is(err, orm.ErrCheck):
case errors.Is(err, orm.ErrTooLong):
}
```

A causa original fica acessível por `errors.Unwrap`. As classes são idênticas
nos quatro bancos — a mensagem do driver difere, a classe não.

### Transação

```go
err := sess.Tx(ctx, func(r orm.Runner) error {
    if _, err := u.InsertWith(ctx, r, orm.Values{f.Nome: "Ana"}); err != nil {
        return err   // devolver erro desfaz tudo
    }
    return nil
})
```

Savepoint aninhado, para a operação que pode falhar sem levar a transação
inteira:

```go
tx := r.(*gokitdb.TxSession)
_ = tx.Attempt(ctx, "sp_import", func(rr orm.Runner) error { ... })
```

> No **Postgres** qualquer erro aborta a transação inteira; nos outros três a
> transação continua. O savepoint é o que iguala o comportamento.

### Agrupamento e relatório

A projeção agrupada não tem a forma da linha da entidade, então o terminal é
`Rows`, que devolve `[]Record`:

```go
totalSaldo := orm.Sum(f.Saldo).As("total")

linhas, _ := u.Select(f.CidadeId, totalSaldo).
    GroupBy(f.CidadeId).
    Having(orm.CountAll().GreaterThan(10)).
    Rows(ctx)

for _, g := range linhas {
    g.Int(f.CidadeId)      // getters recebem a COLUNA, não texto
    g.Float(totalSaldo)    // a agregação guardada em variável é a chave de leitura
}
```

Não existe um `Select` separado para agregação: `Select` decide pelo conteúdo.
`Get` numa projeção agregada devolve `orm.ErrAggregateProjection` apontando o
`Rows`, em vez de descartar a agregação em silêncio.

Agregações escalares seguem diretas: `u.Sum(ctx, f.Saldo)`, `Avg`, `Min`, `Max`
— inclusive sobre data e texto.

### Junções e subconsultas

`Join` é **pela relação declarada**, nunca por tabela crua: o `ON` sai dos
metadados que o gerador preencheu. Junção invertida devolve linhas erradas em
silêncio, e escrever `ON a.id = b.a_id` à mão é onde isso nasce.

```go
u.Join(u.Relation.Cidade()).Where(f.CidadeId.IsNotNull()).Count(ctx)
u.LeftJoin(u.Relation.Cidade()).Count(ctx)
```

> `With` **carrega** o destino em consulta separada e devolve a árvore. `Join`
> não carrega nada — existe para filtrar e ordenar pela outra tabela na mesma
> consulta.

Comparação entre colunas e subconsulta:

```go
u.Where(orm.WhereColumn(f.Saldo, orm.GreaterThan, f.Limite))

sub := core.Cidades.Where(core.Cidades.Column.Uf.Equal("PR")).
    Select(core.Cidades.Column.Id)

u.Where(orm.InQuery(f.CidadeId, sub))        // NotInQuery, ExistsQuery, NotExistsQuery
```

Filtrar o pai pela existência do filho (`EXISTS` correlacionado):

```go
u.Where(u.Relation.PedidosByUser().Where(pf.Entregue.IsTrue()).Exists())
u.Where(u.Relation.PedidosByUser().DoesntExist())
```

### Escala e concorrência

`Chunk` pagina **por chave** (`WHERE id > último`), não por `OFFSET`: com
`OFFSET`, escrita concorrente desloca as páginas e o percurso pula ou repete
linha sem avisar.

```go
err := u.Chunk(ctx, 500, func(lote []core.UsersRow) error { ... })
err := u.Each(ctx, func(linha core.UsersRow) error { ... })   // lotes de 1000
```

Trava de linha, para ler-decidir-gravar dentro de transação:

```go
linhas, err := u.Where(f.Id.Equal(id)).Lock().GetWith(ctx, r)
```

> `FOR UPDATE` no MySQL, Postgres e Oracle; `WITH (UPDLOCK, ROWLOCK)` no SQL
> Server — e a **posição no comando é diferente**. No Oracle, trava combinada
> com limite é recusada pelo banco (ORA-02014), então o gokit falha antes com
> `orm.ErrLockWithLimitOnOracle`, dizendo para restringir pela chave.

### Upsert

Gravar sem saber se a linha já existe. É a operação de forma mais diferente
entre os quatro: dois estendem o `INSERT`, dois trocam o comando por `MERGE`.

```go
u.Upsert(ctx, orm.Values{
    f.Email: "ana@exemplo.com",   // a chave de conflito
    f.Nome:  "Ana",
    f.Saldo: 99.5,
}, f.Email)

u.InsertIgnore(ctx, orm.Values{f.Email: email, f.Nome: "Ana"}, f.Email)
```

`Affected` distingue os desfechos nos quatro dialetos: **1** quando inseriu ou
atualizou, **0** quando o `InsertIgnore` preservou a linha existente. (O MySQL
conta a atualização como duas linhas; o motor normaliza.)

Duas operações, porque são duas intenções, e confundi-las é perda de dado
silenciosa: `Upsert` atualiza a linha existente, `InsertIgnore` a preserva.
Nenhuma das duas devolve `LastID` — numa escrita que pode não inserir, “a chave
gerada” não existe.

| dialeto | forma |
|---|---|
| MySQL | `INSERT … ON DUPLICATE KEY UPDATE` |
| Postgres | `INSERT … ON CONFLICT (chave) DO UPDATE SET … EXCLUDED.col` |
| Oracle | `MERGE INTO … USING (SELECT :1 … FROM dual) …` |
| SQL Server | `MERGE INTO … USING (VALUES (@p1, …)) … ;` |

> **Ressalva do MySQL:** ele não sabe restringir o conflito a uma chave
> específica — colidir em qualquer unicidade dispara a atualização. Nos outros
> três, só a chave informada.
>
> O `InsertIgnore` **não** usa `INSERT IGNORE`: aquele rebaixa `NOT NULL`,
> truncamento e FK a aviso e grava a linha errada. Só o conflito de chave é
> ignorado; erro de escrita continua falhando.

### Expressões cross-dialect

É o bloco que mais elimina `Raw`, porque é aqui que o nome da função muda de
banco para banco. `Expr` implementa `Column`, então entra em `Where`, `Select`,
`OrderBy` e `GroupBy` sem método novo em nenhum deles — e compõe.

```go
u.Where(orm.Lower(f.Nome).Contains("ana"))            // mesma resposta nos 4
u.OrderBy(orm.Lower(f.Nome))
u.Where(orm.Coalesce(f.Saldo, 0).GreaterOrEqual(0))   // nulo entra na conta
u.Where(orm.WhereColumn(f.Vencimento, orm.LessThan, orm.Now()))  // relógio do BANCO

rotulo := orm.Upper(orm.Concat(f.Nome, " <", f.Email, ">")).As("rotulo")
linhas, _ := u.Select(rotulo).Rows(ctx)
linhas[0].Text(rotulo)

ano := orm.YearOf(f.Nascimento).As("ano")
u.Select(ano, orm.CountAll()).GroupBy(orm.YearOf(f.Nascimento)).Rows(ctx)
```

Disponíveis: `Lower`, `Upper`, `Trim`, `Length`, `Concat`, `Coalesce`, `YearOf`,
`MonthOf`, `DayOf`, `Now`.

| | mysql | postgres | oracle | sqlserver |
|---|---|---|---|---|
| tamanho | `LENGTH` | `LENGTH` | `LENGTH` | `LEN` |
| concatenação | `CONCAT_WS('')` | `CONCAT` | `a \|\| b` | `CONCAT` |
| ano | `YEAR` | `EXTRACT(YEAR …)` | `EXTRACT(YEAR …)` | `YEAR` |
| agora | `CURRENT_TIMESTAMP` nos quatro | | | |

> `Concat` trata nulo como vazio nos quatro. Três já fazem isso; o `CONCAT` do
> MySQL devolveria `NULL` para a linha inteira, e por isso a compilação dele usa
> `CONCAT_WS` — promessa única em vez de comportamento por banco.
>
> **A única combinação recusada:** `GroupBy` por expressão que carrega valor. Os
> quatro bancos casam a expressão do `GROUP BY` com a da projeção **pelo texto**,
> e cada valor gera placeholder próprio, então nunca casam. Falha com
> `orm.ErrValueInGroupBy` — inlinear o valor resolveria no banco e abriria
> injeção. Na prática o agrupamento real não tem valor, e o `NULL` já forma grupo
> próprio sem `Coalesce`.

### Raw contido

Último recurso, para o que nem as expressões cobrem. Duas regras fazem esta
porta não virar buraco: **valor só por argumento** e **dialeto declarado, nunca
assumido**.

```go
// obriga os quatro; faltar um falha na compilação
resto := orm.RawPerDialect(map[orm.Dialect]string{
    orm.MySQL:     "MOD(`id`, 2) = {}",
    orm.Postgres:  `MOD("id", 2) = {}`,
    orm.Oracle:    `MOD("ID", 2) = {}`,
    orm.SQLServer: "[id] % 2 = {}",
}, 0)

u.Where(resto).Count(ctx)

// existe em um banco só; nos outros três FALHA em voz alta
u.Where(orm.RawOnly(orm.Oracle, `REGEXP_LIKE("NOME", {})`, "^Ana"))
```

O `{}` é onde o valor entra, e a ORM o troca pelo placeholder do dialeto **na
posição real da consulta**. O autor não poderia escrever o placeholder nem
querendo: o número depende de quantos valores as outras cláusulas vincularam
antes. Placeholder escrito à mão (`$1`, `:1`, `@p1`, e `?` no fragmento do
MySQL) falha apontando o `{}`.

Na projeção, o fragmento exige `.As("nome")` — é pelo alias que o `Record` acha
o valor.

> **Limite do `RawPerDialect`:** os valores são os mesmos para os quatro
> fragmentos, o que é o que garante que a pergunta seja a mesma. Se um banco
> precisa do valor em outro formato, já são quatro perguntas parecidas — e o caso
> é `RawOnly`, ou repensar a consulta.

### A mesma regra no banco e em memória

O mesmo critério que filtra no banco responde sobre uma linha já carregada. A
razão é a camada de regras: escrever a regra duas vezes — uma como cláusula e
outra como `if` — deixa as duas divergirem sem ninguém notar.

```go
criterio := orm.Lower(f.Nome).Contains("ana")

u.Where(criterio).Get(ctx)        // o banco avalia
orm.Matches(criterio, linha)      // o Go avalia, sem ida ao banco

orm.Evaluable(criterio)           // false para fragmento Raw: só o banco responde
```

---

## Modificações

### Camada gerada num pacote único

`internal/gokit/core/orm` e `internal/gokit/core/migration/alias` viraram
**`internal/gokit/core`**. A aplicação tem um import só, e o que é reaproveitado
entre camadas (`Row`, scanner, `Column`) fica disponível sem re-export.

- migrations: `alias.Users` → **`core.Table.Users`**; views: `view.X` → `core.View.X`
- aplicação: `internal/gokit/core/orm` → `internal/gokit/core`
- `fields.gen.go` → `entities.gen.go`; default de `output.orm` → `internal/gokit/core`

**Compatibilidade:** o parser continua aceitando `alias.X`, `table.X` e `view.X`,
e os caminhos legados seguem sendo lidos e mesclados. Projeto que fixou
`output.orm` no `gokit.json` mantém o caminho dele. Nada quebra sem regenerar.

O checksum de migration `.go` é calculado sobre as **operações parseadas**, não
sobre os bytes do arquivo — então trocar a forma de citar a tabela em migration
já aplicada **não gera drift**.

### API pública em inglês

A superfície que o desenvolvedor digita passou a ser toda em inglês. Comentários
e identificadores internos seguem em português.

| antes | agora |
|---|---|
| `Todas()` | `Unrestricted()` |
| `Agora()` | `Now()` |
| `AnoDe` / `MesDe` / `DiaDe` | `YearOf` / `MonthOf` / `DayOf` |
| `RawPor` / `RawEm` | `RawPerDialect` / `RawOnly` |
| `Avaliavel()` | `Evaluable()` |
| `NomeDeSaida()` | `OutputName()` |
| `Record.Valor` / `.PorNome` | `Record.Value` / `.ByName` |
| `ErrSemWhere` | `ErrMissingWhere` |
| `ErrProjecaoAgregada` | `ErrAggregateProjection` |
| `ErrSavepointInvalido` | `ErrInvalidSavepoint` |
| `ErrTravaComLimiteNoOracle` | `ErrLockWithLimitOnOracle` |

> `Unrestricted` em vez de `All` porque `All()` já existe — é o que inicia uma
> pesquisa sem filtro.

### ORM completa

Entraram, nesta ordem: escrita e chave (incluindo PK composta e não-`id`),
transação e erros tipados, `GroupBy`/`Having`/`Rows`, `Join`/`LeftJoin`,
`WhereColumn` e subconsultas, `Chunk`/`Each`/`Lock`, `Upsert`/`InsertIgnore`,
expressões cross-dialect e `Raw` contido.

### Escrita tipada e paridade dos terminais

- **`orm.Set`** move o erro de tipo do banco para o compilador, sem gerar uma
  linha de código: são métodos nos tipos de coluna que já existiam, então vale
  para toda entidade, presente e futura, e compõe com `Upsert`/`InsertIgnore` de
  graça. `orm.Values` passou a ser a saída explícita para o caso dinâmico.
- **Todo terminal promovido no `Model` ganhou a variante `*With`** — eram 15 sem
  ela (`Count`, `Get`, `First`, `Exists`, `Rows`, `Sum`, `Avg`, `Min`, `Max`,
  `Paginate`, os três `Pluck`, `Chunk`, `Each`). A assimetria obrigava a escrever
  `u.All().CountWith(ctx, r)`, e doía dentro de transação — que é exatamente onde
  se lê para decidir e gravar. Um teste trava a paridade: se um terminal novo
  entrar sem `*With`, ele falha na **compilação**.

### Correções que valem citar

- **`Value` não tratava `[]byte`.** O driver do MySQL devolve texto **e decimal**
  como `[]byte` quando o destino do `Scan` é `any` — o caminho do `Record`. Então
  `Float()` num `SUM` agrupado devolvia **zero em silêncio**. Corrigido em
  `Text`, `Int`, `Float`, `Bool` e `Time`.
- **Ordem de compilação das cláusulas.** Com expressões que vinculam valor na
  projeção, no `GROUP BY` e no `ORDER BY`, compilar fora da ordem do texto ligava
  o valor de uma cláusula ao placeholder de outra — SQL válido, resultado errado.
  `Compile` foi reordenado (projeção → `WHERE` → `GROUP BY` → `HAVING` →
  `ORDER BY`), com teste da ordem e teste de que o SQL de quem não usa expressão
  não mudou.
- **`Concat` no Postgres** exige `CAST($n AS TEXT)`: ele não deduz o tipo do
  parâmetro numa função variádica e recusava a consulta inteira.
- **Leitores paralelos do catálogo.** Havia três regexes próprios sobre os
  arquivos gerados, e um deles casava com `var Table = struct {` devolvendo
  “Table” como nome de tabela. Substituídos por acessores com cache.

---

## Convenções

- **Superfície pública e código gerado em inglês.** Comentários, identificadores
  internos e mensagens de erro em **português**.
- **Nome de coluna nunca em texto.** Em nenhum lugar da autoria se digita o nome
  de uma coluna: `Where`, `Select`, `OrderBy`, `Values` e até os getters do
  `Record` recebem a coluna gerada. Se um método público aceita nome de coluna
  como string, está errado. A única saída explícita é `Record.ByName`, cujo nome
  diz que ali se abre mão da checagem do compilador.
- **Valor nunca no texto do SQL.** Sempre placeholder + argumento — inclusive
  dentro de `Raw`.
- **Mensagem de erro diz o que aconteceu e como resolver.** O desenvolvedor a lê
  no meio de um corpus de centenas de migrations.
- **Medir, não supor.** Comportamento de banco é contraintuitivo e diverge entre
  os quatro; afirmação sobre dialeto precisa de evidência de execução.

---

## Comandos

```bash
gokit migrate validate     # confere o corpus sem tocar no banco (~0,5 s)
gokit migrate run          # aplica as pendentes
gokit migrate create       # nova migration a partir do catálogo
gokit orm                  # regera internal/gokit/core
gokit seed run
gokit factory validate | create | run
gokit reload               # pipeline completo de manutenção do projeto
gokit mode dev | prod      # go.work local ou versão publicada
gokit doctor
```

Ao compilar um binário de teste, use sempre
`-ldflags "-X main.CommitHash=development"` — com o hash real, o updater busca a
versão publicada e **sobrescreve o build local em silêncio**.
