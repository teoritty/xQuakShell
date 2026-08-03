package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// CheckOptions configures a layer import scan.
type CheckOptions struct {
	RepoRoot string
	Module   string
}

// ImportIssue is a single layer import violation.
type ImportIssue struct {
	File       string
	Line       int
	Layer      Layer
	ImportPath string
	Rule       string
}

func (i ImportIssue) String() string {
	return fmt.Sprintf("%s:%d: layer %s: import %q forbidden (%s)", i.File, i.Line, i.Layer, i.ImportPath, i.Rule)
}

// ReadModulePath reads the module path from go.mod at repoRoot.
func ReadModulePath(repoRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module directive not found in go.mod")
}

// CheckLayerImports scans production and test Go files under internal/ and composition root.
func CheckLayerImports(opts CheckOptions) ([]ImportIssue, error) {
	if opts.Module == "" {
		module, err := ReadModulePath(opts.RepoRoot)
		if err != nil {
			return nil, err
		}
		opts.Module = module
	}

	var issues []ImportIssue

	walkRoots := []string{
		filepath.Join(opts.RepoRoot, "internal"),
	}
	for _, name := range []string{"app.go", "main.go"} {
		path := filepath.Join(opts.RepoRoot, name)
		if _, err := os.Stat(path); err == nil {
			issueList, err := checkFile(opts, path)
			if err != nil {
				return nil, err
			}
			issues = append(issues, issueList...)
		}
	}
	mainGlob, err := filepath.Glob(filepath.Join(opts.RepoRoot, "main_*.go"))
	if err != nil {
		return nil, err
	}
	for _, path := range mainGlob {
		issueList, err := checkFile(opts, path)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issueList...)
	}

	for _, root := range walkRoots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			fileIssues, err := checkFile(opts, path)
			if err != nil {
				return err
			}
			issues = append(issues, fileIssues...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return issues, nil
}

func checkFile(opts CheckOptions, absPath string) ([]ImportIssue, error) {
	relPath, err := filepath.Rel(opts.RepoRoot, absPath)
	if err != nil {
		return nil, err
	}
	relPath = filepathToSlash(relPath)

	layer, ok := LayerFromRelPath(relPath)
	if !ok {
		return nil, nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relPath, err)
	}

	productionFile := !strings.HasSuffix(relPath, "_test.go")
	var issues []ImportIssue
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		v := checkImport(opts.Module, importPath, layer, productionFile)
		if v == nil {
			continue
		}
		pos := fset.Position(imp.Path.Pos())
		issues = append(issues, ImportIssue{
			File:       relPath,
			Line:       pos.Line,
			Layer:      layer,
			ImportPath: importPath,
			Rule:       v.Rule,
		})
	}
	return issues, nil
}

// ParseImportsFromSource extracts import paths from Go source (for meta-tests).
func ParseImportsFromSource(src string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(file.Imports))
	for _, imp := range file.Imports {
		paths = append(paths, strings.Trim(imp.Path.Value, `"`))
	}
	return paths, nil
}

// CheckImportForLayer is exported for table-driven meta-tests.
func CheckImportForLayer(module, importPath string, layer Layer, productionFile bool) *Violation {
	return checkImport(module, importPath, layer, productionFile)
}
