package architecture

import (
	"fmt"
	"go/ast"
	"strings"
)

// FSBoundaryIssue is a violation of the filesystem trust boundary (ADR-007).
type FSBoundaryIssue struct {
	File string
	Line int
	Rule string
}

func (i FSBoundaryIssue) String() string {
	return fmt.Sprintf("%s:%d: %s", i.File, i.Line, i.Rule)
}

const hostInfraImport = "xquakshell/internal/infra/host"

// removedLocalFSSymbols were deleted with ADR-007 and must not come back.
var removedLocalFSSymbols = map[string]bool{
	"LocalFileSystem":    true,
	"NewLocalFS":         true,
	"ErrLocalPathDenied": true,
}

// usecaseForbiddenOSCalls are the os package filesystem entrypoints that the
// usecase layer must reach through a port instead of calling directly.
var usecaseForbiddenOSCalls = map[string]bool{
	"Stat": true, "Lstat": true, "Remove": true, "RemoveAll": true,
	"ReadFile": true, "WriteFile": true, "Open": true, "OpenFile": true,
	"Create": true, "Mkdir": true, "MkdirAll": true, "Rename": true,
	"ReadDir": true, "Chmod": true, "Chown": true, "Symlink": true,
	"Link": true, "Truncate": true, "ReadLink": true, "TempDir": true,
}

// portableHandlers must stay on the portable data store, hostHandlers on the
// host filesystem. Crossing over is the bug ADR-007 exists to prevent.
var portableOnlyHandlers = []string{"GetPortableDataRoot", "GetTempDir"}
var hostOnlyHandlers = []string{"ListLocalPath", "GetUserHomeDir"}

// CheckFSBoundaries enforces the ADR-007 filesystem trust boundary.
func CheckFSBoundaries(repoRoot string) ([]FSBoundaryIssue, error) {
	var issues []FSBoundaryIssue

	for _, check := range []func(string) ([]FSBoundaryIssue, error){
		checkPluginHostFSUsage,
		checkTransferServicePortableUsage,
		checkLocalFSHandlerSplit,
		checkRemovedLocalFSSymbols,
		checkUsecaseOSCalls,
	} {
		found, err := check(repoRoot)
		if err != nil {
			return nil, err
		}
		issues = append(issues, found...)
	}
	return issues, nil
}

// 1. infra/plugin talks to the sandboxed proxy, never to the host filesystem.
func checkPluginHostFSUsage(repoRoot string) ([]FSBoundaryIssue, error) {
	var issues []FSBoundaryIssue

	err := walkGoFiles(repoRoot, "internal/infra/plugin", func(parsed parsedFile) error {
		for _, imp := range parsed.File.Imports {
			if imp.Path == nil || strings.Trim(imp.Path.Value, `"`) != hostInfraImport {
				continue
			}
			pos := parsed.Fset.Position(imp.Path.Pos())
			issues = append(issues, FSBoundaryIssue{
				File: parsed.RelPath,
				Line: pos.Line,
				Rule: "infra/plugin must not import " + hostInfraImport,
			})
		}

		ast.Inspect(parsed.File, func(n ast.Node) bool {
			selector, ok := n.(*ast.SelectorExpr)
			if !ok || selectorPath(selector) != "domain.HostFileSystem" {
				return true
			}
			pos := parsed.Fset.Position(selector.Pos())
			issues = append(issues, FSBoundaryIssue{
				File: parsed.RelPath,
				Line: pos.Line,
				Rule: "infra/plugin must not use domain.HostFileSystem",
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

// 2. transfer_service moves host files only; the portable store is off limits.
func checkTransferServicePortableUsage(repoRoot string) ([]FSBoundaryIssue, error) {
	var issues []FSBoundaryIssue

	err := walkGoFiles(repoRoot, "internal/usecase/transfer_service.go", func(parsed parsedFile) error {
		forbidden := map[string]bool{"PortableDataStore": true, "portableData": true}
		for _, use := range findIdentUses(parsed, parsed.File, forbidden) {
			issues = append(issues, FSBoundaryIssue{
				File: parsed.RelPath,
				Line: use.Line,
				Rule: fmt.Sprintf("transfer_service must use HostFileSystem only, found %s", use.Name),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// 3. The local-FS handlers must not blur portable and host storage.
func checkLocalFSHandlerSplit(repoRoot string) ([]FSBoundaryIssue, error) {
	var issues []FSBoundaryIssue

	err := walkGoFiles(repoRoot, "internal/presentation/wails/handlers_local_fs.go", func(parsed parsedFile) error {
		inspect := func(names []string, forbidden, rule string) {
			for _, name := range names {
				fn := findMethod(parsed.File, "AppAPI", name)
				if fn == nil || fn.Body == nil {
					issues = append(issues, FSBoundaryIssue{
						File: parsed.RelPath,
						Rule: fmt.Sprintf("expected method (*AppAPI).%s is missing", name),
					})
					continue
				}
				for _, use := range findIdentUses(parsed, fn.Body, map[string]bool{forbidden: true}) {
					issues = append(issues, FSBoundaryIssue{
						File: parsed.RelPath,
						Line: use.Line,
						Rule: fmt.Sprintf("%s %s", name, rule),
					})
				}
			}
		}
		inspect(portableOnlyHandlers, "hostFS", "must not use hostFS")
		inspect(hostOnlyHandlers, "portableData", "must not use portableData")
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// 4. The symbols removed by ADR-007 must stay removed from production code.
func checkRemovedLocalFSSymbols(repoRoot string) ([]FSBoundaryIssue, error) {
	var issues []FSBoundaryIssue

	isTestTree := func(relPath string) bool { return strings.HasPrefix(relPath, "test/") }

	err := walkGoFilesFiltered(repoRoot, ".", isTestTree, func(parsed parsedFile) error {
		for _, use := range findIdentUses(parsed, parsed.File, removedLocalFSSymbols) {
			issues = append(issues, FSBoundaryIssue{
				File: parsed.RelPath,
				Line: use.Line,
				Rule: fmt.Sprintf("removed symbol %s must not remain in production code", use.Name),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// 5. usecase reaches the filesystem through ports, never through os directly.
func checkUsecaseOSCalls(repoRoot string) ([]FSBoundaryIssue, error) {
	var issues []FSBoundaryIssue

	err := walkGoFiles(repoRoot, "internal/usecase", func(parsed parsedFile) error {
		ast.Inspect(parsed.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || pkg.Name != "os" || !usecaseForbiddenOSCalls[selector.Sel.Name] {
				return true
			}
			pos := parsed.Fset.Position(selector.Pos())
			issues = append(issues, FSBoundaryIssue{
				File: parsed.RelPath,
				Line: pos.Line,
				Rule: fmt.Sprintf("internal/usecase must not call os.%s directly", selector.Sel.Name),
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

type identUse struct {
	Name string
	Line int
}

// findIdentUses reports every identifier under root whose name is forbidden.
// Inspecting identifiers covers bare names and selector fields alike, so
// `portableData` and `a.portableData` are both caught.
func findIdentUses(parsed parsedFile, root ast.Node, forbidden map[string]bool) []identUse {
	var uses []identUse
	ast.Inspect(root, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || !forbidden[ident.Name] {
			return true
		}
		uses = append(uses, identUse{
			Name: ident.Name,
			Line: parsed.Fset.Position(ident.Pos()).Line,
		})
		return true
	})
	return uses
}
