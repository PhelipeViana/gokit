# ORM do GoKit — casos de uso

Catálogo do que a camada de **leitura** faz hoje, organizado por tarefa. Todo
exemplo é código que roda (extraídos de `teste/cmd/ormdemo` e `teste/cmd/api`).

Validado nos **4 dialetos** (MySQL, PostgreSQL, Oracle, SQL Server):
`teste/cmd/conform` → 55 casos, 0 divergências.

---

## 0. As três camadas e a ordem de resolução

| Camada | O que é | Onde |
|---|---|---|
| **ORM** | motor genérico: query, operadores, relações, execução | `gokit/orm` |
| **Fields** | o ORM já conhecendo o schema (gerado das migrations) | `internal/gokit/core/entities.gen.go` |
| **Services** | escritas + regras de negócio | *(a fazer)* |

> Regra: usar sempre a camada mais **alta** que resolve. Service → Fields → ORM.

---

## 1. Conectar (uma vez, no boot)

```go
sess, err := gokitdb.Connect(ctx)   // resolve dialeto/schema do gokit.json/.env
if err != nil { log.Fatal(err) }
defer sess.Close()
```

A autoria **nunca** passa dialeto nem schema — vêm da conexão ativa. Precisa
existir porque as queries rodam no processo da **sua aplicação**, não no binário
do gokit.

---

## 2. O handle da entidade

```go
u := core.Users       // entidade
f := u.Column         // operadores por coluna
ur := u.Relation      // relações
```

- `u.<verbo>` → `Where`, `Select`, `OrderBy`, `Limit`, `Get`, `Count`…
- `u.Column.<Coluna>.<operador>()` → `f.Nome.Contains("silva")`
- `u.Relation.<Rel>(...)` → `ur.Cidade(...)`

---

## 3. Consultas básicas

```go
total, err := u.Count(ctx)                                    // int64
existe, err := u.Where(f.Email.EndsWith("@x.com")).Exists(ctx)
row, err := u.OrderBy(f.Id).First(ctx)                        // (T, error)
rows, err := u.OrderBy(f.Id).Limit(3).Get(ctx)                // []UsersRow
```

**Não encontrado** é sentinela, não `bool`:
```go
if errors.Is(err, orm.ErrNotFound) { /* 404 */ }
```
`Get` vazio devolve **slice vazia** (no JSON sai `[]`, não `null`).

---

## 4. Filtros por tipo de coluna

```go
// texto
f.Nome.Equal("x")   f.Nome.NotEqual("x")   f.Nome.Contains("x")
f.Nome.StartsWith("x")   f.Nome.EndsWith("x")   f.Nome.In("a","b")
f.Nome.IsNull()   f.Nome.IsNotNull()

// número
f.Id.Equal(1)  .NotEqual(1)  .GreaterThan(1)  .GreaterOrEqual(1)  .LessThan(1)  .LessOrEqual(1)
f.Id.Between(5, 10)   f.Id.In(1, 3, 5)   f.Id.IsNull()   f.Id.IsNotNull()

// booleano
f.Ativo.IsTrue()   f.Ativo.IsFalse()   f.Ativo.Equal(true)

// data / datetime
f.Criado.Equal(t)  .Before(t)  .After(t)  .OnOrBefore(t)  .OnOrAfter(t)
f.Criado.Between(ini, fim)   f.Criado.In(t1, t2)   f.Criado.IsNull()
```

O valor nunca entra no texto do SQL — sempre placeholder + arg (`:1`/`$1`/`?`/`@p1`).

---

## 5. Combinar condições (um `Where` só)

```go
u.Where(                                  // args = AND
    f.Nome.Contains("silva"),
    orm.Or(                               // parênteses explícitos
        f.CidadeId.Equal(1),
        f.CidadeId.Equal(2),
    ),
)
// WHERE nome LIKE ? AND (cidade_id = ? OR cidade_id = ?)
```

Aninhamento à vontade: `orm.And(orm.Or(a, b), c)`.

> **Por que não `.where().orWhere()`**: encadear esconde a precedência (AND liga
> antes de OR) — o bug clássico do Eloquent. Aqui os parênteses são o que você
> escreveu.

---

## 6. Filtro dinâmico (valores que vêm de fora)

O caso "campo vazio não filtra", sem `if` por campo:

