package gomodule

// Modo de consumo do gokit por um projeto gerado.
//
// A escolha é do início: um projeto em desenvolvimento aponta para o gokit que
// está ao lado, na pasta irmã, e enxerga cada alteração na hora; um projeto em
// produção fixa uma versão publicada e não depende de nada fora dele.
//
// Antes isso era inferido — se existisse ../gokit, o projeto ganhava um replace
// que ninguém pediu, e esse replace ia para o Git. Quem clonasse o projeto sem
// o gokit ao lado recebia um go.mod que não compilava. Agora o modo é um campo
// declarado no gokit.json, e o mecanismo de cada modo decorre dele.

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// Modos válidos. ModoMap está reservado: o campo aceita o valor para não exigir
// migração de configuração quando o modo de mapeamento chegar, mas hoje ele se
// comporta como produção.
const (
	ModoDev  = "dev"
	ModoProd = "prod"
	ModoMap  = "map"
)

// VersaoFixada é a versão que o modo produção exige no go.mod. É também a
// versão da biblioteca que este executável sabe gerar código para — por isso
// mora aqui, e não no template do scaffold: os dois têm de andar juntos.
const VersaoFixada = "v0.1.0"

// versaoDesejada resolve qual versão o go.mod deve exigir.
//
// v0.0.0 era o número que o formato antigo escrevia para acompanhar o replace:
// não existe publicada, servia só para o require ter alguma coisa. Encontrá-la
// significa projeto vindo do formato antigo, e a versão fixada toma o lugar.
// Qualquer outro valor é escolha do usuário e é respeitado.
func versaoDesejada(configurada string) string {
	limpa := strings.TrimSpace(configurada)
	if limpa == "" || limpa == "v0.0.0" {
		return VersaoFixada
	}
	return limpa
}

// NormalizarModo aceita o que estiver escrito no gokit.json e devolve um modo
// conhecido. Valor vazio ou desconhecido cai em produção, que é o modo que não
// depende de nada fora do projeto — errar para o lado seguro.
func NormalizarModo(valor string) string {
	switch strings.ToLower(strings.TrimSpace(valor)) {
	case ModoDev:
		return ModoDev
	case ModoMap:
		return ModoMap
	default:
		return ModoProd
	}
}

// ModoInferido serve a dois casos: sugerir o default na criação do projeto e
// dar um modo a um projeto que ainda não tem o campo. Só o desenvolvimento do
// próprio gokit tem a pasta irmã, então a presença dela é um bom palpite.
func ModoInferido(root, goKitModule string) string {
	if DetectSiblingGoKit(root, goKitModule) != "" {
		return ModoDev
	}
	return ModoProd
}

// caminhoGoWork é onde o apontamento local vive no modo dev. Fora do go.mod de
// propósito: o go.work não é commitado, então cada máquina resolve o seu.
func caminhoGoWork(root string) string {
	return filepath.Join(root, "go.work")
}

// escreverGoWork cria ou atualiza o go.work do modo dev.
//
// O desvio é um replace com versão, não um `use ../gokit`. As duas formas
// parecem equivalentes e não são:
//
//   - `use` põe o gokit como módulo do workspace, o que resolve o pacote mas
//     não o grafo de módulos — o go.mod do projeto exige uma versão publicada e
//     o Go ainda vai ler o go.mod dessa versão para o MVS. Enquanto a tag não
//     existir no remoto, o build morre com "unknown revision".
//   - `replace <módulo> <versão> => <caminho>` satisfaz o grafo e a resolução ao
//     mesmo tempo, e é compatível com o require que fica no go.mod.
//
// Declarar os dois é erro: o Go recusa com "workspace module is replaced at all
// versions". Por isso o `use` lista só o projeto.
//
// Se o arquivo já existe, o que é do desenvolvedor fica: só a entrada do gokit é
// reescrita.
func escreverGoWork(root, versaoGo, versaoGoKit, gokitLocal string) error {
	caminho := caminhoGoWork(root)

	atual, err := os.ReadFile(caminho)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	arquivo, err := modfile.ParseWork(caminho, atual, nil)
	if err != nil {
		// go.work ilegível é problema do usuário para resolver, não nosso para
		// sobrescrever — pode haver trabalho dele ali.
		return err
	}
	if arquivo.Go == nil {
		if err := arquivo.AddGoStmt(versaoGo); err != nil {
			return err
		}
	}

	usados := map[string]bool{}
	for _, uso := range arquivo.Use {
		usados[filepath.ToSlash(filepath.Clean(uso.Path))] = true
	}
	if !usados["."] {
		if err := arquivo.AddUse(".", ""); err != nil {
			return err
		}
	}
	// Um `use` do gokit vindo de execução anterior tem de sair, senão ele colide
	// com o replace abaixo.
	for _, uso := range arquivo.Use {
		if filepath.ToSlash(filepath.Clean(uso.Path)) == "." {
			continue
		}
		if ehGoKitLocal(root, uso.Path) {
			if err := arquivo.DropUse(uso.Path); err != nil {
				return err
			}
		}
	}
	if err := arquivo.AddReplace(CanonicalGoKitModule, versaoGoKit, filepath.ToSlash(gokitLocal), ""); err != nil {
		return err
	}

	arquivo.Cleanup()
	// WorkFile não tem Format próprio: a formatação canônica sai do syntax tree,
	// que é o mesmo caminho que o comando go usa.
	return os.WriteFile(caminho, modfile.Format(arquivo.Syntax), 0o644)
}

