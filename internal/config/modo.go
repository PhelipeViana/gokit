package config

// Leitura, aplicação e troca do modo do projeto (dev | prod).
//
// O modo é decidido na criação e fica gravado em go.mode no gokit.json. Aqui só
// se lê esse campo e se pede ao gomodule que deixe o projeto coerente com ele —
// a política de go.mod e go.work mora lá, não aqui.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/PhelipeViana/gokit/internal/gomodule"
	"github.com/PhelipeViana/gokit/internal/i18n"
)

// ModoDoProjeto devolve o modo declarado. Projeto que ainda não tem o campo
// recebe um palpite pela presença do gokit ao lado — é o caso dos projetos
// criados antes de o modo existir.
func ModoDoProjeto(root string, cfg *Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Go.Mode) != "" {
		return gomodule.NormalizarModo(cfg.Go.Mode)
	}
	modulo := gomodule.CanonicalGoKitModule
	if cfg != nil && strings.TrimSpace(cfg.Go.GoKitModule) != "" {
		modulo = cfg.Go.GoKitModule
	}
	return gomodule.ModoInferido(root, modulo)
}

// AplicarModo põe go.mod e go.work de acordo com o modo declarado. É idempotente
// e é o que o reload chama: rodar duas vezes não muda nada na segunda.
func AplicarModo(root string, cfg *Config) (gomodule.Result, error) {
	goConfig := GoConfig{}
	if cfg != nil {
		goConfig = cfg.Go
	}
	resultado, err := gomodule.Ensure(gomodule.Options{
		Root:         root,
		Module:       goConfig.Module,
		GoKitModule:  goConfig.GoKitModule,
		GoKitVersion: goConfig.GoKitVersion,
		GoKitLocal:   goConfig.GoKitLocal,
		Mode:         ModoDoProjeto(root, cfg),
	})
	if err != nil {
		return resultado, err
	}
	// Projeto criado antes do modo existir não tem go.work no .gitignore, e o
	// modo dev acabou de criar esse arquivo. Commitá-lo levaria para o
	// repositório um caminho que só existe nesta máquina — exatamente o que a
	// mudança para go.work resolve.
	_ = ignorarGoWork(root)
	return resultado, nil
}

// ignorarGoWork garante as duas linhas no .gitignore, sem duplicar e sem mexer
// no que já está lá.
func ignorarGoWork(root string) error {
	caminho := filepath.Join(root, ".gitignore")
	atual, err := os.ReadFile(caminho)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	texto := string(atual)

	var faltando []string
	for _, linha := range []string{"go.work", "go.work.sum"} {
		if !contemLinha(texto, linha) {
			faltando = append(faltando, linha)
		}
	}
	if len(faltando) == 0 {
		return nil
	}
	if texto != "" && !strings.HasSuffix(texto, "\n") {
		texto += "\n"
	}
	texto += strings.Join(faltando, "\n") + "\n"
	return os.WriteFile(caminho, []byte(texto), 0o644)
}

// contemLinha compara linha inteira: procurar a substring "go.work" acharia
// também "go.work.sum" e a segunda entrada nunca seria escrita.
func contemLinha(texto, alvo string) bool {
	for _, linha := range strings.Split(texto, "\n") {
		if strings.TrimSpace(linha) == alvo {
			return true
		}
	}
	return false
}

// DefinirModo troca o modo: grava o campo no gokit.json e reescreve go.mod e
// go.work na mesma passada.
//
// A ordem importa. O gokit.json é escrito antes de mexer nos arquivos de módulo
// porque, se a aplicação falhar (dev sem o gokit ao lado, por exemplo), o campo
// já reflete a intenção do usuário e um `gokit reload` conclui a troca depois.
// O contrário deixaria o projeto apontando para um modo que a configuração nega.
func DefinirModo(root, modo string) (gomodule.Result, error) {
	normalizado := gomodule.NormalizarModo(modo)
	if strings.TrimSpace(modo) != "" && !strings.EqualFold(strings.TrimSpace(modo), normalizado) {
		return gomodule.Result{}, i18n.Errf("modo_desconhecido", modo,
			strings.Join([]string{gomodule.ModoDev, gomodule.ModoProd}, ", "))
	}

	caminho := filepath.Join(root, "internal", "gokit", "gokit.json")
	dados, err := os.ReadFile(caminho)
	if err != nil {
		return gomodule.Result{}, i18n.Errf("cfg_read_failed", err)
	}

	// Edição por mapa genérico, não por struct: reserializar o Config inteiro
	// reordenaria as chaves e apagaria qualquer campo que o gokit ainda não
	// conheça. O gokit.json é arquivo do usuário.
	var bruto map[string]json.RawMessage
	if err := json.Unmarshal(dados, &bruto); err != nil {
		return gomodule.Result{}, i18n.Errf("cfg_bad_json", err)
	}
	secaoGo := map[string]json.RawMessage{}
	if atual, tem := bruto["go"]; tem {
		if err := json.Unmarshal(atual, &secaoGo); err != nil {
			return gomodule.Result{}, i18n.Errf("cfg_bad_json", err)
		}
	}
	valor, err := json.Marshal(normalizado)
	if err != nil {
		return gomodule.Result{}, err
	}
	secaoGo["mode"] = valor
	novaSecao, err := json.Marshal(secaoGo)
	if err != nil {
		return gomodule.Result{}, err
	}
	bruto["go"] = novaSecao

	corpo, err := json.MarshalIndent(bruto, "", "  ")
	if err != nil {
		return gomodule.Result{}, err
	}
	if err := os.WriteFile(caminho, append(corpo, '\n'), 0o644); err != nil {
		return gomodule.Result{}, i18n.Errf("edt_write_failed", "gokit.json", err)
	}

	var cfg Config
	if err := json.Unmarshal(dados, &cfg); err != nil {
		return gomodule.Result{}, i18n.Errf("cfg_bad_json", err)
	}
	cfg.Go.Mode = normalizado
	return AplicarModo(root, &cfg)
}
