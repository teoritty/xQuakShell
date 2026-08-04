package architecture

import (
	"fmt"
	"go/ast"
	"strings"
)

// GoroutineIssue is a raw `go` statement in production code.
type GoroutineIssue struct {
	File string
	Line int
	Rule string
}

func (i GoroutineIssue) String() string {
	return fmt.Sprintf("%s:%d: %s", i.File, i.Line, i.Rule)
}

// goroutineScanRoots are the production trees and entrypoints that must route
// every goroutine through safego.
var goroutineScanRoots = []string{
	"internal",
	"app.go",
	"main.go",
	"main_plugins.go",
	"main_connectors.go",
}

// goroutineExemptPrefixes is where the safego wrapper itself lives; it has to
// launch the goroutine it supervises.
var goroutineExemptPrefixes = []string{
	"internal/pkg/safego/",
}

// CheckGoroutineLaunches reports production code that starts goroutines
// directly instead of using safego.GoNamed.
func CheckGoroutineLaunches(repoRoot string) ([]GoroutineIssue, error) {
	var issues []GoroutineIssue

	for _, root := range goroutineScanRoots {
		err := walkGoFiles(repoRoot, root, func(parsed parsedFile) error {
			if isGoroutineExempt(parsed.RelPath) {
				return nil
			}
			ast.Inspect(parsed.File, func(n ast.Node) bool {
				goStmt, ok := n.(*ast.GoStmt)
				if !ok {
					return true
				}
				pos := parsed.Fset.Position(goStmt.Pos())
				issues = append(issues, GoroutineIssue{
					File: parsed.RelPath,
					Line: pos.Line,
					Rule: "production code must launch goroutines via safego.GoNamed",
				})
				return true
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return issues, nil
}

func isGoroutineExempt(relPath string) bool {
	for _, prefix := range goroutineExemptPrefixes {
		if strings.HasPrefix(relPath, prefix) {
			return true
		}
	}
	return false
}
