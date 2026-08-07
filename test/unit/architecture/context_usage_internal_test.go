package architecture

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// parseCtxSource builds the same ctxFile the repo scan builds, from a literal
// so each rule can be driven at its boundary without a fixture file on disk.
func parseCtxSource(t *testing.T, rel, src string) ctxFile {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	commentEnd := map[int]bool{}
	for _, group := range file.Comments {
		for _, c := range group.List {
			commentEnd[fset.Position(c.End()).Line] = true
		}
	}
	return ctxFile{
		rel:        rel,
		fset:       fset,
		file:       file,
		commentEnd: commentEnd,
		usesSQL:    strings.Contains(src, `"database/sql"`),
	}
}

func rules(t *testing.T, rel, src string) []ContextIssue {
	t.Helper()
	f := parseCtxSource(t, rel, src)
	var issues []ContextIssue
	issues = append(issues, checkUnusedContextParams(f)...)
	issues = append(issues, checkContextNamedFuncs(f)...)
	issues = append(issues, checkSQLContextVariants(f)...)
	issues = append(issues, checkRootContexts(f)...)
	return issues
}

func assertClean(t *testing.T, rel, src string) {
	t.Helper()
	if got := rules(t, rel, src); len(got) != 0 {
		t.Errorf("want no issues, got %v", got)
	}
}

func assertFlags(t *testing.T, rel, src, wantSubstring string) {
	t.Helper()
	got := rules(t, rel, src)
	for _, issue := range got {
		if strings.Contains(issue.Rule, wantSubstring) {
			return
		}
	}
	t.Errorf("want an issue mentioning %q, got %v", wantSubstring, got)
}

const pkgHeader = "package p\n\nimport \"context\"\n\n"

func TestUnusedContextParamMustBeBlank(t *testing.T) {
	assertFlags(t, "internal/a/a.go", pkgHeader+`
func f(ctx context.Context) int { return 1 }
`, "never used")

	assertClean(t, "internal/a/a.go", pkgHeader+`
func f(_ context.Context) int { return 1 }
`)

	assertClean(t, "internal/a/a.go", pkgHeader+`
func f(ctx context.Context) error { return ctx.Err() }
`)
}

// The discard is the form that hides an unused context best: the parameter
// looks read, so nothing objects, while the code still ignores it.
func TestBlankDiscardDoesNotCountAsUsingTheContext(t *testing.T) {
	assertFlags(t, "internal/a/a.go", pkgHeader+`
func f(ctx context.Context) int {
	_ = ctx
	return 1
}
`, "never used")
}

func TestClosureContextIsHeldToTheSameRule(t *testing.T) {
	assertFlags(t, "internal/a/a.go", pkgHeader+`
var f = func(ctx context.Context) int { return 1 }
`, "never used")
}

func TestContextNamedFunctionMustUseItsContext(t *testing.T) {
	assertFlags(t, "internal/a/a.go", pkgHeader+`
func DialContext(_ context.Context, addr string) string { return addr }
`, "named for a context it discards")

	assertClean(t, "internal/a/a.go", pkgHeader+`
func DialContext(ctx context.Context, addr string) error { _ = addr; return ctx.Err() }
`)

	// A plain name carrying a blank context is the honest case, not a violation.
	assertClean(t, "internal/a/a.go", pkgHeader+`
func Dial(_ context.Context, addr string) string { return addr }
`)
}

func TestSQLCallsMustUseTheContextVariants(t *testing.T) {
	const withSQL = "package p\n\nimport (\n\t\"context\"\n\t\"database/sql\"\n)\n\n"

	assertFlags(t, "internal/a/a.go", withSQL+`
func f(ctx context.Context, db *sql.DB) error {
	_, err := db.Exec("DELETE FROM t")
	if err != nil {
		return ctx.Err()
	}
	return nil
}
`, "drops cancellation")

	assertClean(t, "internal/a/a.go", withSQL+`
func f(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "DELETE FROM t")
	return err
}
`)

	// Exec on something that is not a database is none of this rule's business,
	// which is why the rule only looks at files importing database/sql.
	assertClean(t, "internal/a/a.go", pkgHeader+`
type runner interface{ Exec(string) error }

func f(_ context.Context, r runner) error { return r.Exec("go") }
`)
}

func TestBareRootContextNeedsAReason(t *testing.T) {
	assertFlags(t, "internal/a/a.go", pkgHeader+`
func f() error {
	return g(context.Background())
}

func g(ctx context.Context) error { return ctx.Err() }
`, "say why in a comment")

	assertClean(t, "internal/a/a.go", pkgHeader+`
func f() error {
	// Detached on purpose: this cleanup runs after its caller was cancelled.
	return g(context.Background())
}

func g(ctx context.Context) error { return ctx.Err() }
`)
}

// Deriving a context establishes a new lifecycle root rather than discarding an
// existing one, so it needs no defence.
func TestDerivedRootContextIsAllowed(t *testing.T) {
	assertClean(t, "internal/a/a.go", pkgHeader+`
func f() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx.Err()
}
`)
}

func TestCompositionRootMayMintAContext(t *testing.T) {
	src := pkgHeader + `
func f() error { return g(context.Background()) }

func g(ctx context.Context) error { return ctx.Err() }
`
	assertClean(t, "main_plugins.go", src)
	assertFlags(t, "internal/a/a.go", src, "say why in a comment")
}

// Presentation gets no comment escape: after reqCtx exists there is exactly one
// correct answer there, so a rationale would only be a well-argued regression.
func TestPresentationMayNotMintAContextEvenWithAComment(t *testing.T) {
	src := pkgHeader + `
func f() error {
	// Some reason, however good.
	return g(context.Background())
}

func g(ctx context.Context) error { return ctx.Err() }
`
	assertFlags(t, "internal/presentation/wails/handlers_x.go", src, "must not mint a root context")
	assertClean(t, contextHelperExempt, src)
}

func TestContextTODOIsNeverAllowed(t *testing.T) {
	src := pkgHeader + `
func f() error {
	// Even with a reason.
	return g(context.TODO())
}

func g(ctx context.Context) error { return ctx.Err() }
`
	assertFlags(t, "internal/a/a.go", src, "placeholder")
	assertFlags(t, "main.go", src, "placeholder")
}

// A comment far above the call is about something else. Three lines is the
// reach; four is an unrelated remark being borrowed as an excuse.
func TestJustificationMustSitNextToTheCall(t *testing.T) {
	assertFlags(t, "internal/a/a.go", pkgHeader+`
func f() error {
	// This comment is about the statements below, not about the context.
	a := 1
	b := 2
	c := 3
	_, _, _ = a, b, c
	return g(context.Background())
}

func g(ctx context.Context) error { return ctx.Err() }
`, "say why in a comment")
}
