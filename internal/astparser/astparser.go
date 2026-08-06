// Package astparser provides reusable utilities to parse and evaluate Go AST expressions.
package astparser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// Call representa uma chamada de método em uma cadeia com seu nome e argumentos avaliados.
type Call struct {
	Method string
	Args   []any
}

// CallChain representa uma cadeia completa de chamadas (ex: col("id").Varchar(255).PrimaryKey()).
type CallChain struct {
	RootFunc string
	RootArgs []any
	Calls    []Call // Métodos encadeados da esquerda para a direita
}

// ParseFile abre e parseia um arquivo Go em um AST.
func ParseFile(path string) (*ast.File, *token.FileSet, error) {
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, path, nil, 0)
	if err != nil {
		return nil, nil, err
	}
	return file, fs, nil
}

// IdentName extrai o nome do identificador (se a expressão for um ast.Ident).
func IdentName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// StringLiteral extrai um valor string literal de uma expressão.
func StringLiteral(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", fmt.Errorf("esperado texto entre aspas")
	}
	return strconv.Unquote(lit.Value)
}

// IntLiteral extrai um valor inteiro de uma expressão.
func IntLiteral(expr ast.Expr) (int, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, fmt.Errorf("esperado número inteiro")
	}
	return strconv.Atoi(lit.Value)
}

// EvaluateArg avalia um argumento de expressão básico em um tipo primitivo (string, int, bool, ou nil).
func EvaluateArg(expr ast.Expr) any {
	if lit, ok := expr.(*ast.BasicLit); ok {
		switch lit.Kind {
		case token.STRING:
			if val, err := strconv.Unquote(lit.Value); err == nil {
				return val
			}
		case token.INT:
			if val, err := strconv.Atoi(lit.Value); err == nil {
				return val
			}
		}
	}
	if ident, ok := expr.(*ast.Ident); ok {
		if ident.Name == "true" {
			return true
		}
		if ident.Name == "false" {
			return false
		}
	}
	return nil
}

// EvaluateCallChain recursivamente avalia uma cadeia de chamadas (ex: col("id").Varchar(255).PrimaryKey()).
func EvaluateCallChain(expr ast.Expr) (CallChain, error) {
	chain, err := evaluateCallChainRecursive(expr)
	if err != nil {
		return CallChain{}, err
	}
	return chain, nil
}

func evaluateCallChainRecursive(expr ast.Expr) (CallChain, error) {
	switch e := expr.(type) {
	case *ast.CallExpr:
		// Se for uma chamada direta de função (ex: col("id"))
		if ident, ok := e.Fun.(*ast.Ident); ok {
			args := make([]any, len(e.Args))
			for i, arg := range e.Args {
				args[i] = EvaluateArg(arg)
			}
			return CallChain{
				RootFunc: ident.Name,
				RootArgs: args,
			}, nil
		}

		// Se for uma chamada de seletor (ex: migrate.Col("id"))
		if selector, ok := e.Fun.(*ast.SelectorExpr); ok {
			if _, ok := selector.X.(*ast.Ident); ok {
				args := make([]any, len(e.Args))
				for i, arg := range e.Args {
					args[i] = EvaluateArg(arg)
				}
				return CallChain{
					RootFunc: selector.Sel.Name,
					RootArgs: args,
				}, nil
			}
		}

		// Se for um método encadeado (ex: chain.Method(args))
		if selector, ok := e.Fun.(*ast.SelectorExpr); ok {
			parentChain, err := evaluateCallChainRecursive(selector.X)
			if err != nil {
				return CallChain{}, err
			}
			args := make([]any, len(e.Args))
			for i, arg := range e.Args {
				args[i] = EvaluateArg(arg)
			}
			parentChain.Calls = append(parentChain.Calls, Call{
				Method: selector.Sel.Name,
				Args:   args,
			})
			return parentChain, nil
		}

	case *ast.SelectorExpr:
		// Uma referência a um seletor simples sem chamada (ex: alias.Tabela)
		if ident, ok := e.X.(*ast.Ident); ok {
			return CallChain{
				RootFunc: fmt.Sprintf("%s.%s", ident.Name, e.Sel.Name),
			}, nil
		}
	}

	return CallChain{}, fmt.Errorf("expressão não reconhecida como cadeia de chamadas")
}
