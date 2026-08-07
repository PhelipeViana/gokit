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

	"gokit/internal/astparser"
	migrate "gokit/migration"
)

// Campo é uma coluna da factory já resolvida para uma forma executável.
type Campo struct {
	Coluna string
	// Valor gera o dado da linha. É nil quando o campo é um vínculo.
	Valor func(index int) (any, error)
	// Vinculo aponta para a coluna de outra tabela; o executor resolve
	// contra as linhas realmente inseridas lá.
	Vinculo *migrate.Link
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
		if campo.Vinculo != nil {
			continue
		}
		valor, err := campo.Valor(index)
		if err != nil {
			return nil, fmt.Errorf("coluna %s: %w", campo.Coluna, err)
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

// Vinculos lista as tabelas pai referenciadas por esta factory.
func (arquivo Arquivo) Vinculos() []string {
	vistas := map[string]bool{}
	var tabelas []string
	for _, campo := range arquivo.Campos {
		if campo.Vinculo == nil {
			continue
		}
		alvo := strings.ToUpper(campo.Vinculo.Table)
		if !vistas[alvo] {
			vistas[alvo] = true
			tabelas = append(tabelas, alvo)
		}
	}
	sort.Strings(tabelas)
	return tabelas
}

// CarregarPasta lê todos os *_factory.go de uma pasta.
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
		if entrada.IsDir() || !strings.HasSuffix(entrada.Name(), "_factory.go") {
			continue
		}
		caminho := filepath.Join(pasta, entrada.Name())
		arquivo, err := ParseArquivo(caminho)
		if err != nil {
			problemas = append(problemas, fmt.Sprintf("%s: %v", entrada.Name(), err))
			continue
		}
		arquivos = append(arquivos, arquivo)
	}

	sort.Slice(arquivos, func(i, j int) bool { return arquivos[i].Tabela < arquivos[j].Tabela })
	if len(problemas) > 0 {
		return arquivos, fmt.Errorf("%d factory(ies) com problema:\n  - %s", len(problemas), strings.Join(problemas, "\n  - "))
	}
	return arquivos, nil
}

// ParseArquivo lê uma factory e resolve todas as expressões do corpo de Data.
// Erros de vocabulário aparecem aqui, na leitura — não no meio de um INSERT.
func ParseArquivo(caminho string) (Arquivo, error) {
	file, set, err := astparser.ParseFile(caminho)
	if err != nil {
		return Arquivo{}, err
	}

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
			return Arquivo{}, err
		}
		arquivo.Caminho = caminho
		arquivo.Funcao = funcao.Name.Name
		return arquivo, nil
	}
	return Arquivo{}, fmt.Errorf("nenhuma func ...Factory() migrate.Factory encontrada")
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
	arquivo := Arquivo{Ruler: migrate.Ruler{Count: 10, Update: true, Active: true}}
	campoVisto := false

	for _, elemento := range literal.Elts {
		par, ok := elemento.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch astparser.IdentName(par.Key) {
		case "Table":
			tabela, err := astparser.StringLiteral(par.Value)
			if err != nil {
				return arquivo, fmt.Errorf("Table precisa ser um texto entre aspas")
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
		return arquivo, fmt.Errorf("a factory não declara Table")
	}
	if !campoVisto {
		return arquivo, fmt.Errorf("a factory não declara Data")
	}
	if len(arquivo.Campos) == 0 {
		return arquivo, fmt.Errorf("o Data da factory está vazio")
	}
	return arquivo, nil
}

func lerRuler(expressao ast.Expr) (migrate.Ruler, error) {
	ruler := migrate.Ruler{Count: 10, Update: true, Active: true}
	literal, ok := expressao.(*ast.CompositeLit)
	if !ok {
		return ruler, fmt.Errorf("Ruler precisa ser migrate.Ruler{...}")
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
				return ruler, fmt.Errorf("Ruler.Count precisa ser um número inteiro")
			}
			ruler.Count = quantidade
		case "Update", "Active":
			valor := astparser.IdentName(par.Value)
			if valor != "true" && valor != "false" {
				return ruler, fmt.Errorf("Ruler.%s precisa ser true ou false", nome)
			}
			if nome == "Update" {
				ruler.Update = valor == "true"
			} else {
				ruler.Active = valor == "true"
			}
		}
	}
	return ruler, nil
}

