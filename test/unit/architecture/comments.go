package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// CommentIssue is a comment that breaks one of the comment rules.
type CommentIssue struct {
	File string
	Line int
	Rule string
}

func (i CommentIssue) String() string {
	return fmt.Sprintf("%s:%d: %s", i.File, i.Line, i.Rule)
}

// processResidue matches the stage, phase or task number a piece of work was
// delivered under. That is scheduling history: it tells a reader nothing about
// the code, and it is wrong the moment the plan changes.
//
// "Step 3" is deliberately absent. Numbered steps are how an algorithm gets
// explained in order, which is exactly the kind of comment this codebase wants
// more of.
var processResidue = regexp.MustCompile(`\b(Stage|Phase|Tasks?)\s+\d`)

// minNewWords is how many words a doc comment must add beyond the identifier's
// own words before it counts as saying something. Two would let
// "Close closes the connection" through on "connection" alone.
const minNewWords = 3

// docFiller are the words a Go doc comment spends on grammar rather than
// meaning. They are removed before counting so that "X returns the Y" is
// measured on Y alone.
var docFiller = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "it": true, "its": true, "this": true,
	"that": true, "these": true, "those": true, "of": true, "for": true, "to": true,
	"in": true, "on": true, "at": true, "by": true, "with": true, "and": true,
	"or": true, "as": true, "from": true, "into": true, "when": true, "if": true,
	"not": true, "no": true, "do": true, "doe": true, "did": true, "ha": true,
	"have": true, "had": true, "will": true, "would": true, "can": true,
	"may": true, "must": true, "should": true, "return": true, "which": true,
	"there": true, "here": true, "any": true, "all": true, "one": true,
	"deprecated": true, "nolint": true,
}

// CheckComments enforces the comment rules over every Go file in the repo,
// tests included: a stale process note misleads a reader wherever it sits.
func CheckComments(repoRoot string) ([]CommentIssue, error) {
	files, err := commentScanFiles(repoRoot)
	if err != nil {
		return nil, err
	}

	var issues []CommentIssue
	for _, rel := range files {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(repoRoot, filepath.FromSlash(rel)), nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		issues = append(issues, checkProcessResidue(rel, fset, file)...)
		issues = append(issues, checkTautologies(rel, fset, file)...)
	}
	return issues, nil
}

func commentScanFiles(repoRoot string) ([]string, error) {
	files, err := GoBudgetFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	tests, err := goTestFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	return append(files, tests...), nil
}

func checkProcessResidue(rel string, fset *token.FileSet, file *ast.File) []CommentIssue {
	var issues []CommentIssue
	for _, group := range file.Comments {
		for _, comment := range group.List {
			match := processResidue.FindString(comment.Text)
			if match == "" {
				continue
			}
			issues = append(issues, CommentIssue{
				File: rel,
				Line: fset.Position(comment.Pos()).Line,
				Rule: fmt.Sprintf("comment carries development-process history (%q). Say what the code does or why; a stage number describes a schedule that no longer exists", strings.TrimSpace(match)),
			})
		}
	}
	return issues
}

// checkTautologies flags a doc comment that only restates its identifier.
// Only exported declarations are checked: an unexported symbol with a thin
// comment is a much smaller problem than an exported one, and widening the
// rule would bury the real hits.
func checkTautologies(rel string, fset *token.FileSet, file *ast.File) []CommentIssue {
	var issues []CommentIssue
	for _, decl := range file.Decls {
		for _, doc := range documentedNames(decl) {
			if newWordCount(doc.text, doc.name) >= minNewWords {
				continue
			}
			issues = append(issues, CommentIssue{
				File: rel,
				Line: fset.Position(doc.pos).Line,
				Rule: fmt.Sprintf("doc comment on %s only restates its name. Explain why it exists, what it guarantees, or what breaks without it - or delete the comment", doc.name),
			})
		}
	}
	return issues
}

type documentedName struct {
	name string
	text string
	pos  token.Pos
}

func documentedNames(decl ast.Decl) []documentedName {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Doc == nil || !d.Name.IsExported() {
			return nil
		}
		return []documentedName{{name: d.Name.Name, text: d.Doc.Text(), pos: d.Doc.Pos()}}
	case *ast.GenDecl:
		return documentedSpecs(d)
	}
	return nil
}

func documentedSpecs(decl *ast.GenDecl) []documentedName {
	var out []documentedName
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			out = append(out, specDoc(s.Name, s.Doc, decl.Doc)...)
		case *ast.ValueSpec:
			for _, name := range s.Names {
				out = append(out, specDoc(name, s.Doc, decl.Doc)...)
			}
		}
	}
	return out
}

// specDoc prefers the spec's own comment and falls back to the declaration's,
// which is where the doc sits for a single-spec `var X = ...` or `type X ...`.
func specDoc(name *ast.Ident, own, group *ast.CommentGroup) []documentedName {
	doc := own
	if doc == nil {
		doc = group
	}
	if doc == nil || !name.IsExported() {
		return nil
	}
	return []documentedName{{name: name.Name, text: doc.Text(), pos: doc.Pos()}}
}

// newWordCount counts the distinct stems a comment contributes beyond the
// words already in the identifier and the grammatical filler around them.
func newWordCount(text, identifier string) int {
	known := map[string]bool{}
	for _, word := range splitIdentifier(identifier) {
		known[stem(word)] = true
	}

	seen := map[string]bool{}
	count := 0
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		s := stem(word)
		if known[s] || docFiller[s] || seen[s] || len(s) < 2 {
			continue
		}
		seen[s] = true
		count++
	}
	return count
}

// splitIdentifier breaks CamelCase and snake_case into lowercase words, so a
// comment repeating "session not found" back at ErrSessionNotFound adds
// nothing.
func splitIdentifier(name string) []string {
	var words []string
	current := strings.Builder{}
	flush := func() {
		if current.Len() > 0 {
			words = append(words, strings.ToLower(current.String()))
			current.Reset()
		}
	}
	for _, r := range name {
		switch {
		case r == '_' || r == '.':
			flush()
		case unicode.IsUpper(r):
			flush()
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return words
}

// stem strips the plural and third-person "s" so "loads" matches "load". It is
// deliberately crude: the comparison only has to catch a comment echoing its
// own identifier, and a wrong stem costs at most one counted word.
func stem(word string) string {
	if len(word) > 3 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") {
		return strings.TrimSuffix(word, "s")
	}
	return word
}
