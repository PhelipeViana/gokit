// Package db é o runtime PÚBLICO de conexão do GoKit, usado pela aplicação
// cliente para executar as pesquisas do ORM.
//
// Ele é a fachada que resolve a conexão ativa (dialeto, schema e DSN) a partir
// do gokit.json/.env — a mesma lógica das migrations/seeds — e a entrega ao
// motor orm. O cliente é outro módulo e não pode importar internal/config; este
// pacote pode (mesmo módulo do gokit), então concentra aqui a ponte.
//
// Modelo híbrido:
//   - db.Connect(ctx)  → resolve e instala a conexão PADRÃO global; depois é só
//     orm.Users.DB.Where(...).Get(ctx).
//   - db.Open(ctx)     → resolve e devolve uma sessão para uso explícito.
//   - db.Use(sqlDB...) → embrulha um *sql.DB já aberto (transação, pool próprio,
//     múltiplos bancos), passado aos métodos *With.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/orm"
)

// Session é uma conexão ativa que satisfaz orm.Runner. Embute *sql.DB, então
// herda QueryContext/QueryRowContext (o pool do database/sql).
type Session struct {
	*sql.DB
	dialect orm.Dialect
	schema  string
}

func (s *Session) Dialect() orm.Dialect { return s.dialect }
func (s *Session) Schema() string       { return s.schema }

// Use embrulha um *sql.DB já aberto como sessão (metade explícita do híbrido).
func Use(conn *sql.DB, dialect orm.Dialect, schema string) *Session {
	return &Session{DB: conn, dialect: dialect, schema: schema}
}

// Open resolve a conexão ativa do gokit.json/.env, abre o pool e faz ping.
func Open(ctx context.Context) (*Session, error) {
	state := config.RunConfigChecks()
	if state.ConfigFileError != nil {
		return nil, state.ConfigFileError
	}
	conn, ok := state.Config.Connections[state.ActiveClient]
	if !ok {
		return nil, fmt.Errorf("conexão ativa %q não encontrada no gokit.json", state.ActiveClient)
	}
	sqlDB, err := sql.Open(driverName(state.ActiveDialect), state.ActiveURL)
	if err != nil {
		return nil, fmt.Errorf("não foi possível abrir a conexão %s: %w", state.ActiveDialect, err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("sem resposta do banco %s: %w", state.ActiveDialect, err)
	}
	return Use(sqlDB, orm.Dialect(strings.ToLower(state.ActiveDialect)), conn.Schema), nil
}

// Connect faz o Open e instala a sessão como conexão padrão global do ORM.
func Connect(ctx context.Context) (*Session, error) {
	session, err := Open(ctx)
	if err != nil {
		return nil, err
	}
	orm.UseDefault(session)
	return session, nil
}

// driverName mapeia o dialeto para o driver registrado (postgres → pgx).
// Espelha internal/migraterun/doctor.go:getDriverName.
func driverName(dialect string) string {
	d := strings.ToLower(strings.TrimSpace(dialect))
	if d == "postgres" || d == "postgresql" {
		return "pgx"
	}
	return d
}
