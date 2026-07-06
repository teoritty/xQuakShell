package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// CompositionIssue describes a composition-root policy violation.
type CompositionIssue struct {
	File string
	Line int
	Rule string
}

func (i CompositionIssue) String() string {
	if i.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", i.File, i.Line, i.Rule)
	}
	return fmt.Sprintf("%s: %s", i.File, i.Rule)
}

var compositionWiringSymbols = []string{
	"NewPluginAuthMethodBuilder",
	"NewPluginAuthBridge",
	"NewPluginAuthAttemptRegistry",
	"NewVaultRepo",
	"NewDialer",
	"NewConnectionRepo",
	"NewIdentityRepo",
	"NewPasswordRepo",
	"NewKnownHostsRepo",
	"NewSQLiteRepo",
	"NewTCPPinger",
}

var facadeForbiddenImportPrefixes = []string{
	"/internal/infra/",
	"/internal/pkg/conlimit",
	"/internal/pkg/ratelimit",
}

// CheckCompositionRoot enforces ADR-010: DI only in main.go + main_*.go; app.go is Wails facade.
func CheckCompositionRoot(opts CheckOptions) ([]CompositionIssue, error) {
	if opts.Module == "" {
		module, err := ReadModulePath(opts.RepoRoot)
		if err != nil {
			return nil, err
		}
		opts.Module = module
	}

	var issues []CompositionIssue

	rootGoFiles, err := filepath.Glob(filepath.Join(opts.RepoRoot, "*.go"))
	if err != nil {
		return nil, err
	}

	for _, absPath := range rootGoFiles {
		relPath, err := filepath.Rel(opts.RepoRoot, absPath)
		if err != nil {
			return nil, err
		}
		relPath = filepathToSlash(relPath)
		if strings.HasSuffix(relPath, "_test.go") {
			continue
		}

		fileIssues, err := checkCompositionRootFile(opts, absPath, relPath)
		if err != nil {
			return nil, err
		}
		issues = append(issues, fileIssues...)
	}

	wiringIssues, err := checkWiringSymbolLocations(opts.RepoRoot)
	if err != nil {
		return nil, err
	}
	issues = append(issues, wiringIssues...)

	return issues, nil
}

func isCompositionRootFile(relPath string) bool {
	if relPath == "main.go" {
		return true
	}
	return strings.HasPrefix(relPath, "main_") && strings.HasSuffix(relPath, ".go")
}

func isFacadeFile(relPath string) bool {
	return relPath == "app.go"
}

func checkCompositionRootFile(opts CheckOptions, absPath, relPath string) ([]CompositionIssue, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relPath, err)
	}
	if file.Name == nil || file.Name.Name != "main" {
		return nil, nil
	}

	var issues []CompositionIssue

	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if !isForbiddenFacadeImport(opts.Module, importPath) {
			continue
		}
		if isFacadeFile(relPath) {
			pos := fset.Position(imp.Path.Pos())
			issues = append(issues, CompositionIssue{
				File: relPath,
				Line: pos.Line,
				Rule: fmt.Sprintf("app.go must not import %q (Wails facade only; wire in main_*.go)", importPath),
			})
			continue
		}
		if !isCompositionRootFile(relPath) {
			pos := fset.Position(imp.Path.Pos())
			issues = append(issues, CompositionIssue{
				File: relPath,
				Line: pos.Line,
				Rule: fmt.Sprintf("only main.go and main_*.go may import %q (composition root)", importPath),
			})
		}
	}

	if isFacadeFile(relPath) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Recv != nil {
				continue
			}
			if strings.HasPrefix(fn.Name.Name, "New") {
				pos := fset.Position(fn.Name.Pos())
				issues = append(issues, CompositionIssue{
					File: relPath,
					Line: pos.Line,
					Rule: fmt.Sprintf("app.go must not define constructor %s (use main_*.go)", fn.Name.Name),
				})
			}
		}
	}

	return issues, nil
}

func isForbiddenFacadeImport(module, importPath string) bool {
	for _, prefix := range facadeForbiddenImportPrefixes {
		if strings.Contains(importPath, prefix) {
			return true
		}
	}
	return false
}

func checkWiringSymbolLocations(repoRoot string) ([]CompositionIssue, error) {
	var issues []CompositionIssue

	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == "node_modules" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relPath = filepathToSlash(relPath)

		if isAllowedWiringLocation(relPath) {
			return nil
		}
		if strings.HasPrefix(relPath, "internal/infra/") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relPath, err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sym := wiringCallSymbol(call.Fun)
			if sym == "" || !isCompositionWiringSymbol(sym) {
				return true
			}
			pos := fset.Position(call.Fun.Pos())
			issues = append(issues, CompositionIssue{
				File: relPath,
				Line: pos.Line,
				Rule: fmt.Sprintf("%s may only be called from main.go or main_*.go", sym),
			})
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issues, nil
}

func isAllowedWiringLocation(relPath string) bool {
	if isCompositionRootFile(relPath) {
		return true
	}
	if strings.HasPrefix(relPath, "test/") {
		return true
	}
	if strings.HasPrefix(relPath, "internal/usecase/") && strings.HasSuffix(relPath, "_test.go") {
		return true
	}
	return false
}

func wiringCallSymbol(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if e.Sel != nil {
			return e.Sel.Name
		}
	}
	return ""
}

func isCompositionWiringSymbol(name string) bool {
	for _, sym := range compositionWiringSymbols {
		if sym == name {
			return true
		}
	}
	return false
}
