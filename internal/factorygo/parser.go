// Package factorygo lê os arquivos de factory por AST.
//
// O gokit é um binário pronto: ele não compila nem executa o projeto do
// usuário. O corpo de Data, portanto, não é Go arbitrário — é uma expressão
// declarativa que este pacote avalia. O que é aceito ali está em
// vocabulario.go, e nada além disso passa.
package factorygo

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/PhelipeViana/gokit/internal/astparser"
	"github.com/PhelipeViana/gokit/internal/i18n"
	migrate "github.com/PhelipeViana/gokit/migration"
)

// Campo é uma coluna da factory já resolvida para uma forma executável.
type Campo struct {
	Coluna string
	// Valor gera o dado da linha. É nil quando o campo é um vínculo.
	Valor func(index int) (any, error)
	// Referencia aponta para a coluna de outra tabela; o executor resolve
	// contra as linhas realmente inseridas lá.
	Referencia *migrate.Link
	// Origem é o texto exato da expressão como está no arquivo. Serve às
	// mensagens de erro e, principalmente, à regeneração: o gerador mantém a
	// coluna com a expressão que o autor escreveu.
	Origem string
}

// Arquivo é uma factory carregada de disco.
type Arquivo struct {
	Caminho string
	Funcao  string
	Tabela  string
	Ruler   migrate.Ruler
	Campos  []Campo
}

// Linha gera os valores de uma linha. Vínculos ficam de fora: quem os resolve
// é o executor, que conhece as linhas já inseridas nas tabelas pai.
func (arquivo Arquivo) Linha(index int) (map[string]any, error) {
	valores := make(map[string]any, len(arquivo.Campos))
	for _, campo := range arquivo.Campos {
		if campo.Referencia != nil {
			continue
		}
		valor, err := campo.Valor(index)
		if err != nil {
			return nil, i18n.Errf("fcp_column_wrap", campo.Coluna, err)
		}
		valores[campo.Coluna] = valor
	}
	return valores, nil
}

// Colunas devolve os nomes na ordem em que foram escritos, para que o INSERT
// gerado seja estável entre execuções.
func (arquivo Arquivo) Colunas() []string {
	nomes := make([]string, 0, len(arquivo.Campos))
	for _, campo := range arquivo.Campos {
		nomes = append(nomes, campo.Coluna)
	}
	return nomes
}

// Referencias lista as tabelas pai referenciadas por esta factory.
func (arquivo Arquivo) Referencias() []string {
	vistas := map[string]bool{}
	var tabelas []string
	for _, campo := range arquivo.Campos {
		if campo.Referencia == nil {
			continue
		}
		alvo := strings.ToUpper(campo.Referencia.Table)
		if !vistas[alvo] {
			vistas[alvo] = true
			tabelas = append(tabelas, alvo)
		}
	}
	sort.Strings(tabelas)
	return tabelas
}

// CarregarPasta lê as factories de uma pasta.
//
// Qualquer .go serve, e um arquivo pode declarar várias factories: não há regra
// implícita no fatiamento por tabela, então a organização é escolha de quem
// escreve — um arquivo só, um por tabela, ou um por assunto. Arquivo sem nenhuma
// função *Factory é ignorado, para que um helper na mesma pasta não vire erro.
func CarregarPasta(pasta string) ([]Arquivo, error) {
	entradas, err := os.ReadDir(pasta)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var arquivos []Arquivo
	var problemas []string
	for _, entrada := range entradas {
		if entrada.IsDir() || !strings.HasSuffix(entrada.Name(), ".go") || strings.HasSuffix(entrada.Name(), "_test.go") {
			continue
		}
		caminho := filepath.Join(pasta, entrada.Name())
		lidas, err := ParseArquivos(caminho)
		if err != nil {
			problemas = append(problemas, fmt.Sprintf("%s: %v", entrada.Name(), err))
			continue
		}
		arquivos = append(arquivos, lidas...)
	}

	sort.Slice(arquivos, func(i, j int) bool { return arquivos[i].Tabela < arquivos[j].Tabela })
	if len(problemas) > 0 {
		return arquivos, i18n.Errf("fcp_problems", len(problemas), strings.Join(problemas, "\n  - "))
	}
	return arquivos, nil
}

