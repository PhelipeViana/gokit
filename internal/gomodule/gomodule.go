package gomodule

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/PhelipeViana/gokit/internal/i18n"
	"golang.org/x/mod/modfile"
)

const CanonicalGoKitModule = "github.com/PhelipeViana/gokit"

type Options struct {
	Root         string
	Module       string
	GoKitModule  string
	GoKitVersion string
	GoKitLocal   string
	// Mode é "dev", "prod" ou "map". Vazio é normalizado para produção.
	Mode string
}

type Result struct {
	Module       string
	GoKitModule  string
	GoKitVersion string
	GoKitLocal   string
	Mode         string
}

var invalidModuleChar = regexp.MustCompile(`[^A-Za-z0-9._~\-/]+`)

func SuggestedModule(root string) string {
	name := strings.TrimSpace(filepath.Base(filepath.Clean(root)))
	name = strings.Trim(invalidModuleChar.ReplaceAllString(name, "-"), "-./")
	if name == "" {
		return "gokit-app"
	}
	return strings.ToLower(name)
}

func DetectSiblingGoKit(root, module string) string {
	candidate := filepath.Clean(filepath.Join(root, "..", "gokit"))
	data, err := os.ReadFile(filepath.Join(candidate, "go.mod"))
	if err != nil {
		return ""
	}
	parsed, err := modfile.Parse("go.mod", data, nil)
	if err != nil || parsed.Module == nil || parsed.Module.Mod.Path != module {
		return ""
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return candidate
	}
	return filepath.ToSlash(relative)
}

func Ensure(options Options) (Result, error) {
	root := options.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}

	goKitModule := strings.TrimSpace(options.GoKitModule)
	if goKitModule == "" {
		goKitModule = CanonicalGoKitModule
	}
	goModPath := filepath.Join(absRoot, "go.mod")
	data, readErr := os.ReadFile(goModPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return Result{}, readErr
	}

	file, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return Result{}, i18n.Errf("mod_invalid", err)
	}
	moduleName := strings.TrimSpace(options.Module)
	if file.Module != nil && strings.TrimSpace(file.Module.Mod.Path) != "" {
		moduleName = file.Module.Mod.Path
	}
	if moduleName == "" {
		moduleName = SuggestedModule(absRoot)
	}
	if file.Module == nil {
		if err := file.AddModuleStmt(moduleName); err != nil {
			return Result{}, err
		}
	}
	versaoGo := strings.TrimPrefix(runtime.Version(), "go")
	if parts := strings.Split(versaoGo, "."); len(parts) >= 2 {
		versaoGo = parts[0] + "." + parts[1]
	}
	if file.Go == nil {
		if err := file.AddGoStmt(versaoGo); err != nil {
			return Result{}, err
		}
	}

	local := strings.TrimSpace(options.GoKitLocal)
	if local == "auto" || local == "" {
		local = DetectSiblingGoKit(absRoot, goKitModule)
	}

	// O go.mod é igual nos dois modos: require de uma versão publicada, sem
	// replace. É o que torna o arquivo commitável — o desvio para a pasta local
	// é responsabilidade do go.work, que fica fora do Git.
	version := versaoDesejada(options.GoKitVersion)
	if err := file.AddRequire(goKitModule, version); err != nil {
		return Result{}, err
	}
	// Projeto vindo do formato antigo carrega um replace para ../gokit. Ele sai
	// aqui: deixá-lo faria o modo prod continuar compilando contra a pasta local
	// sem ninguém perceber.
	if err := file.DropReplace(goKitModule, ""); err != nil {
		return Result{}, err
	}

	mode := NormalizarModo(options.Mode)
	if mode == ModoDev {
		// Uma mensagem só para os dois jeitos de faltar o gokit local — não achamos
		// a pasta irmã, ou o caminho configurado não tem go.mod. Para quem lê o
		// erro é o mesmo problema, e a saída é a mesma.
		exibicao := local
		if exibicao == "" {
			exibicao = "../gokit"
		}
		localPath := local
		if localPath != "" && !filepath.IsAbs(localPath) {
			localPath = filepath.Join(absRoot, filepath.FromSlash(localPath))
		}
		if localPath == "" {
			return Result{}, i18n.Errf("mod_dev_sem_local", exibicao)
		}
		if _, err := os.Stat(filepath.Join(localPath, "go.mod")); err != nil {
			return Result{}, i18n.Errf("mod_dev_sem_local", exibicao)
		}
		if err := escreverGoWork(absRoot, versaoGo, version, local); err != nil {
			return Result{}, i18n.Errf("mod_gowork_falhou", err)
		}
	} else {
		if err := removerGoWorkDoGoKit(absRoot, local); err != nil {
			return Result{}, i18n.Errf("mod_gowork_falhou", err)
		}
	}

	formatted, err := file.Format()
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(goModPath, formatted, 0o644); err != nil {
		return Result{}, err
	}
	return Result{
		Module: moduleName, GoKitModule: goKitModule,
		GoKitVersion: version, GoKitLocal: local, Mode: mode,
	}, nil
}
