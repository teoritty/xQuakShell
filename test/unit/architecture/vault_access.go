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

// VaultAccessIssue is a direct vault repository reference in presentation.
type VaultAccessIssue struct {
	File string
	Line int
	Rule string
}

func (i VaultAccessIssue) String() string {
	return fmt.Sprintf("%s:%d: %s", i.File, i.Line, i.Rule)
}

var vaultRepoFieldNames = map[string]bool{
	"connRepo":     true,
	"passwordRepo": true,
	"identRepo":    true,
}

var vaultRepoTypes = map[string]bool{
	"ConnectionRepository": true,
	"PasswordRepository":   true,
	"IdentityRepository":   true,
}

// CheckPresentationVaultAccess scans presentation production code for direct vault repo usage.
func CheckPresentationVaultAccess(repoRoot string) ([]VaultAccessIssue, error) {
	presentationRoot := filepath.Join(repoRoot, "internal", "presentation")
	var issues []VaultAccessIssue

	err := filepath.WalkDir(presentationRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relPath = filepathToSlash(relPath)

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relPath, err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			structType, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range structType.Fields.List {
				if len(field.Names) == 0 {
					continue
				}
				for _, name := range field.Names {
					if !vaultRepoFieldNames[name.Name] {
						continue
					}
					if vaultRepoFieldType(field.Type) {
						pos := fset.Position(name.Pos())
						issues = append(issues, VaultAccessIssue{
							File: relPath,
							Line: pos.Line,
							Rule: "presentation must not hold vault repository fields; use VaultService",
						})
					}
				}
			}
			return true
		})

		ast.Inspect(file, func(n ast.Node) bool {
			selector, ok := n.(*ast.SelectorExpr)
			if !ok || !vaultRepoFieldNames[selector.Sel.Name] {
				return true
			}
			pos := fset.Position(selector.Pos())
			issues = append(issues, VaultAccessIssue{
				File: relPath,
				Line: pos.Line,
				Rule: "presentation must not reference vault repositories directly; use VaultService",
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

func vaultRepoFieldType(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "domain" {
		return false
	}
	return vaultRepoTypes[selector.Sel.Name]
}
