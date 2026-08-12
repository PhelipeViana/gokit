package orm

// Compilação das operações de escrita: INSERT, UPDATE e DELETE.
//
// A autoria é a mesma dos quatro bancos — quem escreve informa colunas e valores
// pelos Fields, e o dialeto entra só aqui. É a razão de a ORM existir: sem isso,
// cada escrita que fuja do trivial viraria SQL específico, e o "escreve uma vez,
// roda nos quatro" morre na primeira tabela.
//
// A devolução da chave gerada é a parte em que os quatro bancos mais divergem, e
// está tratada em retorno.go.

import (
	"fmt"
	"sort"
	"strings"
)

// Values são as colunas e valores de uma escrita, chaveados pelo Field.
//
// Mapa em vez de fatia de pares porque a autoria fica direta
// (orm.Values{u.Field.Nome: "Ana"}) e porque coluna repetida deixa de ser
// possível. A ordem de iteração de mapa em Go é aleatória, então a compilação
// ordena pela posição da coluna na entidade — SQL estável entre execuções é o
// que permite testar e comparar dialetos.
type Values map[Column]any

// ordenar devolve os pares na ordem de declaração da entidade, validando que
// cada coluna pertence a ela.
func (v Values) ordenar(entity EntityFields) ([]Field, []any, error) {
	if len(v) == 0 {
		return nil, nil, nil
	}
	type par struct {
		campo   Field
		valor   any
		posicao int
	}
	pares := make([]par, 0, len(v))
	for coluna, valor := range v {
		campo := coluna.columnField()
		// A comparação inclui a tabela, não só o nome da coluna. Nomes como "nome"
		// e "id" repetem em quase toda entidade, então comparar só a coluna deixava
		// passar um Field de outra entidade — e o INSERT ia para a tabela certa com
		// a coluna de outra, sem reclamar.
		posicao := -1
		for i, declarado := range entity.Fields {
			if strings.EqualFold(declarado.Column, campo.Column) &&
				strings.EqualFold(declarado.Table, campo.Table) {
				posicao = i
				break
			}
		}
		if posicao < 0 {
			return nil, nil, errColunaDeOutraEntidade(campo.Column, entity.Name)
		}
		pares = append(pares, par{campo: campo, valor: valor, posicao: posicao})
	}
	sort.Slice(pares, func(i, j int) bool { return pares[i].posicao < pares[j].posicao })

	campos := make([]Field, 0, len(pares))
	valores := make([]any, 0, len(pares))
	for _, p := range pares {
		campos = append(campos, p.campo)
		valores = append(valores, p.valor)
	}
	return campos, valores, nil
}

// CompileInsert monta o INSERT de uma ou mais linhas.
//
// Unrestricted as linhas usam o mesmo conjunto de colunas — o da primeira. Linha com
// coluna faltando receberia NULL numa posição que o desenvolvedor não escreveu,
// e isso é erro de autoria, não conveniência.
func compileInsert(entity EntityFields, linhas []Values, options CompileOptions) (CompiledQuery, error) {
	d := options.Dialect
	if !supportedDialect(d) {
		return CompiledQuery{}, errDialetoNaoSuportado(d)
	}
	if len(linhas) == 0 {
		return CompiledQuery{}, errSemValores("Insert")
	}

	campos, primeiros, err := linhas[0].ordenar(entity)
	if err != nil {
		return CompiledQuery{}, err
	}
	if len(campos) == 0 {
		return CompiledQuery{}, errSemValores("Insert")
	}

	colunas := make([]string, 0, len(campos))
	for _, campo := range campos {
		colunas = append(colunas, quoteIdentFor(d, campo.Column))
	}

	var args []any
	var grupos []string
	adicionar := func(valores []any) {
		marcas := make([]string, 0, len(valores))
		for _, valor := range valores {
			args = append(args, valor)
			marcas = append(marcas, placeholderFor(d, len(args)))
		}
		grupos = append(grupos, "("+strings.Join(marcas, ", ")+")")
	}
	adicionar(primeiros)

	for _, linha := range linhas[1:] {
		outros, valores, err := linha.ordenar(entity)
		if err != nil {
			return CompiledQuery{}, err
		}
		if !mesmasColunas(campos, outros) {
			return CompiledQuery{}, fmt.Errorf(
				"orm: todas as linhas do Insert precisam ter as mesmas colunas (esperado: %s)",
				nomesDeColunas(campos))
		}
		adicionar(valores)
	}

	sql := "INSERT INTO " + qualifyFor(d, options.Schema, entity.Name) +
		" (" + strings.Join(colunas, ", ") + ") VALUES " + strings.Join(grupos, ", ")

	return CompiledQuery{SQL: sql, Args: args}, nil
}

func mesmasColunas(a, b []Field) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i].Column, b[i].Column) {
			return false
		}
	}
	return true
}

// compileUpdate monta o UPDATE. O WHERE vem dos itens já acumulados na Query, o
// que faz o UPDATE herdar exatamente os mesmos operadores e combinadores da
// leitura — inclusive Filter e When.
func compileUpdate[T any](q Query[T], valores Values, options CompileOptions) (CompiledQuery, error) {
	d := options.Dialect
	if !supportedDialect(d) {
		return CompiledQuery{}, errDialetoNaoSuportado(d)
	}
	campos, dados, err := valores.ordenar(q.Entity)
	if err != nil {
		return CompiledQuery{}, err
	}
	if len(campos) == 0 {
		return CompiledQuery{}, errSemValores("Update")
	}

	// Os placeholders do SET vêm antes dos do WHERE, então o contexto de
	// compilação do WHERE precisa começar depois deles.
	atribuicoes := make([]string, 0, len(campos))
	args := make([]any, 0, len(campos))
	for i, campo := range campos {
		args = append(args, dados[i])
		atribuicoes = append(atribuicoes,
			quoteIdentFor(d, campo.Column)+" = "+placeholderFor(d, len(args)))
	}

	ctx := &compileCtx{dialect: d, parentTable: q.Entity.Name, schema: options.Schema, args: args}
	where, err := compileItems(q.items, ctx)
	if err != nil {
		return CompiledQuery{}, err
	}

	sql := "UPDATE " + qualifyFor(d, options.Schema, q.Entity.Name) +
		" SET " + strings.Join(atribuicoes, ", ")
	if where != "" {
		sql += " WHERE " + where
	}
	return CompiledQuery{SQL: sql, Args: ctx.args}, nil
}

// compileDelete monta o DELETE, com o mesmo WHERE da leitura.
func compileDelete[T any](q Query[T], options CompileOptions) (CompiledQuery, error) {
	d := options.Dialect
	if !supportedDialect(d) {
		return CompiledQuery{}, errDialetoNaoSuportado(d)
	}
	ctx := &compileCtx{dialect: d, parentTable: q.Entity.Name, schema: options.Schema}
	where, err := compileItems(q.items, ctx)
	if err != nil {
		return CompiledQuery{}, err
	}
	sql := "DELETE FROM " + qualifyFor(d, options.Schema, q.Entity.Name)
	if where != "" {
		sql += " WHERE " + where
	}
	return CompiledQuery{SQL: sql, Args: ctx.args}, nil
}