// ParseArquivos lê TODAS as factories de um arquivo e resolve as expressões do
// corpo de Data. Erros de vocabulário aparecem aqui, na leitura — não no meio de
// um INSERT.
//
// Devolve lista vazia, sem erro, quando o arquivo não declara factory nenhuma.
func ParseArquivos(caminho string) ([]Arquivo, error) {
	file, set, err := astparser.ParseFile(caminho)
	if err != nil {
		return nil, err
	}

	var arquivos []Arquivo
	for _, declaracao := range file.Decls {
		funcao, ok := declaracao.(*ast.FuncDecl)
		if !ok || funcao.Name == nil || !strings.HasSuffix(funcao.Name.Name, "Factory") {
			continue
		}
		literal := retornoComposto(funcao)
		if literal == nil {
			continue
		}
		arquivo, err := lerFactory(set, literal)
		if err != nil {
			return nil, i18n.Errf("fcp_func_wrap", funcao.Name.Name, err)
		}
		arquivo.Caminho = caminho
		arquivo.Funcao = funcao.Name.Name
		arquivos = append(arquivos, arquivo)
	}
	return arquivos, nil
}

// ParseArquivo lê a primeira factory do arquivo. Existe para quem só precisa de
// uma; a leitura da pasta usa ParseArquivos.
func ParseArquivo(caminho string) (Arquivo, error) {
	arquivos, err := ParseArquivos(caminho)
	if err != nil {
		return Arquivo{}, err
	}
	if len(arquivos) == 0 {
		return Arquivo{}, i18n.Errf("fcp_no_func")
	}
	return arquivos[0], nil
}

// retornoComposto extrai o migrate.Factory{...} do return da função.
func retornoComposto(funcao *ast.FuncDecl) *ast.CompositeLit {
	if funcao.Body == nil {
		return nil
	}
	for _, statement := range funcao.Body.List {
		retorno, ok := statement.(*ast.ReturnStmt)
		if !ok || len(retorno.Results) != 1 {
			continue
		}
		if literal, ok := retorno.Results[0].(*ast.CompositeLit); ok {
			return literal
		}
	}
	return nil
}

func lerFactory(set *token.FileSet, literal *ast.CompositeLit) (Arquivo, error) {
	arquivo := Arquivo{Ruler: migrate.Ruler{Count: 10, Active: true}}
	campoVisto := false

	for _, elemento := range literal.Elts {
		par, ok := elemento.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch astparser.IdentName(par.Key) {
		case "Table":
			tabela, err := nomeDeTabela(par.Value)
			if err != nil {
				return arquivo, err
			}
			arquivo.Tabela = tabela
		case "Ruler":
			ruler, err := lerRuler(par.Value)
			if err != nil {
				return arquivo, err
			}
			arquivo.Ruler = ruler
		case "Data":
			campos, err := lerData(set, par.Value)
			if err != nil {
				return arquivo, err
			}
			arquivo.Campos = campos
			campoVisto = true
		}
	}

	if arquivo.Tabela == "" {
		return arquivo, i18n.Errf("fcp_no_table")
	}
	if !campoVisto {
		return arquivo, i18n.Errf("fcp_no_data")
	}
	if len(arquivo.Campos) == 0 {
		return arquivo, i18n.Errf("fcp_empty_data")
	}
	return arquivo, nil
}

