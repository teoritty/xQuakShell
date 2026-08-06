package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
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

// countGoCodeLines counts the lines of a Go file that carry at least one
// non-comment token.
//
// Comments are excluded on purpose. A size budget that counts them taxes the
// one thing this codebase wants more of: rationale written next to the code it
// explains. An author facing a budget measured in raw lines deletes the
// explanation, not the complexity. Measuring only executable weight means a
// twenty-line "why" block above a five-line function is free, while the
// five lines still count.
//
// A line carrying both code and a trailing comment counts as code, and a
// multi-line raw string literal counts every line it spans - the scanner
// reports token boundaries, so both cases fall out of marking the span of each
// non-comment token.
func countGoCodeLines(path string) (int, error) {
	lines, err := goCodeLineSet(path)
	if err != nil {
		return 0, err
	}
	return len(lines), nil
}

// goCodeLineSet reports which 1-based lines of a Go file carry code, so a
// caller measuring a single function can intersect it with that function's
// span instead of re-scanning the file per declaration.
func goCodeLineSet(path string) (map[int]bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	file := fset.AddFile(path, fset.Base(), len(src))

	var s scanner.Scanner
	// The no-op error handler keeps a syntactically broken file from aborting
	// the whole gate run: the layer-import check parses every file anyway and
	// reports the parse error with a usable message, so failing twice here
	// would only bury it.
	s.Init(file, src, func(token.Position, string) {}, scanner.ScanComments)

	codeLines := make(map[int]bool)
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT {
			continue
		}
		start := fset.Position(pos).Line
		codeLines[start] = true
		// Only a raw string literal can carry a newline inside a single
		// non-comment token. Expanding on any token's literal would double
		// count every line end, because the scanner reports its automatic
		// semicolons with "\n" as their literal.
		if tok == token.STRING {
			for i := 1; i <= strings.Count(lit, "\n"); i++ {
				codeLines[start+i] = true
			}
		}
	}
	return codeLines, nil
}
