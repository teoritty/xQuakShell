package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
)

// FuncBudgetIssue is a function that outgrew one of its budgets.
type FuncBudgetIssue struct {
	Symbol string
	Rule   string
}

func (i FuncBudgetIssue) String() string {
	return fmt.Sprintf("%s: %s", i.Symbol, i.Rule)
}

// FuncShape is what the three function budgets measure.
type FuncShape struct {
	CodeLines int
	Params    int
	Nesting   int
}

// ShapeExceeds reports whether the shape breaks any limit. It is exported so
// scripts/budgets decides what "over budget" means by calling the gate rather
// than by reimplementing it: a regenerator that disagrees with the checker
// writes a baseline that fails the moment it is written.
func ShapeExceeds(s FuncShape, limit FuncLimit) bool {
	return s.exceeds(limit)
}

func (s FuncShape) exceeds(limit FuncLimit) bool {
	return s.CodeLines > limit.MaxCodeLines ||
		s.Params > limit.MaxParams ||
		s.Nesting > limit.MaxNesting
}

// MeasureGoFuncs reports the shape of every production Go function, keyed by
// "path/to/file.go::Name" or "path/to/file.go::Receiver.Name".
func MeasureGoFuncs(repoRoot string) (map[string]FuncShape, error) {
	files, err := GoBudgetFiles(repoRoot)
	if err != nil {
		return nil, err
	}

	out := map[string]FuncShape{}
	for _, rel := range files {
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		codeLines, err := goCodeLineSet(abs)
		if err != nil {
			return nil, err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, abs, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			out[rel+"::"+funcSymbol(fn)] = FuncShape{
				CodeLines: bodyCodeLines(fset, fn, codeLines),
				Params:    countParams(fn),
				Nesting:   maxNesting(fn.Body),
			}
		}
	}
	return out, nil
}

// CheckGoFuncBudgets enforces the per-function limits with the same exemption
// and ratchet mechanics as the file budgets.
func CheckGoFuncBudgets(repoRoot string, cfg BudgetConfig) ([]FuncBudgetIssue, error) {
	measured, err := MeasureGoFuncs(repoRoot)
	if err != nil {
		return nil, err
	}

	limit := cfg.Limits.GoFunc
	exempt := cfg.ExemptFunctions()
	var issues []FuncBudgetIssue

	for _, symbol := range sortedFileKeys(measured) {
		shape := measured[symbol]
		recorded, baselined := cfg.Baseline.Functions[symbol]

		if _, isExempt := exempt[symbol]; isExempt {
			if !shape.exceeds(limit) {
				issues = append(issues, FuncBudgetIssue{
					Symbol: symbol,
					Rule:   fmt.Sprintf("%s is within every limit, so its exemption in %s is stale; delete the entry", shape, BudgetConfigFile),
				})
			}
			continue
		}

		if !baselined {
			if shape.exceeds(limit) {
				issues = append(issues, FuncBudgetIssue{
					Symbol: symbol,
					Rule:   fmt.Sprintf("%s exceeds %s; extract a helper, or - if it is wiring or a flat dispatch switch - add an exemption with a kind and a reason to %s", shape, limit, BudgetConfigFile),
				})
			}
			continue
		}

		issues = append(issues, funcBaselineDrift(symbol, shape, recorded, limit)...)
	}

	issues = append(issues, missingFuncEntries(measured, cfg)...)
	return issues, nil
}

func funcBaselineDrift(symbol string, shape FuncShape, recorded FuncMeasurement, limit FuncLimit) []FuncBudgetIssue {
	was := FuncShape{CodeLines: recorded.CodeLines, Params: recorded.Params, Nesting: recorded.Nesting}

	switch {
	case shape.CodeLines > was.CodeLines || shape.Params > was.Params || shape.Nesting > was.Nesting:
		return []FuncBudgetIssue{{
			Symbol: symbol,
			Rule:   fmt.Sprintf("grew from %s to %s. Baselined functions may shrink, never grow", was, shape),
		}}
	case !shape.exceeds(limit):
		return []FuncBudgetIssue{{
			Symbol: symbol,
			Rule:   fmt.Sprintf("is down to %s and now meets %s; drop it from the %s baseline (`go run ./scripts/budgets -update`)", shape, limit, BudgetConfigFile),
		}}
	case shape != was:
		return []FuncBudgetIssue{{
			Symbol: symbol,
			Rule:   fmt.Sprintf("shrank from %s to %s; re-record it so the ratchet tightens (`go run ./scripts/budgets -update`)", was, shape),
		}}
	}
	return nil
}

func missingFuncEntries(measured map[string]FuncShape, cfg BudgetConfig) []FuncBudgetIssue {
	var issues []FuncBudgetIssue
	for _, symbol := range sortedFileKeys(cfg.Baseline.Functions) {
		if _, ok := measured[symbol]; !ok {
			issues = append(issues, FuncBudgetIssue{
				Symbol: symbol,
				Rule:   fmt.Sprintf("is baselined in %s but no longer exists (deleted or renamed); remove the entry", BudgetConfigFile),
			})
		}
	}
	for _, e := range cfg.Exemptions.Functions {
		if _, ok := measured[e.Symbol]; !ok {
			issues = append(issues, FuncBudgetIssue{
				Symbol: e.Symbol,
				Rule:   fmt.Sprintf("is exempted in %s but no longer exists (deleted or renamed); remove the entry", BudgetConfigFile),
			})
		}
	}
	return issues
}