// lerRuler lê o Ruler do arquivo. Chave desconhecida é ignorada — inclusive o
// antigo `Update`, que deixou de existir: o arquivo de um projeto velho não
// compila mais, mas `factory create` consegue lê-lo e reescrever a linha.
func lerRuler(expressao ast.Expr) (migrate.Ruler, error) {
	ruler := migrate.Ruler{Count: 10, Active: true}
	literal, ok := expressao.(*ast.CompositeLit)
	if !ok {
		return ruler, i18n.Errf("fcp_ruler_shape")
	}
	for _, elemento := range literal.Elts {
		par, ok := elemento.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		nome := astparser.IdentName(par.Key)
		switch nome {
		case "Count":
			quantidade, err := astparser.IntLiteral(par.Value)
			if err != nil {
				return ruler, i18n.Errf("fcp_ruler_count")
			}
			ruler.Count = quantidade
		case "Active":
			valor := astparser.IdentName(par.Value)
			if valor != "true" && valor != "false" {
				return ruler, i18n.Errf("fcp_ruler_bool", nome)
			}
			ruler.Active = valor == "true"
		}
	}
	return ruler, nil
}

// lerData avalia o Data da factory.
//
// A forma aceita é exatamente esta:
//
//	Data: migrate.Fields{ ... }
//
// Era uma closure `func(index int) migrate.Fields`, e o índice implícito tornou o
// invólucro inútil: nada dentro do mapa referencia o parâmetro.
func lerData(set *token.FileSet, expressao ast.Expr) ([]Campo, error) {
	retorno, ok := expressao.(*ast.CompositeLit)
	if !ok {
		return nil, i18n.Errf("fcp_data_signature")
	}

	ambiente := ambienteDeAvaliacao{set: set}
	campos := make([]Campo, 0, len(retorno.Elts))
	vistas := map[string]bool{}

	for _, elemento := range retorno.Elts {
		par, ok := elemento.(*ast.KeyValueExpr)
		if !ok {
			return nil, i18n.Errf("fcp_data_line_shape", posicao(set, elemento.Pos()))
		}
		coluna, ok := nomeDeColuna(par.Key)
		if !ok {
			return nil, i18n.Errf("fcp_data_column_quoted", posicao(set, par.Key.Pos()))
		}
		if vistas[strings.ToUpper(coluna)] {
			return nil, i18n.Errf("fcp_data_dup_column", coluna)
		}
		vistas[strings.ToUpper(coluna)] = true

		campo, err := ambiente.campo(coluna, par.Value)
		if err != nil {
			return nil, i18n.Errf("fcp_data_column_wrap", posicao(set, par.Value.Pos()), coluna, err)
		}
		campos = append(campos, campo)
	}
	return campos, nil
}

// ── Referências de catálogo ──
//
// A factory pode endereçar tabela e coluna de duas formas:
//
//	Table: "CIDADES"                    texto — casado por caixa contra a migration
//	Table: core.Table.Cidades           referência de catálogo
//
//	"nome":                    …        texto
//	core.Column.Cidades.Nome:  …        referência de catálogo
//
// As duas convivem porque o valor gerado é o mesmo: o catálogo guarda o nome
// físico, e o texto solto já era casado por caixa. O ganho da referência é o
// compilador — nome errado deixa de compilar em vez de falhar no `factory
// validate`.
//
// O identificador do pacote NÃO é validado, só o agrupador (`Table`/`Column`):
// o apelido do import é livre, como no resto do gokit.

// nomeDeTabela aceita literal ou `X.Table.Nome`.
func nomeDeTabela(expressao ast.Expr) (string, error) {
	if nome, err := astparser.StringLiteral(expressao); err == nil {
		return nome, nil
	}
	if nome, ok := referenciaDeGrupo(expressao, "Table"); ok {
		return nome, nil
	}
	return "", i18n.Errf("fcp_table_quoted")
}

// nomeDeColuna aceita literal ou `X.Column.Tabela.Coluna`.
//
// O agrupador de coluna tem um nível a mais que o de tabela, porque a coluna é
// endereçada dentro da tabela dela.
func nomeDeColuna(expressao ast.Expr) (string, bool) {
	if nome, err := astparser.StringLiteral(expressao); err == nil {
		return nome, true
	}
	seletor, ok := expressao.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	// X.Column.Tabela.Coluna → o pai do seletor é X.Column.Tabela
	tabelaSeletor, ok := seletor.X.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	if grupo, ok := tabelaSeletor.X.(*ast.SelectorExpr); !ok || grupo.Sel.Name != "Column" {
		return "", false
	}
	return seletor.Sel.Name, true
}