// lerData avalia o corpo da closure de Data.
//
// A forma aceita é exatamente esta:
//
//	func(index int) migrate.Fields {
//	    local := migrate.FakeLocation()   // opcional
//	    return migrate.Fields{ ... }
//	}
func lerData(set *token.FileSet, expressao ast.Expr) ([]Campo, error) {
	closure, ok := expressao.(*ast.FuncLit)
	if !ok {
		return nil, fmt.Errorf("Data precisa ser func(index int) migrate.Fields { return migrate.Fields{...} }")
	}

	nomeIndice := parametroDoIndice(closure)
	if nomeIndice == "" {
		return nil, fmt.Errorf("Data precisa receber o índice da linha: func(index int) migrate.Fields")
	}

	// Acessores de localidade declarados antes do return, do tipo
	// `local := migrate.FakeLocation()`.
	acessores := map[string]bool{}
	var retorno *ast.CompositeLit

	for _, statement := range closure.Body.List {
		switch tipo := statement.(type) {
		case *ast.AssignStmt:
			nome, err := lerAcessorDeLocalidade(set, tipo)
			if err != nil {
				return nil, err
			}
			acessores[nome] = true
		case *ast.ReturnStmt:
			if len(tipo.Results) != 1 {
				return nil, fmt.Errorf("o return de Data precisa devolver um único migrate.Fields{...}")
			}
			literal, ok := tipo.Results[0].(*ast.CompositeLit)
			if !ok {
				return nil, fmt.Errorf("o return de Data precisa ser migrate.Fields{...}")
			}
			retorno = literal
		default:
			return nil, fmt.Errorf("%s: só `nome := migrate.FakeLocation()` e o return são aceitos dentro de Data", posicao(set, statement.Pos()))
		}
	}

	if retorno == nil {
		return nil, fmt.Errorf("Data não tem return")
	}

	ambiente := ambienteDeAvaliacao{indice: nomeIndice, acessores: acessores, set: set}
	campos := make([]Campo, 0, len(retorno.Elts))
	vistas := map[string]bool{}

	for _, elemento := range retorno.Elts {
		par, ok := elemento.(*ast.KeyValueExpr)
		if !ok {
			return nil, fmt.Errorf("%s: cada linha de Data precisa ser \"COLUNA\": valor", posicao(set, elemento.Pos()))
		}
		coluna, err := astparser.StringLiteral(par.Key)
		if err != nil {
			return nil, fmt.Errorf("%s: o nome da coluna precisa estar entre aspas", posicao(set, par.Key.Pos()))
		}
		if vistas[strings.ToUpper(coluna)] {
			return nil, fmt.Errorf("a coluna %s aparece duas vezes em Data", coluna)
		}
		vistas[strings.ToUpper(coluna)] = true

		campo, err := ambiente.campo(coluna, par.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: coluna %s: %w", posicao(set, par.Value.Pos()), coluna, err)
		}
		campos = append(campos, campo)
	}
	return campos, nil
}

func parametroDoIndice(closure *ast.FuncLit) string {
	if closure.Type == nil || closure.Type.Params == nil {
		return ""
	}
	for _, campo := range closure.Type.Params.List {
		if astparser.IdentName(campo.Type) != "int" {
			continue
		}
		if len(campo.Names) > 0 {
			return campo.Names[0].Name
		}
	}
	return ""
}

// lerAcessorDeLocalidade valida `nome := migrate.FakeLocation()`.
func lerAcessorDeLocalidade(set *token.FileSet, atribuicao *ast.AssignStmt) (string, error) {
	erro := fmt.Errorf("%s: a única atribuição aceita dentro de Data é `nome := migrate.FakeLocation()`", posicao(set, atribuicao.Pos()))
	if len(atribuicao.Lhs) != 1 || len(atribuicao.Rhs) != 1 {
		return "", erro
	}
	nome := astparser.IdentName(atribuicao.Lhs[0])
	if nome == "" {
		return "", erro
	}
	chamada, ok := atribuicao.Rhs[0].(*ast.CallExpr)
	if !ok || len(chamada.Args) != 0 {
		return "", erro
	}
	seletor, ok := chamada.Fun.(*ast.SelectorExpr)
	if !ok || seletor.Sel.Name != "FakeLocation" {
		return "", erro
	}
	return nome, nil
}

