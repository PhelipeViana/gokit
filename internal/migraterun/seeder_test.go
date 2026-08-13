package migraterun

import (
	"fmt"
	"testing"

	migrate "github.com/PhelipeViana/gokit/migration"
)

// O Seeder é escolhido pelo índice, então a ordem do resultado não pode vir do
// banco: cada dialeto entrega as linhas numa ordem própria e a mesma factory
// passaria a apontar para alvos diferentes em cada um.
func TestNaOrdemDeclarada(t *testing.T) {
	casos := []struct {
		nome        string
		listados    []any
		encontrados []any
		esperado    string
	}{
		{"segue o arquivo, não o banco", []any{1, 2, 15}, []any{15, 1, 2}, "1 2 15"},
		{"descarta o que não existe", []any{1, 2, 15}, []any{2}, "2"},
		{"número e texto são o mesmo alvo", []any{"2", 1}, []any{int64(1), int64(2)}, "2 1"},
		{"nada existe", []any{99}, []any{}, ""},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			obtido := fmt.Sprint(naOrdemDeclarada(caso.listados, caso.encontrados)...)
			if obtido != caso.esperado {
				t.Fatalf("esperava %q, obteve %q", caso.esperado, obtido)
			}
		})
	}
}

// Reference sem valores não restringe nada — a distinção entre os dois vínculos
// é o que decide se o caminho do banco filtra ou não.
func TestValoresListados(t *testing.T) {
	if valores := valoresListados(nil); valores != nil {
		t.Fatalf("vínculo nulo não deveria restringir, obteve %v", valores)
	}

	referencia := migrate.Reference("USERS")
	if valores := valoresListados(&referencia); len(valores) != 0 {
		t.Fatalf("Reference não deveria restringir, obteve %v", valores)
	}

	seeder := migrate.Seeder("USERS", 1, "2")
	if valores := valoresListados(&seeder); len(valores) != 2 {
		t.Fatalf("Seeder deveria listar 2 valores, obteve %v", valores)
	}
}