// referenciaDeGrupo resolve `X.<grupo>.Nome` e devolve o Nome.
func referenciaDeGrupo(expressao ast.Expr, grupo string) (string, bool) {
	seletor, ok := expressao.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	interno, ok := seletor.X.(*ast.SelectorExpr)
	if !ok || interno.Sel.Name != grupo {
		return "", false
	}
	if astparser.IdentName(interno.X) == "" {
		return "", false
	}
	return seletor.Sel.Name, true
}

// ambienteDeAvaliacao carrega o que o corpo de Data pode referenciar.
type ambienteDeAvaliacao struct {
	set *token.FileSet
}

func (ambiente ambienteDeAvaliacao) campo(coluna string, expressao ast.Expr) (Campo, error) {
	origem := textoDaExpressao(ambiente.set, expressao)

	// A referência é resolvida pelo executor, não aqui. Reference e Seeder são as
	// duas formas: a primeira aceita qualquer linha do pai, a segunda restringe a
	// valores específicos.
	if chamada, ok := expressao.(*ast.CallExpr); ok {
		if seletor, ok := chamada.Fun.(*ast.SelectorExpr); ok {
			var (
				link *migrate.Link
				err  error
			)
			switch seletor.Sel.Name {
			case "Reference":
				link, err = ambiente.referencia(chamada)
			case "Seeder":
				link, err = ambiente.seeder(chamada)
			}
			if err != nil {
				return Campo{}, err
			}
			if link != nil {
				return Campo{Coluna: coluna, Referencia: link, Origem: origem}, nil
			}
		}
	}

	// Avalia uma vez com índice 0 para que um nome errado ou um argumento de
	// tipo trocado apareça na leitura, e não no meio de um lote.
	if _, err := ambiente.avaliar(expressao, 0); err != nil {
		return Campo{}, err
	}

	return Campo{
		Coluna: coluna,
		Origem: origem,
		Valor:  func(index int) (any, error) { return ambiente.avaliar(expressao, index) },
	}, nil
}

func (ambiente ambienteDeAvaliacao) referencia(chamada *ast.CallExpr) (*migrate.Link, error) {
	// Um argumento (só a tabela) ou dois (tabela + coluna, ou tabela + restrição).
	if len(chamada.Args) < 1 || len(chamada.Args) > 2 {
		return nil, i18n.Errf("fcp_link_args")
	}
	tabela, err := nomeDeTabela(chamada.Args[0])
	if err != nil {
		return nil, i18n.Errf("fcp_link_table_quoted")
	}
	// O segundo argumento é opcional: sem ele, a coluna sai da FK declarada na
	// migration. Com ele, é a coluna do pai ou uma restrição de valores.
	if len(chamada.Args) == 1 {
		link := migrate.Reference(migrate.Table(tabela))
		return &link, nil
	}

	coluna, ok := nomeDeColuna(chamada.Args[1])
	if !ok {
		return nil, i18n.Errf("fcp_link_column_quoted")
	}
	link := migrate.Reference(migrate.Table(tabela), migrate.ColumnName(coluna))
	return &link, nil
}

// seeder lê migrate.Seeder(tabela, valores...): a referência apontando para
// valores específicos do pai.
//
// Os valores têm de ser literais, como todo valor do corpo de Data — o avaliador
// não executa código. Número e texto valem os dois: 1 e "1" são o mesmo alvo.
func (ambiente ambienteDeAvaliacao) seeder(chamada *ast.CallExpr) (*migrate.Link, error) {
	if len(chamada.Args) < 2 {
		return nil, i18n.Errf("fcp_seeder_args")
	}
	tabela, err := nomeDeTabela(chamada.Args[0])
	if err != nil {
		return nil, i18n.Errf("fcp_link_table_quoted")
	}
	valores := make([]any, 0, len(chamada.Args)-1)
	for _, argumento := range chamada.Args[1:] {
		valor, err := ambiente.avaliar(argumento, 0)
		if err != nil {
			return nil, i18n.Errf("fcp_seeder_literal")
		}
		valores = append(valores, valor)
	}
	link := migrate.Seeder(migrate.Table(tabela), valores...)
	return &link, nil
}

