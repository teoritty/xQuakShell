// Package architecture enforces Clean Architecture layer import rules.
// Canonical rule definitions live here; see docs/architecture.md for human-readable summary.
package architecture

import (
	"strings"
)

// Layer identifies a Clean Architecture package zone.
type Layer string

const (
	LayerDomain       Layer = "domain"
	LayerUsecase      Layer = "usecase"
	LayerInfra        Layer = "infra"
	LayerPresentation Layer = "presentation"
	LayerMain         Layer = "main"
)

// Violation describes a forbidden import.
type Violation struct {
	ImportPath string
	Rule       string
}

func checkImport(module, importPath string, layer Layer, productionFile bool) *Violation {
	if layer == LayerMain {
		return nil
	}
	if isStdLib(module, importPath) {
		return nil
	}

	internalPath := strings.TrimPrefix(importPath, module+"/")

	switch layer {
	case LayerDomain:
		return checkDomainImport(module, internalPath, importPath)
	case LayerUsecase:
		if !productionFile {
			return nil
		}
		return checkUsecaseImport(module, internalPath, importPath)
	case LayerInfra:
		return checkInfraImport(module, internalPath, importPath)
	case LayerPresentation:
		return checkPresentationImport(module, internalPath, importPath)
	default:
		return nil
	}
}

func checkDomainImport(module, internalPath, importPath string) *Violation {
	if strings.HasPrefix(internalPath, "internal/domain") {
		return nil
	}
	if importPath == "golang.org/x/crypto/ssh" {
		return nil
	}
	if strings.HasPrefix(importPath, moduleInternalPrefix(module, "internal/")) {
		return &Violation{ImportPath: importPath, Rule: "domain must not import other internal packages"}
	}
	if strings.HasPrefix(importPath, "golang.org/") {
		return &Violation{ImportPath: importPath, Rule: "domain may import only golang.org/x/crypto/ssh"}
	}
	if strings.HasPrefix(importPath, "github.com/") {
		return &Violation{ImportPath: importPath, Rule: "domain must not import third-party packages"}
	}
	return &Violation{ImportPath: importPath, Rule: "domain may import only stdlib, internal/domain, and golang.org/x/crypto/ssh"}
}

func checkUsecaseImport(module, internalPath, importPath string) *Violation {
	if strings.HasPrefix(internalPath, "internal/domain") {
		return nil
	}
	if importPath == module+"/internal/pkg/safego" {
		return nil
	}
	if strings.HasPrefix(internalPath, "internal/infra") {
		return &Violation{ImportPath: importPath, Rule: "usecase must not import internal/infra"}
	}
	if strings.HasPrefix(internalPath, "internal/presentation") {
		return &Violation{ImportPath: importPath, Rule: "usecase must not import internal/presentation"}
	}
	if strings.HasPrefix(internalPath, "internal/pkg") {
		return &Violation{ImportPath: importPath, Rule: "usecase may import only internal/pkg/safego"}
	}
	if strings.HasPrefix(importPath, "golang.org/") {
		return &Violation{ImportPath: importPath, Rule: "usecase production code must not import golang.org packages"}
	}
	if strings.HasPrefix(importPath, "github.com/") {
		return &Violation{ImportPath: importPath, Rule: "usecase must not import third-party packages"}
	}
	if strings.HasPrefix(importPath, module+"/") {
		return &Violation{ImportPath: importPath, Rule: "usecase may import only internal/domain and internal/pkg/safego"}
	}
	return &Violation{ImportPath: importPath, Rule: "usecase may import only internal/domain, internal/pkg/safego, and stdlib"}
}

func checkInfraImport(module, internalPath, importPath string) *Violation {
	if strings.HasPrefix(internalPath, "internal/domain") {
		return nil
	}
	if strings.HasPrefix(internalPath, "internal/pkg") {
		return nil
	}
	if strings.HasPrefix(internalPath, "internal/infra") {
		return nil
	}
	if strings.HasPrefix(internalPath, "internal/usecase") {
		return &Violation{ImportPath: importPath, Rule: "infra must not import internal/usecase"}
	}
	if strings.HasPrefix(internalPath, "internal/presentation") {
		return &Violation{ImportPath: importPath, Rule: "infra must not import internal/presentation"}
	}
	if strings.HasPrefix(importPath, module+"/") {
		return &Violation{ImportPath: importPath, Rule: "infra may import only internal/domain, internal/pkg, and internal/infra"}
	}
	return nil
}

func checkPresentationImport(module, internalPath, importPath string) *Violation {
	if strings.HasPrefix(internalPath, "internal/domain") {
		return nil
	}
	if strings.HasPrefix(internalPath, "internal/usecase") {
		return nil
	}
	if strings.HasPrefix(internalPath, "internal/presentation") {
		return nil
	}
	if importPath == module+"/internal/pkg/safego" {
		return nil
	}
	if strings.HasPrefix(internalPath, "internal/infra") {
		return &Violation{ImportPath: importPath, Rule: "presentation must not import internal/infra"}
	}
	if strings.HasPrefix(internalPath, "internal/pkg") {
		return &Violation{ImportPath: importPath, Rule: "presentation may import only internal/pkg/safego"}
	}
	if strings.HasPrefix(importPath, "github.com/wailsapp/wails/v2") {
		return nil
	}
	if strings.HasPrefix(importPath, "golang.org/x/sys/") {
		return nil
	}
	if strings.HasPrefix(importPath, "github.com/") {
		return &Violation{ImportPath: importPath, Rule: "presentation may import only github.com/wailsapp/wails/v2"}
	}
	if strings.HasPrefix(importPath, "golang.org/") {
		return &Violation{ImportPath: importPath, Rule: "presentation may import only golang.org/x/sys"}
	}
	if strings.HasPrefix(importPath, module+"/") {
		return &Violation{ImportPath: importPath, Rule: "presentation may import only internal/domain, internal/usecase, internal/presentation, and internal/pkg/safego"}
	}
	return &Violation{ImportPath: importPath, Rule: "presentation may import only allowed internal packages, Wails, golang.org/x/sys, and stdlib"}
}

func moduleInternalPrefix(module, suffix string) string {
	return module + "/" + suffix
}

func isStdLib(module, importPath string) bool {
	if strings.HasPrefix(importPath, module+"/") {
		return false
	}
	i := strings.Index(importPath, "/")
	if i < 0 {
		i = len(importPath)
	}
	return !strings.Contains(importPath[:i], ".")
}

// LayerFromRelPath maps a repository-relative file path to its architecture layer.
func LayerFromRelPath(relPath string) (Layer, bool) {
	relPath = filepathToSlash(relPath)
	switch {
	case relPath == "app.go" || relPath == "main.go" || (strings.HasPrefix(relPath, "main_") && strings.HasSuffix(relPath, ".go")):
		return LayerMain, true
	case strings.HasPrefix(relPath, "internal/domain/"):
		return LayerDomain, true
	case strings.HasPrefix(relPath, "internal/usecase/"):
		return LayerUsecase, true
	case strings.HasPrefix(relPath, "internal/infra/"):
		return LayerInfra, true
	case strings.HasPrefix(relPath, "internal/presentation/"):
		return LayerPresentation, true
	default:
		return "", false
	}
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
