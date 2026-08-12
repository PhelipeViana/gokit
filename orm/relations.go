package orm

import (
	"context"
	"strings"
)

// Relation é um nó da árvore de eager load. É opaco: carrega um loader tipado
// (gerado por tabela), os filhos (relação de relação) e as CONSTRAINTS a aplicar
// na query filha (filtro/ordem/limite). Imutável — os métodos devolvem cópia.
type Relation struct {
	name   string
	load   RelationLoader
	nested []Relation

	// metadados de compilação (preenchidos pelo gerado) — para EXISTS correlacionado
	kind       string // "belongsTo" | "hasMany"
	childTable string // tabela do destino
	parentCol  string // coluna de correlação no PAI
	childCol   string // coluna de correlação no FILHO

	// constraints aplicadas à query da relação (o "sub-query" do .With)
	wheres  []Expression
	orders  []ordering
	limit   int     // hasMany: máximo POR PAI (corte em memória); belongsTo: ignorado
	selects []Field // projeção do nó (vazio = todas as colunas). A chave de
	// junção é injetada automaticamente na carga.
}

// NodeArg é o que pode ir dentro de um nó de relação, em qualquer ordem:
//   - outra relação → aninhamento (cr.Estado(), cr.Status())
//   - uma coluna    → projeção do retorno DAQUELE nó (cr.Field.Nome)
//
// Só relações e colunas implementam a interface, então o compilador rejeita
// qualquer outro argumento.
type NodeArg interface{ nodeArg() }

func (Relation) nodeArg()     {}
func (Field) nodeArg()        {}
func (StringColumn) nodeArg() {}
func (NumberColumn) nodeArg() {}
func (DateColumn) nodeArg()   {}
func (BoolColumn) nodeArg()   {}

// Node configura o nó: separa os argumentos entre relações aninhadas e colunas
// de projeção. É o que o código gerado chama em cada relação —
// pr.Cidade(cr.Estado(), cr.Field.Nome).
func (rel Relation) Node(args ...NodeArg) Relation {
	if len(args) == 0 {
		return rel
	}
	next := rel
	next.nested = append([]Relation(nil), rel.nested...)
	next.selects = append([]Field(nil), rel.selects...)
	for _, a := range args {
		switch v := a.(type) {
		case RelationSource:
			next.nested = append(next.nested, v.relation())
		case Column:
			next.selects = append(next.selects, v.columnField())
		}
	}
	return next
}

// RelationLoader executa a carga de UMA relação para um conjunto de linhas-pai
// já buscadas e costura o resultado nelas. parents chega como []<Row> (any) — o
// loader gerado faz o type-assert. rel carrega os filhos (rel.nested) e as
// constraints da relação.
type RelationLoader func(ctx context.Context, r Runner, parents any, rel Relation) error

// NewRelation é usado pelo código gerado para montar o descritor de cada relação
// (nome, tipo, tabela filha e as colunas de correlação pai/filho).
func NewRelation(name, kind, childTable, parentCol, childCol string, load RelationLoader) Relation {
	return Relation{
		name: name, kind: kind, childTable: childTable,
		parentCol: parentCol, childCol: childCol, load: load,
	}
}

// Exists devolve um predicado para o Where do PAI: só passam os pais que TÊM ao
// menos um filho nesta relação (respeitando as constraints .Where da relação).
// Compila como EXISTS correlacionado — não carrega os filhos.
func (rel Relation) Exists() Expression {
	return existsPredicate{childTable: rel.childTable, parentCol: rel.parentCol, childCol: rel.childCol, wheres: rel.wheres}
}

// DoesntExist é o inverso de Exists (NOT EXISTS): pais SEM filho correspondente.
func (rel Relation) DoesntExist() Expression {
	return existsPredicate{childTable: rel.childTable, parentCol: rel.parentCol, childCol: rel.childCol, wheres: rel.wheres, negate: true}
}

// existsPredicate compila para [NOT] EXISTS (SELECT 1 FROM filho WHERE
// filho.childCol = pai.parentCol [AND constraints]).
type existsPredicate struct {
	childTable string
	parentCol  string
	childCol   string
	negate     bool
	wheres     []Expression
}

func (existsPredicate) expression()  {}
func (existsPredicate) active() bool { return true }

func compileExists(e existsPredicate, ctx *compileCtx) (string, error) {
	d := ctx.dialect
	child := qualifyFor(d, ctx.schema, e.childTable)
	parent := qualifyFor(d, ctx.schema, ctx.parentTable)
	inner := child + "." + quoteIdentFor(d, e.childCol) + " = " + parent + "." + quoteIdentFor(d, e.parentCol)
	if len(e.wheres) > 0 {
		frag, err := compileItems(toItems(e.wheres), ctx)
		if err != nil {
			return "", err
		}
		if frag != "" {
			inner += " AND " + frag
		}
	}
	sql := "EXISTS (SELECT 1 FROM " + child + " WHERE " + inner + ")"
	if e.negate {
		sql = "NOT " + sql
	}
	return sql, nil
}