// avaliar resolve uma expressão de valor para a linha `index`.
func (ambiente ambienteDeAvaliacao) avaliar(expressao ast.Expr, index int) (any, error) {
	switch valor := expressao.(type) {
	case *ast.BasicLit:
		switch valor.Kind {
		case token.STRING:
			return strconv.Unquote(valor.Value)
		case token.INT:
			return strconv.ParseInt(valor.Value, 0, 64)
		case token.FLOAT:
			return strconv.ParseFloat(valor.Value, 64)
		}
		return nil, i18n.Errf("fcp_literal_unsupported", valor.Value)

	case *ast.Ident:
		switch valor.Name {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil":
			return nil, nil
		}
		// `index` não existe mais aqui: o índice é implícito e nenhuma função do
		// vocabulário o recebe.
		return nil, i18n.Errf("fcp_ident_unsupported", valor.Name)

	case *ast.UnaryExpr:
		if valor.Op != token.SUB {
			return nil, i18n.Errf("fcp_operator_unsupported")
		}
		interno, err := ambiente.avaliar(valor.X, index)
		if err != nil {
			return nil, err
		}
		switch numero := interno.(type) {
		case int64:
			return -numero, nil
		case int:
			return -numero, nil
		case float64:
			return -numero, nil
		}
		return nil, i18n.Errf("fcp_negative_numbers_only")

	case *ast.CallExpr:
		return ambiente.chamar(valor, index)
	}
	return nil, i18n.Errf("fcp_expr_unsupported")
}

func (ambiente ambienteDeAvaliacao) chamar(chamada *ast.CallExpr, index int) (any, error) {
	argumentos := make([]any, 0, len(chamada.Args))
	for _, argumento := range chamada.Args {
		valor, err := ambiente.avaliar(argumento, index)
		if err != nil {
			return nil, err
		}
		argumentos = append(argumentos, valor)
	}

	// Só chamada qualificada (migrate.X) é aceita. Antes havia o acessor de
	// localidade, `local("uf", 2)`, que exigia uma declaração antes do return —
	// e era o único motivo que sustentava a closure. As funções independentes
	// (FakeUF, FakeCity, FakeState…) derivam do mesmo índice, então mantêm a
	// localidade coerente na linha sem precisar de acessor.
	seletor, ok := chamada.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, i18n.Errf("fcp_call_unknown")
	}
	nome := seletor.Sel.Name

	funcao, conhecida := vocabulario[nome]
	if !conhecida {
		if sugestao := sugestaoDeNome(nome); sugestao != "" {
			return nil, i18n.Errf("fcp_not_in_vocabulary_hint", nome, sugestao)
		}
		return nil, i18n.Errf("fcp_not_in_vocabulary", nome)
	}

	// O índice não vem dos argumentos escritos: ele é injetado aqui.
	resultado, err := funcao(index, argumentos)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", nome, err)
	}
	return resultado, nil
}

func posicao(set *token.FileSet, pos token.Pos) string {
	if set == nil {
		return "?"
	}
	return fmt.Sprintf("linha %d", set.Position(pos).Line)
}

// textoDaExpressao reimprime a expressão a partir do AST. É o que permite ao
// gerador reescrever o arquivo sem perder o que foi ajustado à mão.
func textoDaExpressao(set *token.FileSet, expressao ast.Expr) string {
	var buffer bytes.Buffer
	if err := printer.Fprint(&buffer, set, expressao); err != nil {
		return "?"
	}
	return buffer.String()
}
