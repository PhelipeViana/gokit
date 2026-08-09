package gomodule

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/mod/modfile"
)

const CanonicalGoKitModule = "github.com/PhelipeViana/gokit"

type Options struct {
	Root         string
	Module       string
	GoKitModule  string
	GoKitVersion string
	GoKitLocal   string
}

type Result struct {
	Module       string
	GoKitModule  string
	GoKitVersion string
	GoKitLocal   string
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
		return Result{}, fmt.Errorf("go.mod invalido: %w", err)
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
	if file.Go == nil {
		version := strings.TrimPrefix(runtime.Version(), "go")
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			version = parts[0] + "." + parts[1]
		}
		if err := file.AddGoStmt(version); err != nil {
			return Result{}, err
		}
	}

	local := strings.TrimSpace(options.GoKitLocal)
	if local == "auto" {
		local = DetectSiblingGoKit(absRoot, goKitModule)
	}
	version := strings.TrimSpace(options.GoKitVersion)
	if local != "" {
		if version == "" {
			version = "v0.0.0"
		}
		localPath := local
		if !filepath.IsAbs(localPath) {
			localPath = filepath.Join(absRoot, filepath.FromSlash(localPath))
		}
		if _, err := os.Stat(filepath.Join(localPath, "go.mod")); err != nil {
			return Result{}, fmt.Errorf("codigo local do GoKit nao encontrado em %s", local)
		}
		if err := file.AddRequire(goKitModule, version); err != nil {
			return Result{}, err
		}
		if err := file.AddReplace(goKitModule, "", filepath.ToSlash(local), ""); err != nil {
			return Result{}, err
		}
	} else if version != "" {
		if err := file.AddRequire(goKitModule, version); err != nil {
			return Result{}, err
		}
	}

	formatted, err := file.Format()
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(goModPath, formatted, 0o644); err != nil {
		return Result{}, err
	}
	return Result{Module: moduleName, GoKitModule: goKitModule, GoKitVersion: version, GoKitLocal: local}, nil
}