// ambienteDeAvaliacao carrega o que o corpo de Data pode referenciar.
type ambienteDeAvaliacao struct {
	indice    string
	acessores map[string]bool
	set       *token.FileSet
}

func (ambiente ambienteDeAvaliacao) campo(coluna string, expressao ast.Expr) (Campo, error) {
	origem := textoDaExpressao(ambiente.set, expressao)

	// Vinculo é resolvido pelo executor, não aqui.
	if chamada, ok := expressao.(*ast.CallExpr); ok {
		if seletor, ok := chamada.Fun.(*ast.SelectorExpr); ok && seletor.Sel.Name == "Vinculo" {
			link, err := ambiente.vinculo(chamada)
			if err != nil {
				return Campo{}, err
			}
			return Campo{Coluna: coluna, Vinculo: link, Origem: origem}, nil
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

func (ambiente ambienteDeAvaliacao) vinculo(chamada *ast.CallExpr) (*migrate.Link, error) {
	if len(chamada.Args) != 2 {
		return nil, fmt.Errorf("Vinculo exige a tabela e a coluna: Vinculo(\"TABELA\", \"COLUNA\")")
	}
	tabela, err := astparser.StringLiteral(chamada.Args[0])
	if err != nil {
		return nil, fmt.Errorf("o primeiro argumento de Vinculo precisa ser o nome da tabela entre aspas")
	}
	coluna, err := astparser.StringLiteral(chamada.Args[1])
	if err != nil {
		return nil, fmt.Errorf("o segundo argumento de Vinculo precisa ser o nome da coluna entre aspas")
	}
	link := migrate.Vinculo(tabela, coluna)
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
		return nil, fmt.Errorf("literal não suportado: %s", valor.Value)

	case *ast.Ident:
		switch valor.Name {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil":
			return nil, nil
		case ambiente.indice:
			return index, nil
		}
		return nil, fmt.Errorf("%s não existe aqui; use um literal, %s ou uma função migrate.Fake*", valor.Name, ambiente.indice)

	case *ast.UnaryExpr:
		if valor.Op != token.SUB {
			return nil, fmt.Errorf("operador não suportado em valor de factory")
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
		return nil, fmt.Errorf("o sinal negativo só vale para números")

	case *ast.CallExpr:
		return ambiente.chamar(valor, index)
	}
	return nil, fmt.Errorf("expressão não suportada; use um literal ou uma função migrate.Fake*")
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

	// Acessor de localidade: local(index, "uf", 2).
	if nome := astparser.IdentName(chamada.Fun); nome != "" {
		if !ambiente.acessores[nome] {
			return nil, fmt.Errorf("%s não foi declarado; use `%s := migrate.FakeLocation()` antes do return", nome, nome)
		}
		return chamarLocalidade(argumentos)
	}

	seletor, ok := chamada.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, fmt.Errorf("chamada não reconhecida")
	}
	nome := seletor.Sel.Name

	if nome == "FakeLocation" {
		return nil, fmt.Errorf("FakeLocation() precisa ser guardada antes do return: `local := migrate.FakeLocation()` e depois `local(%s, \"uf\", 2)`", ambiente.indice)
	}

	funcao, conhecida := vocabulario[nome]
	if !conhecida {
		if sugestao := sugestaoDeNome(nome); sugestao != "" {
			return nil, fmt.Errorf("%s não existe no vocabulário das factories; você quis dizer %s?", nome, sugestao)
		}
		return nil, fmt.Errorf("%s não existe no vocabulário das factories", nome)
	}

	resultado, err := funcao(argumentos)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", nome, err)
	}
	return resultado, nil
}

// chamarLocalidade executa o acessor devolvido por FakeLocation.
func chamarLocalidade(argumentos []any) (any, error) {
	if len(argumentos) < 2 || len(argumentos) > 3 {
		return nil, fmt.Errorf("o acessor de localidade exige (índice, campo) e aceita um tamanho: local(index, \"cidade\", 60)")
	}
	indice, err := inteiroEm(argumentos, 0)
	if err != nil {
		return nil, err
	}
	campo, err := textoEm(argumentos, 1)
	if err != nil {
		return nil, err
	}
	acessor := migrate.FakeLocation()
	if len(argumentos) == 2 {
		return acessor(indice, campo), nil
	}
	tamanho, err := inteiroEm(argumentos, 2)
	if err != nil {
		return nil, err
	}
	return acessor(indice, campo, tamanho), nil
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
