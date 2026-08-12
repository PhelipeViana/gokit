package gomodule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projetoComGoKitIrmao monta o layout que os dois modos assumem: o gokit numa
// pasta e o projeto ao lado.
func projetoComGoKitIrmao(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	framework := filepath.Join(root, "gokit")
	project := filepath.Join(root, "Minha Aplicacao")
	if err := os.MkdirAll(framework, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(framework, "go.mod"),
		[]byte("module "+CanonicalGoKitModule+"\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return project
}

func leia(t *testing.T, caminho string) string {
	t.Helper()
	dados, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatal(err)
	}
	return string(dados)
}

// O go.mod é igual nos dois modos: require de versão publicada, nunca replace.
// É o que torna o arquivo commitável sem quebrar a máquina de quem clonar.
func TestModoDevUsaGoWorkENuncaReplace(t *testing.T) {
	project := projetoComGoKitIrmao(t)

	result, err := Ensure(Options{Root: project, GoKitLocal: "../gokit", Mode: ModoDev})
	if err != nil {
		t.Fatal(err)
	}
	if result.Module != "minha-aplicacao" {
		t.Fatalf("modulo inesperado: %s", result.Module)
	}
	if result.Mode != ModoDev {
		t.Fatalf("modo inesperado: %s", result.Mode)
	}

	goMod := leia(t, filepath.Join(project, "go.mod"))
	for _, esperado := range []string{
		"module minha-aplicacao",
		"require " + CanonicalGoKitModule + " " + VersaoFixada,
	} {
		if !strings.Contains(goMod, esperado) {
			t.Errorf("go.mod não contém %q:\n%s", esperado, goMod)
		}
	}
	if strings.Contains(goMod, "replace") {
		t.Errorf("go.mod não deveria ter replace no modo dev:\n%s", goMod)
	}

	// O desvio local é um replace com versão dentro do go.work — e não um
	// `use ../gokit`, que resolveria o pacote mas não o grafo de módulos.
	goWork := leia(t, filepath.Join(project, "go.work"))
	for _, esperado := range []string{
		"use .",
		"replace " + CanonicalGoKitModule + " " + VersaoFixada + " => ../gokit",
	} {
		if !strings.Contains(goWork, esperado) {
			t.Errorf("go.work não contém %q:\n%s", esperado, goWork)
		}
	}
	if strings.Contains(goWork, "use (") {
		t.Errorf("o gokit não deve entrar como módulo do workspace:\n%s", goWork)
	}

	if _, err := Ensure(Options{Root: project, GoKitLocal: "../gokit", Mode: ModoDev}); err != nil {
		t.Fatalf("segunda execução deve ser idempotente: %v", err)
	}
}

// Trocar para prod tem de apagar o go.work: um workspace esquecido continuaria
// redirecionando o import para a pasta local, e o "prod" seria mentira.
func TestModoProdRemoveOGoWork(t *testing.T) {
	project := projetoComGoKitIrmao(t)

	if _, err := Ensure(Options{Root: project, GoKitLocal: "../gokit", Mode: ModoDev}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, "go.work")); err != nil {
		t.Fatal("go.work deveria existir depois do modo dev")
	}

	result, err := Ensure(Options{Root: project, GoKitLocal: "../gokit", Mode: ModoProd})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != ModoProd {
		t.Fatalf("modo inesperado: %s", result.Mode)
	}
	if _, err := os.Stat(filepath.Join(project, "go.work")); !os.IsNotExist(err) {
		t.Error("go.work deveria ter sido removido no modo prod")
	}
	goMod := leia(t, filepath.Join(project, "go.mod"))
	if !strings.Contains(goMod, "require "+CanonicalGoKitModule+" "+VersaoFixada) {
		t.Errorf("prod deve fixar a versão publicada:\n%s", goMod)
	}
}

