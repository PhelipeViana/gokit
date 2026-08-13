package migraterun

import (
	"context"
	"testing"

	"github.com/PhelipeViana/gokit/migration/acao"
)

// migrate.TODO() é o que o scaffold escreve numa migration nova e ainda vazia, e
// o parser o traduz para uma operação de tipo "todo". Ela precisa ser um no-op no
// executor: quem cria a migration e roda antes de preencher não pode receber
// "operação desconhecida".
//
// O caminho não toca no banco nem no cache, então db/cache nulos bastam.
func TestOperacaoTodoNaoQuebraOExecutor(t *testing.T) {
	err := executeOperation(context.Background(), nil, "postgres", "",
		acao.Operacao{Kind: string(acao.Todo)}, map[string]bool{}, nil)
	if err != nil {
		t.Fatalf("a operação todo deveria ser no-op, veio: %v", err)
	}
}