// toItems converte expressões soltas em itens unidos por AND.
func toItems(exprs []Expression) []item {
	its := make([]item, 0, len(exprs))
	for _, e := range exprs {
		its = append(its, item{or: false, expr: e})
	}
	return its
}

// RelationSource é qualquer coisa que represente uma relação: a própria
// Relation ou os tipos de CAMINHO gerados (usersRelCidadeEstado etc.), que
// embutem Relation e por isso herdam relation(). É o que permite escrever
// orm.Users.Relation.Cidade.Estado.Pais direto no With, sem .With() aninhado.
type RelationSource interface{ relation() Relation }

func (rel Relation) relation() Relation { return rel }

// relationsDe resolve uma lista de fontes para Relation.
func relationsDe(fontes []RelationSource) []Relation {
	out := make([]Relation, 0, len(fontes))
	for _, f := range fontes {
		if f == nil {
			continue
		}
		out = append(out, f.relation())
	}
	return out
}

// With aninha relações do destino (users → cidade → estado). Devolve cópia.
func (rel Relation) With(children ...RelationSource) Relation {
	next := rel
	next.nested = append(append([]Relation(nil), rel.nested...), relationsDe(children)...)
	return next
}

// Where restringe a relação carregada (ex.: só pedidos entregues). Vários
// argumentos = AND; use orm.Or/orm.And para compor, igual ao Where da query.
func (rel Relation) Where(expressions ...Expression) Relation {
	next := rel
	next.wheres = append(append([]Expression(nil), rel.wheres...), expressions...)
	return next
}

// OrderBy/OrderByDesc ordenam a relação carregada.
func (rel Relation) OrderBy(c Column) Relation {
	next := rel
	next.orders = append(append([]ordering(nil), rel.orders...), ordering{field: c.columnField()})
	return next
}

func (rel Relation) OrderByDesc(c Column) Relation {
	next := rel
	next.orders = append(append([]ordering(nil), rel.orders...), ordering{field: c.columnField(), desc: true})
	return next
}

// Limit, numa hasMany, é o máximo de filhos POR PAI (top-N por grupo, cortado em
// memória respeitando o OrderBy). Numa belongsTo não tem efeito (é sempre um).
func (rel Relation) Limit(n int) Relation {
	next := rel
	next.limit = n
	return next
}

// mergeRelations funde relações repetidas na mesma lista, unindo recursivamente
// os filhos. Sem isso, dois caminhos que compartilham prefixo
// (cidade.estado.pais e cidade.estado.codigo_verificacao) rodariam em sequência e
// o segundo SOBRESCREVERIA a costura do primeiro — o pais desapareceria da
// resposta. As constraints (Where/OrderBy/Limit) do primeiro pedido prevalecem.
func mergeRelations(rels []Relation) []Relation {
	if len(rels) == 0 {
		return rels
	}
	ordem := make([]string, 0, len(rels))
	grupo := make(map[string]Relation, len(rels))
	for _, r := range rels {
		anterior, existe := grupo[r.name]
		if !existe {
			grupo[r.name] = r
			ordem = append(ordem, r.name)
			continue
		}
		anterior.nested = append(anterior.nested, r.nested...)
		grupo[r.name] = anterior
	}
	out := make([]Relation, 0, len(ordem))
	for _, nome := range ordem {
		r := grupo[nome]
		r.nested = mergeRelations(r.nested)
		out = append(out, r)
	}
	return out
}

// maxIn é o teto de itens por lista IN. O Oracle não aceita mais que 1000 numa
// IN literal; fatiamos por baixo desse limite em todos os dialetos.
const maxIn = 1000

// BelongsTo carrega a relação "para um" (a FK está no pai). É genérica no tipo
// do pai (P) e do filho (C); o loader gerado só fornece os acessos tipados.
func BelongsTo[P any, C any](
	ctx context.Context,
	r Runner,
	parents []P,
	parentKey func(P) (int64, bool), // valor da FK no pai (ok=false se NULL)
	child Model[C],
	childKeyField NumberColumn, // coluna-chave no filho (normalmente o id)
	childKeyOf func(C) int64,
	set func(*P, *C),
	rel Relation,
) error {
	ids := distinctKeys(parents, parentKey)
	if len(ids) == 0 {
		return nil
	}
	children, err := loadChildren(ctx, r, child, childKeyField, ids, rel)
	if err != nil {
		return err
	}
	index := make(map[int64]*C, len(children))
	for i := range children {
		// primeiro vencedor por chave (respeita a ordem retornada)
		if _, ok := index[childKeyOf(children[i])]; !ok {
			index[childKeyOf(children[i])] = &children[i]
		}
	}
	for i := range parents {
		if k, ok := parentKey(parents[i]); ok {
			if c, found := index[k]; found {
				set(&parents[i], c)
			}
		}
	}
	return nil
}