// Workspace com outros módulos é trabalho do desenvolvedor: sai a entrada do
// gokit, ficam as dele.
func TestModoProdPreservaOutrosModulosNoGoWork(t *testing.T) {
	project := projetoComGoKitIrmao(t)
	outro := filepath.Join(project, "ferramenta")
	if err := os.MkdirAll(outro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outro, "go.mod"), []byte("module ferramenta\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.work"),
		[]byte("go 1.26\n\nuse (\n\t.\n\t../gokit\n\t./ferramenta\n)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Ensure(Options{Root: project, GoKitLocal: "../gokit", Mode: ModoProd}); err != nil {
		t.Fatal(err)
	}
	goWork := leia(t, filepath.Join(project, "go.work"))
	if strings.Contains(goWork, "../gokit") {
		t.Errorf("a entrada do gokit deveria ter saído:\n%s", goWork)
	}
	if !strings.Contains(goWork, "ferramenta") {
		t.Errorf("o módulo do desenvolvedor deveria ter ficado:\n%s", goWork)
	}
}

// Dev sem o gokit ao lado não tem como funcionar; falhar com uma frase que diz o
// que fazer é melhor que gerar um workspace apontando para o vazio.
func TestModoDevSemGoKitIrmaoFalha(t *testing.T) {
	_, err := Ensure(Options{Root: t.TempDir(), GoKitLocal: "../inexistente", Mode: ModoDev})
	if err == nil {
		t.Fatal("esperado erro no modo dev sem o gokit ao lado")
	}
	if !strings.Contains(err.Error(), "prod") {
		t.Errorf("a mensagem deveria sugerir a saída (modo prod): %v", err)
	}
}