func (s FuncShape) String() string {
	return fmt.Sprintf("%d code lines / %d params / nesting %d", s.CodeLines, s.Params, s.Nesting)
}

func (l FuncLimit) String() string {
	return fmt.Sprintf("the limits of %d code lines / %d params / nesting %d", l.MaxCodeLines, l.MaxParams, l.MaxNesting)
}

func funcSymbol(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, isPtr := recv.(*ast.StarExpr); isPtr {
		recv = star.X
	}
	// A generic receiver arrives as Type[T]; the type name alone identifies it.
	if idx, isGeneric := recv.(*ast.IndexExpr); isGeneric {
		recv = idx.X
	}
	if ident, isIdent := recv.(*ast.Ident); isIdent {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// bodyCodeLines counts the code lines strictly inside the braces, so a
// signature broken across several lines does not inflate the body it
// introduces.
func bodyCodeLines(fset *token.FileSet, fn *ast.FuncDecl, codeLines map[int]bool) int {
	open := fset.Position(fn.Body.Lbrace).Line
	end := fset.Position(fn.Body.Rbrace).Line
	count := 0
	for line := open + 1; line < end; line++ {
		if codeLines[line] {
			count++
		}
	}
	return count
}

// countParams counts parameter names rather than fields: `a, b int` is two
// things the caller has to get right, which is what the limit is about.
// Results are excluded - a function returning (value, error) is not harder to
// call for it - and so is the receiver.
func countParams(fn *ast.FuncDecl) int {
	if fn.Type.Params == nil {
		return 0
	}
	count := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			count++ // an unnamed parameter is still one argument
			continue
		}
		count += len(field.Names)
	}
	return count
}

type nestingWalker struct{ deepest int }

// maxNesting reports the deepest chain of nested control structures.
//
// Two shapes are deliberately not charged for. A switch counts as one level
// rather than one per case, because the arms are alternatives, not layers -
// charging per case would put every dispatch table over the limit for code
// that reads perfectly flat. An `else if` continues its chain at the depth of
// the `if` it extends, for the same reason: the reader follows a list of
// conditions, not a staircase.
//
// A closure keeps the depth it appears at, so a loop inside a callback inside
// a loop is reported as the three levels it reads as.
func maxNesting(body *ast.BlockStmt) int {
	w := &nestingWalker{}
	w.block(body, 0)
	return w.deepest
}

func (w *nestingWalker) block(b *ast.BlockStmt, depth int) {
	if b == nil {
		return
	}
	for _, stmt := range b.List {
		w.stmt(stmt, depth)
	}
}

func (w *nestingWalker) stmt(s ast.Stmt, depth int) {
	switch n := s.(type) {
	case *ast.IfStmt:
		w.enter(depth)
		w.closures(depth, n.Cond)
		w.block(n.Body, depth+1)
		switch other := n.Else.(type) {
		case *ast.IfStmt:
			w.stmt(other, depth)
		case *ast.BlockStmt:
			w.block(other, depth+1)
		}
	case *ast.ForStmt:
		w.enter(depth)
		w.block(n.Body, depth+1)
	case *ast.RangeStmt:
		w.enter(depth)
		w.closures(depth, n.X)
		w.block(n.Body, depth+1)
	case *ast.SwitchStmt:
		w.enter(depth)
		w.clauses(n.Body, depth+1)
	case *ast.TypeSwitchStmt:
		w.enter(depth)
		w.clauses(n.Body, depth+1)
	case *ast.SelectStmt:
		w.enter(depth)
		w.clauses(n.Body, depth+1)
	case *ast.LabeledStmt:
		w.stmt(n.Stmt, depth)
	case *ast.BlockStmt:
		// A bare block scopes variables; it is not a level of control flow.
		w.block(n, depth)
	default:
		w.closures(depth, s)
	}
}

// clauses walks the bodies of case and comm clauses, which are statement
// lists rather than blocks.
func (w *nestingWalker) clauses(body *ast.BlockStmt, depth int) {
	if body == nil {
		return
	}
	for _, clause := range body.List {
		switch c := clause.(type) {
		case *ast.CaseClause:
			for _, stmt := range c.Body {
				w.stmt(stmt, depth)
			}
		case *ast.CommClause:
			for _, stmt := range c.Body {
				w.stmt(stmt, depth)
			}
		}
	}
}

// closures descends into function literals reachable from a node, which is
// the only way statements that are not control flow can carry more of it.
func (w *nestingWalker) closures(depth int, nodes ...ast.Node) {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		ast.Inspect(node, func(child ast.Node) bool {
			lit, ok := child.(*ast.FuncLit)
			if !ok {
				return true
			}
			w.block(lit.Body, depth)
			return false
		})
	}
}

func (w *nestingWalker) enter(depth int) {
	if depth+1 > w.deepest {
		w.deepest = depth + 1
	}
}
