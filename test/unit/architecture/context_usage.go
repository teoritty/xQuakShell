package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// ContextIssue is a context.Context used in a way that misleads the next reader.
type ContextIssue struct {
	File string
	Line int
	Rule string
}

func (i ContextIssue) String() string {
	return fmt.Sprintf("%s:%d: %s", i.File, i.Line, i.Rule)
}

// contextRootExempt are the files allowed to mint a bare context.Background()
// with no justification: the composition root has no caller to inherit from,
// because it is where the process itself begins.
func contextRootExempt(rel string) bool {
	base := filepath.Base(rel)
	return filepath.Dir(rel) == "." &&
		(base == "main.go" || strings.HasPrefix(base, "main_"))
}

// contextHelperExempt is the single place the presentation layer is allowed to
// name a root context. Everything else there goes through AppAPI.reqCtx, so
// that handlers inherit the app lifecycle instead of each inventing one.
const contextHelperExempt = "internal/presentation/wails/request_context.go"

// sqlNonContextMethods are the database/sql calls that silently drop
// cancellation. Each has an identical sibling taking a context, so choosing
// these is choosing to ignore the caller - never what the code meant.
var sqlNonContextMethods = map[string]bool{
	"Exec": true, "Query": true, "QueryRow": true, "Prepare": true, "Begin": true,
}

// contextDerivers take a parent context, so a Background() inside one is
// establishing a new lifecycle root rather than discarding an existing one.
var contextDerivers = map[string]bool{
	"WithCancel": true, "WithTimeout": true, "WithDeadline": true,
	"WithValue": true, "WithoutCancel": true,
}

// justificationReach is how far above a call a comment may sit and still count
// as explaining it. Three lines covers a short rationale block written directly
// above the call without letting an unrelated comment further up excuse it.
const justificationReach = 3

// ctxFile is one parsed file plus the comment positions the rules need.
// Comments are parsed here and dropped by the other checks, which is why this
// does its own pass rather than reusing walkGoFiles.
type ctxFile struct {
	rel        string
	fset       *token.FileSet
	file       *ast.File
	commentEnd map[int]bool
	usesSQL    bool
}

// CheckContextUsage reports contexts that lie about what the code does with
// them: parameters named as if they were honoured but never read, database
// calls that throw cancellation away, and root contexts minted with no reason
// given.
func CheckContextUsage(repoRoot string) ([]ContextIssue, error) {
	files, err := GoBudgetFiles(repoRoot)
	if err != nil {
		return nil, err
	}

	var issues []ContextIssue
	for _, rel := range files {
		parsed, err := parseCtxFile(repoRoot, rel)
		if err != nil {
			return nil, err
		}
		issues = append(issues, checkUnusedContextParams(parsed)...)
		issues = append(issues, checkContextNamedFuncs(parsed)...)
		issues = append(issues, checkSQLContextVariants(parsed)...)
		issues = append(issues, checkRootContexts(parsed)...)
	}
	return issues, nil
}

func parseCtxFile(repoRoot, rel string) (ctxFile, error) {
	fset := token.NewFileSet()
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return ctxFile{}, fmt.Errorf("parse %s: %w", rel, err)
	}

	commentEnd := make(map[int]bool, len(file.Comments))
	for _, group := range file.Comments {
		for _, c := range group.List {
			commentEnd[fset.Position(c.End()).Line] = true
		}
	}

	usesSQL := false
	for _, imp := range file.Imports {
		if imp.Path != nil && imp.Path.Value == `"database/sql"` {
			usesSQL = true
		}
	}
	return ctxFile{rel: rel, fset: fset, file: file, commentEnd: commentEnd, usesSQL: usesSQL}, nil
}

func (f ctxFile) issue(pos token.Pos, rule string) ContextIssue {
	return ContextIssue{File: f.rel, Line: f.fset.Position(pos).Line, Rule: rule}
}

// isContextType reports whether an expression is written as context.Context.
func isContextType(expr ast.Expr) bool {
	return selectorPath(expr) == "context.Context"
}

// checkUnusedContextParams is the rule the Go compiler cannot enforce and
// staticcheck does not: an unused parameter compiles fine, so a ctx that is
// declared and never read looks exactly like one that is honoured. Naming it _
// costs one character and tells the truth.
func checkUnusedContextParams(f ctxFile) []ContextIssue {
	var issues []ContextIssue
	forEachFunc(f.file, func(_ string, params *ast.FieldList, body *ast.BlockStmt) {
		for _, field := range params.List {
			if !isContextType(field.Type) {
				continue
			}
			for _, name := range field.Names {
				if name.Name == "_" || usesIdent(body, name) {
					continue
				}
				issues = append(issues, f.issue(name.Pos(), fmt.Sprintf(
					"parameter %s is never used; name it _ to say so, or use it", name.Name)))
			}
		}
	})
	return issues
}

// checkContextNamedFuncs catches the sharpest version of the same lie: a
// function called DialContext or WithContext that discards the context it is
// named after. The name is a promise the signature then breaks.
func checkContextNamedFuncs(f ctxFile) []ContextIssue {
	var issues []ContextIssue
	forEachFunc(f.file, func(name string, params *ast.FieldList, _ *ast.BlockStmt) {
		if name == "" || !strings.HasSuffix(name, "Context") {
			return
		}
		for _, field := range params.List {
			if !isContextType(field.Type) {
				continue
			}
			for _, ident := range field.Names {
				if ident.Name != "_" {
					continue
				}
				issues = append(issues, f.issue(ident.Pos(), fmt.Sprintf(
					"%s is named for a context it discards; honour it or drop Context from the name", name)))
			}
		}
	})
	return issues
}

