package migraterun

import (
	"strings"
	"testing"

	"github.com/PhelipeViana/gokit/internal/config"
)

func TestMigrationChangedAdviceDoesNotBlameConnection(t *testing.T) {
	diagnosis, solution := migrationConnectionAdvice(config.ConnConfig{}, MigrationChangedError{
		Name: "2026_08_08_193251_cidade.go",
	})

	if !strings.Contains(diagnosis, "conexão com o banco está funcionando") {
		t.Fatalf("diagnóstico deveria distinguir checksum de conexão: %q", diagnosis)
	}
	for _, expected := range []string{"restaure o arquivo original", "nova migration", "Reload Fresh", "nunca foram executados"} {
		if !strings.Contains(solution, expected) {
			t.Errorf("solução não contém %q: %q", expected, solution)
		}
	}
}