// removerGoWorkDoGoKit desfaz o modo dev. Só remove o arquivo se ele existir
// exclusivamente para o gokit: se o desenvolvedor pôs outros módulos no
// workspace, tiramos apenas a entrada do gokit e deixamos o resto.
//
// Sem isso, "prod" seria mentira: um go.work esquecido continua redirecionando
// o import para a pasta local, e o build passaria a usar código não publicado.
func removerGoWorkDoGoKit(root, gokitLocal string) error {
	caminho := caminhoGoWork(root)
	atual, err := os.ReadFile(caminho)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	arquivo, err := modfile.ParseWork(caminho, atual, nil)
	if err != nil {
		return err
	}
	// Cai fora qualquer replace do gokit, em qualquer versão: é o que faz o
	// projeto voltar a compilar contra o módulo publicado.
	for _, replace := range append([]*modfile.Replace(nil), arquivo.Replace...) {
		if replace.Old.Path == CanonicalGoKitModule {
			if err := arquivo.DropReplace(replace.Old.Path, replace.Old.Version); err != nil {
				return err
			}
		}
	}

	alvo := ""
	if gokitLocal != "" {
		alvo = filepath.ToSlash(filepath.Clean(gokitLocal))
	}
	var restantes []string
	for _, uso := range arquivo.Use {
		limpo := filepath.ToSlash(filepath.Clean(uso.Path))
		if limpo == "." {
			continue
		}
		if alvo != "" && limpo == alvo {
			continue
		}
		if ehGoKitLocal(root, uso.Path) {
			continue
		}
		restantes = append(restantes, uso.Path)
	}
	if len(restantes) == 0 {
		return os.Remove(caminho)
	}

	for _, uso := range arquivo.Use {
		limpo := filepath.ToSlash(filepath.Clean(uso.Path))
		manter := false
		for _, restante := range restantes {
			if filepath.ToSlash(filepath.Clean(restante)) == limpo {
				manter = true
				break
			}
		}
		if !manter {
			if err := arquivo.DropUse(uso.Path); err != nil {
				return err
			}
		}
	}
	arquivo.Cleanup()
	// WorkFile não tem Format próprio: a formatação canônica sai do syntax tree,
	// que é o mesmo caminho que o comando go usa.
	return os.WriteFile(caminho, modfile.Format(arquivo.Syntax), 0o644)
}

// ehGoKitLocal reconhece uma entrada de workspace que aponta para um checkout do
// gokit, mesmo que o caminho não seja o que detectamos. Comparar pelo módulo
// declarado é mais confiável que comparar strings de caminho.
func ehGoKitLocal(root, caminhoUso string) bool {
	destino := caminhoUso
	if !filepath.IsAbs(destino) {
		destino = filepath.Join(root, filepath.FromSlash(destino))
	}
	dados, err := os.ReadFile(filepath.Join(destino, "go.mod"))
	if err != nil {
		return false
	}
	analisado, err := modfile.Parse("go.mod", dados, nil)
	if err != nil || analisado.Module == nil {
		return false
	}
	return analisado.Module.Mod.Path == CanonicalGoKitModule
}