```go
q := u.Where(
    orm.Filter(f.Nome,     orm.Contains, qp.Get("nome")),      // "" → ignorado
    orm.Filter(f.CidadeId, orm.Equal,    qp.Get("cidade_id")), // converte p/ int
    orm.Filter(f.Ativo,    orm.Equal,    qp.Get("ativo")),     // converte p/ bool
    orm.When(qp.Get("has") == "pedidos", ur.PedidosByUser().Exists()),
)
```

- **vazio** → a condição sai do `WHERE`;
- **converte** para o tipo da coluna (int/decimal/bool/**data**);
- **valor inválido** (`cidade_id=abc`) → ignorado, sem erro;
- `When(cond, expr)` para condições que não são "campo = valor".

---

## 7. Datas e intervalos

```go
// intervalo fechado
p.Where(f.DataPedido.Between(ini, fim))            // BETWEEN

// intervalo aberto de um lado, vindo da query string
p.Where(
    orm.Filter(f.DataPedido, orm.GreaterOrEqual, qp.Get("de")),   // >= de
    orm.Filter(f.DataPedido, orm.LessOrEqual,    qp.Get("ate")),  // <= ate
)
```

Aceita `time.Time`, `*time.Time`, `orm.Value` ou **texto** (`2006-01-02`,
`2006-01-02 15:04:05`, RFC3339) — `orm.DateValue` converte. Isso importa: string
crua funciona no MySQL mas quebra em outros bancos.

---

## 8. Ordenação, paginação e projeção

```go
u.OrderByDesc(f.Id).Offset(2).Limit(3).Get(ctx)

pg, err := u.OrderBy(f.Id).Paginate(ctx, 2, 15)
// pg.Items, pg.Total, pg.Page, pg.Size, pg.Pages

u.Select(f.Id, f.Nome).Get(ctx)   // só 2 colunas; o resto vem zero
u.Distinct().Get(ctx)
```

---

## 9. Agregações e escalares

```go
v, err := p.Sum(ctx, f.Valor)     // Valor
v.Float()   v.Int()   v.Time()   v.Text()   v.Bool()   v.IsNull()

p.Avg(ctx, f.Valor)
p.Min(ctx, f.DataPedido)          // funciona em data (e texto)
p.Max(ctx, f.Valor)
```

`Value` existe porque agregação é dinâmica: `MIN` de data devolve data, de id
devolve número. **Ao expor em JSON, extraia o tipo** (`sum.Float()`) — o bruto
depende do driver (DECIMAL do MySQL vem como texto).

---

## 10. Conveniências

```go
row, err := u.Find(ctx, 3)                       // por PK; ErrNotFound
ids, err := u.PluckInt(ctx, f.Id)                // []int64
ufs, err := e.Distinct().PluckString(ctx, ef.Uf)  // []string
vs,  err := u.Pluck(ctx, f.Id)                   // []Value (genérico)
```

---

## 11. Relações (eager loading)

O nó é uma **chamada**: o que está dentro do parêntese pertence àquele nó.

```go
u.With(ur.Cidade())                                  // belongsTo
u.With(ur.PedidosByUser())                          // hasMany
u.With(ur.Cidade(cr.Estado(er.Pais())))              // profundidade ilimitada
u.With(ur.Cidade(cr.Estado(), cr.Status()))          // 2 relações DO MESMO nó
u.With(ur.Cidade(cr.Estado(), cf.Nome))              // + projeção do nó
u.With(ur.Cidade(), ur.PedidosByUser())             // ramos irmãos
```

Ler o resultado (nil-safe — relação não pedida é `nil`):
```go
if r.Cidade != nil && r.Cidade.Estado != nil { ... }
len(r.PedidosByUser)
```

**Constraints na relação:**
```go
u.With(ur.PedidosByUser().
    Where(pf.Entregue.IsTrue()).   // filtra a relação
    OrderByDesc(pf.Id).
    Limit(1))                          // N por PAI (top-N por grupo)
```

**Filtrar o pai pela relação** (não carrega os filhos):
```go
u.Where(ur.PedidosByUser().Where(pf.Entregue.IsTrue()).Exists())
u.Where(ur.PedidosByUser().DoesntExist())
// → [NOT] EXISTS (SELECT 1 FROM pedidos WHERE pedidos.user_id = users.id AND ...)
```

**Árvore dinâmica** (montada em runtime, ex.: `?include=`):
```go
var loads []orm.RelationSource
if req.Cidade  { loads = append(loads, ur.Cidade(cr.Estado())) }
if req.Pedidos { loads = append(loads, ur.PedidosByUser()) }
u.With(loads...).Get(ctx)
```

### Sem N+1, por design
1 query por **nó** da árvore, constante em N (ids distintos via `IN`, fatiado em
lotes de 1000). Não existe lazy loading: `u.Cidade` é `nil` se você não pediu —
então N+1 por acidente é impossível.

---

## 12. Conexão explícita (transação, multi-banco)

Todo terminal tem variante `*With`:
```go
u.Where(...).CountWith(ctx, sess)
u.Where(...).GetWith(ctx, sess)
u.Find(ctx, 3)  →  u.FindWith(ctx, sess, 3)
p.Sum(ctx, col) →  p.SumWith(ctx, sess, col)
```

---

## 13. Resposta HTTP (padrão gerado)

```go
app.RespondList(w, r, q)              // pagina + {data, meta}
app.RespondItem(w, r, row, err)        // {data} ou 404 (do ErrNotFound)
app.RespondData(w, r, payload, err)   // {data} arbitrário
app.RespondErrorr(w, r, app.BadRequest("id inválido"))  // 400
```
```json
{ "success": true, "message": "Operação realizada com sucesso",
  "data": [...],
  "meta": { "timestamp": "...", "request_id": "",
            "pagination": { "page": 1, "per_page": 15, "total": 20, "last_page": 2 } } }
```

---

## 14. Regras e armadilhas

| Situação | Comportamento |
|---|---|
| `Select` + `With` | inclua a coluna da FK, senão não há como costurar |
| Projeção **no nó** | o motor injeta as chaves (do pai e dos filhos) sozinho |
| `Limit` na relação | é **por pai** (busca todos e corta em memória) |
| Constraint no meio do caminho | `.Where` devolve `Relation` — use `.With()` manual |
| Coluna/relação de outra entidade | compila, falha em runtime com erro claro |
| Duas FKs pra mesma tabela | nomes desambiguados (`PedidosByUser`/`PedidosByAprovador`) |
| `Value` em JSON | extraia o tipo (`.Float()`), não devolva cru |
| Binário do gokit defasado | regerar com `.exe` antigo **rebaixa** o `entities.gen.go` |

---

## 15. O que ainda **não** existe

> Este documento cobre a camada de **leitura**. Tudo que era listado aqui como
> ausente — escritas, transação, `GroupBy`/`Having`, `Join`, subconsultas,
> `Chunk`/`Lock`, `Upsert`, expressões cross-dialect, `Raw` contido, PK composta
> e não-`id` — **existe hoje** e está documentado no
> [README](../README.md#casos-de-uso).

Segue fora do escopo:

- m2m (pivot) e relações polimórficas
- cursor de banco (o percurso em lote é `Chunk`, por chave)
- camada de **Services** (escritas + regras de negócio)

---

## 16. Como validar

```bash
cd teste && go run ./cmd/ormdemo             # catálogo executável (21 seções)
cd teste && go run ./cmd/conform             # conformidade nos 4 dialetos
cd teste && go run ./cmd/conform -bench      # performance bruta
cd teste && docker compose up -d app         # API + internal/gokit/test/api.gen.http
```

### Performance medida (50 ops, mesma máquina)

| operação | mysql | postgres | oracle | sqlserver |
|---|---|---|---|---|
| compilar query (sem I/O) | 2 µs | 2 µs | 2 µs | 2 µs |
| `Count()` | 494 µs | 540 µs | 906 µs | 635 µs |
| `Get()` 20 linhas | 575 µs | 555 µs | 1,14 ms | 765 µs |
| `Get()` + belongsTo | 1,57 ms | 1,01 ms | 2,33 ms | 1,47 ms |
| `Get()` + árvore 3 níveis | 3,43 ms | 2,03 ms | 4,37 ms | 2,72 ms |
| `Paginate(1,15)` | 1,09 ms | 1,03 ms | 2,19 ms | 1,33 ms |

O motor é irrelevante no custo (2 µs); o que pesa é **I/O por nó de relação**
(~0,5–1 ms por query extra).
