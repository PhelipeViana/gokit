package main

import (
	"strings"
	"testing"
)

// Panic é sempre defeito do motor, nunca erro de uso: o gokit deveria ter tratado a
// condição. Por isso vira danger, e por isso o STACK vai no relatório — sem a linha
// onde quebrou, o relatório não resolve nada.
//
// A montagem do aviso é separada da recuperação porque a recuperação chama os.Exit, e
// teste que sai do processo não afirma nada.
func TestAvisoDePanicLevaOStackEViraDanger(t *testing.T) {
	pilha := "goroutine 1 [running]:\nmain.quebra(...)\n\t/app/main.go:42 +0x1d"
	aviso := avisoDePanic("migrate run", "index out of range [3] with length 2", pilha)

	if aviso.Nivel != "danger" {
		t.Errorf("panic deveria ser danger, veio %q", aviso.Nivel)
	}
	if aviso.Bruto != pilha {
		t.Error("o stack tem de ir cru no relatório: é ele que diz a linha")
	}
	if !strings.Contains(aviso.Contexto["panic"], "index out of range") {
		t.Errorf("o contexto deveria trazer o valor do panic, veio %q", aviso.Contexto["panic"])
	}
	if !strings.Contains(aviso.Corpo, "defeito do motor") {
		t.Error("o corpo deveria dizer que panic não é erro de uso")
	}
	if !strings.Contains(aviso.Contexto["comando"], "migrate run") {
		t.Error("o comando é o que permite reproduzir")
	}
}
