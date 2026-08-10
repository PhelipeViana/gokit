package orm

import "context"

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
	wheres []Expression
	orders []ordering
	limit  int // hasMany: máximo POR PAI (corte em memória); belongsTo: ignorado
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

// With aninha relações do destino (users → cidade → estado). Devolve cópia.
func (rel Relation) With(children ...Relation) Relation {
	next := rel
	next.nested = append(append([]Relation(nil), rel.nested...), children...)
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
	childKeyField NumberFilterField, // coluna-chave no filho (normalmente o id)
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
	childFkField NumberFilterField, // coluna FK no filho
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
func loadChildren[C any](ctx context.Context, r Runner, child Model[C], keyField NumberFilterField, ids []any, rel Relation) ([]C, error) {
	var out []C
	for start := 0; start < len(ids); start += maxIn {
		end := start + maxIn
		if end > len(ids) {
			end = len(ids)
		}
		q := child.Where(keyField.Em(ids[start:end]...))
		if len(rel.wheres) > 0 {
			q = q.Where(rel.wheres...)
		}
		if len(rel.orders) > 0 {
			q = q.appendOrders(rel.orders)
		}
		q = q.With(rel.nested...)
		lote, err := q.GetWith(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, lote...)
	}
	return out, nil
}
