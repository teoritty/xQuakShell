package architecture

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// Shared source-scanning helpers for the checks that were ported from the
// scripts/check-*.ps1 gates. Everything here works on slash-normalised paths
// relative to the repo root, so the rules read the same on Windows and Linux.

// prunedDirs never contain Go sources we want to gate.
var prunedDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	".git":         true,
	"frontend":     true,
	"build":        true,
}

// parsedFile is one production Go file, ready for AST inspection.
type parsedFile struct {
	// RelPath is slash-separated and relative to the repo root.
	RelPath string
	File    *ast.File
	Fset    *token.FileSet
}

// parseGoFile parses a single file and reports its repo-relative path.
// Comments are dropped (default parser mode) so the rules never fire on
// prose — several files legitimately name forbidden symbols in comments.
func parseGoFile(repoRoot, path string) (parsedFile, error) {
	relPath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return parsedFile{}, err
	}
	relPath = filepathToSlash(relPath)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return parsedFile{}, fmt.Errorf("parse %s: %w", relPath, err)
	}
	return parsedFile{RelPath: relPath, File: file, Fset: fset}, nil
}

// walkGoFiles visits every production Go file under root, skipping tests and
// pruned directories. root may be a directory or a single file, and is
// interpreted relative to repoRoot.
func walkGoFiles(repoRoot, root string, visit func(parsedFile) error) error {
	return walkGoFilesFiltered(repoRoot, root, nil, visit)
}

// walkGoFilesFiltered is walkGoFiles with an extra skip predicate applied to
// the repo-relative path before the file is parsed.
func walkGoFilesFiltered(repoRoot, root string, skip func(relPath string) bool, visit func(parsedFile) error) error {
	absRoot := filepath.Join(repoRoot, filepath.FromSlash(root))

	info, err := os.Stat(absRoot)
	if err != nil {
		return fmt.Errorf("scan root %s: %w", root, err)
	}
	if !info.IsDir() {
		parsed, err := parseGoFile(repoRoot, absRoot)
		if err != nil {
			return err
		}
		return visit(parsed)
	}

	return filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if prunedDirs[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if skip != nil {
			relPath, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			if skip(filepathToSlash(relPath)) {
				return nil
			}
		}
		parsed, err := parseGoFile(repoRoot, path)
		if err != nil {
			return err
		}
		return visit(parsed)
	})
}

// selectorPath renders a dotted expression such as os.Stat or m.mu.Lock.
// It returns "" for anything that is not a chain of identifiers.
func selectorPath(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		prefix := selectorPath(e.X)
		if prefix == "" || e.Sel == nil {
			return ""
		}
		return prefix + "." + e.Sel.Name
	}
	return ""
}

// findMethod locates a method declaration by receiver type and name.
// recvType is written without the pointer star, e.g. "AppAPI".
func findMethod(file *ast.File, recvType, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != name || fn.Recv == nil {
			continue
		}
		if len(fn.Recv.List) != 1 {
			continue
		}
		recv := fn.Recv.List[0].Type
		if star, isPtr := recv.(*ast.StarExpr); isPtr {
			recv = star.X
		}
		if ident, isIdent := recv.(*ast.Ident); isIdent && ident.Name == recvType {
			return fn
		}
	}
	return nil
}

// countNonBlankLines counts lines with at least one non-whitespace character.
//
// This deliberately matches PowerShell's `Measure-Object -Line`, which the
// replaced scripts used: the budgets below were all calibrated against
// non-blank counts, and switching to raw line counts would retroactively
// break files that are within budget today.
func countNonBlankLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}
