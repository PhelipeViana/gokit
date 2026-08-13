package orm

import (
	"context"
	"errors"
	"testing"
)

// Todo terminal promovido no Model precisa ter a variante *With.
//
// A assimetria custava um .All() no meio da autoria — u.All().CountWith(ctx, r) —
// e doía exatamente dentro de transação, que é onde se lê para decidir e gravar.
// Este teste existe para que um terminal novo não reintroduza a lacuna: se algum
// par faltar, ele falha na COMPILAÇÃO, o que é o lugar certo — a ausência de um
// método é fato de tipo, não de runtime.
func TestTodoTerminalDoModelTemVarianteWith(t *testing.T) {
	users := entidadeUsers()
	m := modelo(users)
	col := campo(users, "id")
	ctx := context.Background()

	// Sem conexão, todos falham com ErrNoConnection. O que se prova aqui é que
	// cada método existe com a assinatura esperada e chega ao mesmo lugar.
	terminais := map[string]func() error{
		"GetWith":         func() error { _, err := m.GetWith(ctx, nil); return err },
		"CountWith":       func() error { _, err := m.CountWith(ctx, nil); return err },
		"FirstWith":       func() error { _, err := m.FirstWith(ctx, nil); return err },
		"ExistsWith":      func() error { _, err := m.ExistsWith(ctx, nil); return err },
		"RowsWith":        func() error { _, err := m.RowsWith(ctx, nil); return err },
		"SumWith":         func() error { _, err := m.SumWith(ctx, nil, col); return err },
		"AvgWith":         func() error { _, err := m.AvgWith(ctx, nil, col); return err },
		"MinWith":         func() error { _, err := m.MinWith(ctx, nil, col); return err },
		"MaxWith":         func() error { _, err := m.MaxWith(ctx, nil, col); return err },
		"PaginateWith":    func() error { _, err := m.PaginateWith(ctx, nil, 1, 10); return err },
		"PluckWith":       func() error { _, err := m.PluckWith(ctx, nil, col); return err },
		"PluckIntWith":    func() error { _, err := m.PluckIntWith(ctx, nil, col); return err },
		"PluckStringWith": func() error { _, err := m.PluckStringWith(ctx, nil, col); return err },
		"ChunkWith":       func() error { return m.ChunkWith(ctx, nil, 10, func([]Record) error { return nil }) },
		"EachWith":        func() error { return m.EachWith(ctx, nil, func(Record) error { return nil }) },
	}
	for nome, chamar := range terminais {
		if err := chamar(); !errors.Is(err, ErrNoConnection) {
			t.Errorf("%s: esperado ErrNoConnection, veio: %v", nome, err)
		}
	}
}