// HasMany carrega a relação "para muitos" (a FK está no filho). Agrupa os
// filhos pela FK e costura o slice em cada pai. Se a relação define Limit, corta
// cada grupo para no máximo N (top-N por pai, respeitando o OrderBy do SQL).
func HasMany[P any, C any](
	ctx context.Context,
	r Runner,
	parents []P,
	parentKey func(P) int64, // normalmente o id do pai
	child Model[C],
	childFkField NumberColumn, // coluna FK no filho
	childFkOf func(C) (int64, bool),
	set func(*P, []C),
	rel Relation,
) error {
	ids := make([]any, 0, len(parents))
	seen := make(map[int64]bool, len(parents))
	for _, p := range parents {
		k := parentKey(p)
		if !seen[k] {
			seen[k] = true
			ids = append(ids, k)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	children, err := loadChildren(ctx, r, child, childFkField, ids, rel)
	if err != nil {
		return err
	}
	grupo := make(map[int64][]C, len(ids))
	for _, c := range children {
		if k, ok := childFkOf(c); ok {
			grupo[k] = append(grupo[k], c)
		}
	}
	if rel.limit > 0 {
		for k, cs := range grupo {
			if len(cs) > rel.limit {
				grupo[k] = cs[:rel.limit]
			}
		}
	}
	for i := range parents {
		set(&parents[i], grupo[parentKey(parents[i])])
	}
	return nil
}

// fieldByColumn acha o Field de uma coluna na entidade (case-insensitive).
func fieldByColumn(e EntityFields, coluna string) (Field, bool) {
	for _, f := range e.Fields {
		if strings.EqualFold(f.Column, coluna) {
			return f, true
		}
	}
	return Field{}, false
}

// withKey garante a coluna de junção na projeção do nó.
func withKey(cols []Field, chave Field) []Field {
	for _, c := range cols {
		if strings.EqualFold(c.Column, chave.Column) {
			return cols
		}
	}
	return append(append([]Field(nil), cols...), chave)
}

// distinctKeys colhe os valores de chave não-nulos e distintos dos pais.
func distinctKeys[P any](parents []P, key func(P) (int64, bool)) []any {
	ids := make([]any, 0, len(parents))
	seen := make(map[int64]bool, len(parents))
	for _, p := range parents {
		if k, ok := key(p); ok && !seen[k] {
			seen[k] = true
			ids = append(ids, k)
		}
	}
	return ids
}

// loadChildren roda a query filha em lotes de até maxIn ids e concatena, para
// não estourar o limite de IN (Oracle). Aplica as constraints da relação
// (Where/OrderBy) e as relações aninhadas. O Limit NÃO vai no SQL — é cortado
// por grupo em memória (ver HasMany), para significar "N por pai".
func loadChildren[C any](ctx context.Context, r Runner, child Model[C], keyField NumberColumn, ids []any, rel Relation) ([]C, error) {
	var out []C
	for start := 0; start < len(ids); start += maxIn {
		end := start + maxIn
		if end > len(ids) {
			end = len(ids)
		}
		q := child.Where(keyField.In(ids[start:end]...))
		if len(rel.wheres) > 0 {
			q = q.Where(rel.wheres...)
		}
		if len(rel.orders) > 0 {
			q = q.appendOrders(rel.orders)
		}
		if len(rel.selects) > 0 {
			// Chaves entram sempre, senão a costura falha em silêncio:
			//  - a que liga este nó ao PAI (keyField);
			//  - a que liga este nó a cada FILHO aninhado (parentCol do filho) —
			//    sem ela, projetar o nó apagaria as relações de dentro dele.
			cols := withKey(rel.selects, keyField.Field)
			for _, filho := range rel.nested {
				if f, ok := fieldByColumn(child.Entity, filho.parentCol); ok {
					cols = withKey(cols, f)
				}
			}
			q = q.selectFields(cols)
		}
		q = q.withAll(rel.nested)
		lote, err := q.GetWith(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, lote...)
	}
	return out, nil
}

// ladosParaJuncao expõe os metadados de correlação para o JOIN montar o ON.
// Devolve a tabela de destino, a coluna dela e a coluna da tabela de origem.
//
// O par depende do tipo: em belongsTo a FK mora na própria tabela e aponta para a
// chave do destino; em hasMany é o inverso. Errar isso gera junção invertida, que
// devolve linhas silenciosamente erradas — por isso a informação vem do gerador e
// não de quem escreve a pesquisa.
func (rel Relation) ladosParaJuncao() (alvo, chaveAlvo, chavePropria string) {
	switch rel.kind {
	case "belongsTo":
		return rel.childTable, rel.childCol, rel.parentCol
	case "hasMany":
		return rel.childTable, rel.childCol, rel.parentCol
	}
	return "", "", ""
}
