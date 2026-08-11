package orm_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	orm "github.com/PhelipeViana/gokit/orm"
)

// fakeRunner captura o SQL/args compilados sem tocar em banco. QueryContext
// devolve erro de propósito: queremos validar que a pesquisa foi compilada com
// o dialeto/schema DA CONEXÃO, não executá-la.
type fakeRunner struct {
	dialect orm.Dialect
	schema  string
	gotSQL  string
	gotArgs []any
}

func (f *fakeRunner) Dialect() orm.Dialect { return f.dialect }
func (f *fakeRunner) Schema() string       { return f.schema }

var errBoom = errors.New("boom")

func (f *fakeRunner) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	f.gotSQL = query
	f.gotArgs = args
	return nil, errBoom
}

func (f *fakeRunner) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	f.gotSQL = query
	f.gotArgs = args
	return nil
}

func TestGetWithCompilaComDialetoDaConexao(t *testing.T) {
	m, f := testModel()
	fr := &fakeRunner{dialect: orm.Postgres, schema: "public"}
	_, err := m.Where(f.Nome.Equal("silva")).GetWith(context.Background(), fr)
	if !errors.Is(err, errBoom) {
		t.Fatalf("esperava o erro do runner, veio: %v", err)
	}
	want := `SELECT "id", "nome", "idade", "ativo" FROM "public"."users" WHERE "nome" = $1`
	if fr.gotSQL != want {
		t.Fatalf("SQL compilado:\n got %q\nwant %q", fr.gotSQL, want)
	}
	if len(fr.gotArgs) != 1 || fr.gotArgs[0] != "silva" {
		t.Fatalf("args: %#v", fr.gotArgs)
	}
}

func TestSemConexaoRetornaErro(t *testing.T) {
	m, f := testModel()
	if _, err := m.Where(f.Id.Equal(1)).Get(context.Background()); !errors.Is(err, orm.ErrNoConnection) {
		t.Fatalf("esperava ErrNoConnection, veio: %v", err)
	}
}
