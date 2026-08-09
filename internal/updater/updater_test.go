package updater

import "testing"

func TestCheckDevelopmentNaoConsultaAtualizacao(t *testing.T) {
	status := Check("development")
	if status.Available || status.Error != nil {
		t.Fatalf("build local não deve oferecer atualização: %+v", status)
	}
}

func TestShortHash(t *testing.T) {
	if got := shortHash("1234567890"); got != "1234567" {
		t.Fatalf("hash curto inesperado: %s", got)
	}
}