// Prod é o default seguro: sem o campo, o projeto não depende de pasta local.
func TestModoVazioCaiEmProd(t *testing.T) {
	project := projetoComGoKitIrmao(t)
	result, err := Ensure(Options{Root: project, GoKitLocal: "../gokit"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != ModoProd {
		t.Errorf("modo vazio deveria virar prod, veio %q", result.Mode)
	}
	if _, err := os.Stat(filepath.Join(project, "go.work")); !os.IsNotExist(err) {
		t.Error("modo vazio não deveria criar go.work")
	}
}

// O replace do formato antigo tem de sair na primeira passada, senão o projeto
// migrado continua compilando contra a pasta local sem dizer.
func TestReplaceLegadoEhRemovido(t *testing.T) {
	project := projetoComGoKitIrmao(t)
	antigo := "module minha-aplicacao\n\ngo 1.26\n\nrequire " + CanonicalGoKitModule +
		" v0.0.0\n\nreplace " + CanonicalGoKitModule + " => ../gokit\n"
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte(antigo), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Ensure(Options{Root: project, GoKitLocal: "../gokit", Mode: ModoProd}); err != nil {
		t.Fatal(err)
	}
	goMod := leia(t, filepath.Join(project, "go.mod"))
	if strings.Contains(goMod, "replace") {
		t.Errorf("o replace legado deveria ter saído:\n%s", goMod)
	}
	if strings.Contains(goMod, "v0.0.0") {
		t.Errorf("a versão falsa v0.0.0 deveria ter sido substituída:\n%s", goMod)
	}
}

func TestModoInferidoSegueAPastaIrma(t *testing.T) {
	project := projetoComGoKitIrmao(t)
	if modo := ModoInferido(project, CanonicalGoKitModule); modo != ModoDev {
		t.Errorf("com o gokit ao lado o palpite deveria ser dev, veio %q", modo)
	}
	if modo := ModoInferido(t.TempDir(), CanonicalGoKitModule); modo != ModoProd {
		t.Errorf("sem o gokit ao lado o palpite deveria ser prod, veio %q", modo)
	}
}

// O layout de simulação que o gokit precisa suportar:
//
//	PROJETO_DE_TESTE/
//	  gokit/
//	  projetoteste1/
//	  projetoteste2/
//
// Cada projeto resolve o gokit pela pasta irmã e decide o seu modo sozinho — um
// em dev não arrasta o outro.
func TestProjetosIrmaosSaoIndependentes(t *testing.T) {
	root := t.TempDir()
	framework := filepath.Join(root, "gokit")
	if err := os.MkdirAll(framework, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(framework, "go.mod"),
		[]byte("module "+CanonicalGoKitModule+"\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	um := filepath.Join(root, "projetoteste1")
	dois := filepath.Join(root, "projetoteste2")
	for _, projeto := range []string{um, dois} {
		if err := os.MkdirAll(projeto, 0o755); err != nil {
			t.Fatal(err)
		}
		if modo := ModoInferido(projeto, CanonicalGoKitModule); modo != ModoDev {
			t.Fatalf("%s deveria inferir dev pela pasta irmã, veio %q", filepath.Base(projeto), modo)
		}
		if _, err := Ensure(Options{Root: projeto, GoKitLocal: "auto", Mode: ModoDev}); err != nil {
			t.Fatalf("%s em dev: %v", filepath.Base(projeto), err)
		}
	}

	// Trocar um para prod não pode tocar no outro.
	if _, err := Ensure(Options{Root: dois, GoKitLocal: "auto", Mode: ModoProd}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dois, "go.work")); !os.IsNotExist(err) {
		t.Error("projetoteste2 deveria ter perdido o go.work ao virar prod")
	}
	goWorkUm := leia(t, filepath.Join(um, "go.work"))
	if !strings.Contains(goWorkUm, "../gokit") {
		t.Errorf("projetoteste1 deveria seguir em dev:\n%s", goWorkUm)
	}
}

// Um go.work acima dos projetos vale para quem não tem o seu próprio. Isso faz
// um projeto declarado como prod compilar contra a pasta local sem avisar, e é
// justamente o que ConferirModo existe para denunciar.
func TestConferirModoDenunciaGoWorkAcimaDoProjeto(t *testing.T) {
	root := t.TempDir()
	framework := filepath.Join(root, "gokit")
	projeto := filepath.Join(root, "projetoteste2")
	if err := os.MkdirAll(framework, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projeto, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(framework, "go.mod"),
		[]byte("module "+CanonicalGoKitModule+"\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projeto, "go.mod"),
		[]byte("module projetoteste2\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOWORK", "")
	if aviso := ConferirModo(projeto, ModoProd); aviso != "" {
		t.Fatalf("sem go.work não há conflito, veio %q", aviso)
	}

	// Por use: o gokit entra como módulo do workspace do pai.
	raizWork := filepath.Join(root, "go.work")
	if err := os.WriteFile(raizWork, []byte("go 1.26\n\nuse (\n\t./gokit\n\t./projetoteste2\n)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if aviso := ConferirModo(projeto, ModoProd); aviso != raizWork {
		t.Errorf("esperado o caminho %q, veio %q", raizWork, aviso)
	}

	// Por replace: mesma consequência, forma diferente.
	if err := os.WriteFile(raizWork, []byte("go 1.26\n\nuse ./projetoteste2\n\nreplace "+
		CanonicalGoKitModule+" "+VersaoFixada+" => ./gokit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if aviso := ConferirModo(projeto, ModoProd); aviso != raizWork {
		t.Errorf("replace no go.work do pai deveria acusar, veio %q", aviso)
	}

	// Em dev o desvio é o esperado, então não há o que avisar.
	if aviso := ConferirModo(projeto, ModoDev); aviso != "" {
		t.Errorf("dev não deveria gerar aviso, veio %q", aviso)
	}

	// GOWORK=off desliga workspace no Go; o aviso tem de seguir a mesma regra.
	t.Setenv("GOWORK", "off")
	if aviso := ConferirModo(projeto, ModoProd); aviso != "" {
		t.Errorf("com GOWORK=off não há workspace em escopo, veio %q", aviso)
	}
}
