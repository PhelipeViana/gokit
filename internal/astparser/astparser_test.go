package astparser

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestEvaluateCallChain(t *testing.T) {
	src := `package test
var _ = col("id").Varchar(255).PrimaryKey()
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("falha ao parsear código: %v", err)
	}

	// Extrai a expressão col("id").Varchar(255).PrimaryKey()
	var expr ast.Expr
	for _, decl := range f.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok {
					expr = value.Values[0]
				}
			}
		}
	}

	if expr == nil {
		t.Fatalf("expressão de teste não encontrada no AST")
	}

	chain, err := EvaluateCallChain(expr)
	if err != nil {
		t.Fatalf("erro ao avaliar cadeia: %v", err)
	}

	if chain.RootFunc != "col" {
		t.Errorf("RootFunc esperado 'col', obtido: %s", chain.RootFunc)
	}

	if len(chain.RootArgs) != 1 || chain.RootArgs[0] != "id" {
		t.Errorf("RootArgs esperados ['id'], obtidos: %v", chain.RootArgs)
	}

	if len(chain.Calls) != 2 {
		t.Fatalf("esperados 2 métodos encadeados, obtidos: %d", len(chain.Calls))
	}

	if chain.Calls[0].Method != "Varchar" || len(chain.Calls[0].Args) != 1 || chain.Calls[0].Args[0] != 255 {
		t.Errorf("primeiro método incorreto: %v", chain.Calls[0])
	}

	if chain.Calls[1].Method != "PrimaryKey" || len(chain.Calls[1].Args) != 0 {
		t.Errorf("segundo método incorreto: %v", chain.Calls[1])
	}
}