// checkSQLContextVariants flags database/sql calls with a context-taking
// sibling. Unlike an SSH dial or an SFTP write, cancellation here is available
// for the asking, so dropping it is never a constraint - only an oversight.
func checkSQLContextVariants(f ctxFile) []ContextIssue {
	if !f.usesSQL {
		return nil
	}
	var issues []ContextIssue
	ast.Inspect(f.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !sqlNonContextMethods[sel.Sel.Name] {
			return true
		}
		issues = append(issues, f.issue(sel.Sel.Pos(), fmt.Sprintf(
			"database/sql call %s drops cancellation; use %sContext and pass the caller's context",
			sel.Sel.Name, sel.Sel.Name)))
		return true
	})
	return issues
}

// checkRootContexts governs where a context may be invented rather than
// inherited. context.TODO is a placeholder and never belongs in a merged tree.
// context.Background is legitimate in three shapes and suspicious in every
// other: at the composition root, as the parent of a derived context, and where
// a comment states why the work must outlive its caller.
//
// A comment can of course be beside the point - no gate can read English. What
// it cannot be is absent, and absent is the case this catches: a bare
// Background() that nobody ever decided on.
func checkRootContexts(f ctxFile) []ContextIssue {
	derived := derivedContextArgs(f.file)

	var issues []ContextIssue
	ast.Inspect(f.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch selectorPath(call.Fun) {
		case "context.TODO":
			issues = append(issues, f.issue(call.Pos(),
				"context.TODO is a placeholder; use the caller's context or context.Background with a reason"))
		case "context.Background":
			if issue, bad := f.checkBackground(call, derived); bad {
				issues = append(issues, issue)
			}
		}
		return true
	})
	return issues
}

func (f ctxFile) checkBackground(call *ast.CallExpr, derived map[token.Pos]bool) (ContextIssue, bool) {
	if derived[call.Pos()] || contextRootExempt(f.rel) || f.rel == contextHelperExempt {
		return ContextIssue{}, false
	}
	if strings.HasPrefix(f.rel, "internal/presentation/") {
		return f.issue(call.Pos(),
			"presentation must not mint a root context; handlers inherit the app lifecycle via reqCtx"), true
	}
	if f.hasJustification(call.Pos()) {
		return ContextIssue{}, false
	}
	return f.issue(call.Pos(),
		"context.Background here detaches the work from its caller; say why in a comment above it"), true
}

// hasJustification reports whether a comment sits on the call's line or just
// above it.
func (f ctxFile) hasJustification(pos token.Pos) bool {
	line := f.fset.Position(pos).Line
	for candidate := line; candidate >= line-justificationReach; candidate-- {
		if f.commentEnd[candidate] {
			return true
		}
	}
	return false
}

// derivedContextArgs collects the positions of context.Background calls passed
// straight into context.WithCancel and friends, which is a new lifecycle root
// rather than a discarded one.
func derivedContextArgs(file *ast.File) map[token.Pos]bool {
	derived := map[token.Pos]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		path := selectorPath(call.Fun)
		if !strings.HasPrefix(path, "context.") || !contextDerivers[strings.TrimPrefix(path, "context.")] {
			return true
		}
		if arg, isCall := call.Args[0].(*ast.CallExpr); isCall {
			derived[arg.Pos()] = true
		}
		return true
	})
	return derived
}

// forEachFunc visits every declaration and literal with a body, so a closure is
// held to the same rules as a method.
func forEachFunc(file *ast.File, visit func(name string, params *ast.FieldList, body *ast.BlockStmt)) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body != nil && fn.Type.Params != nil {
				visit(fn.Name.Name, fn.Type.Params, fn.Body)
			}
		case *ast.FuncLit:
			if fn.Body != nil && fn.Type.Params != nil {
				visit("", fn.Type.Params, fn.Body)
			}
		}
		return true
	})
}

// usesIdent reports whether a body reads the identifier for anything.
//
// A bare `_ = ctx` does not count. That statement exists only to silence a
// reader's question, and it is the form that hides an unused context best:
// the parameter looks used, so no tool objects, while the code still ignores it.
func usesIdent(body *ast.BlockStmt, decl *ast.Ident) bool {
	used := false
	ast.Inspect(body, func(n ast.Node) bool {
		if isBlankDiscardOf(n, decl.Name) {
			return false
		}
		ident, ok := n.(*ast.Ident)
		if ok && ident.Name == decl.Name && ident.Pos() != decl.Pos() {
			used = true
		}
		return !used
	})
	return used
}

// isBlankDiscardOf matches the statement `_ = name` and nothing else.
func isBlankDiscardOf(n ast.Node, name string) bool {
	assign, ok := n.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	lhs, lhsOK := assign.Lhs[0].(*ast.Ident)
	rhs, rhsOK := assign.Rhs[0].(*ast.Ident)
	return lhsOK && rhsOK && lhs.Name == "_" && rhs.Name == name
}
