package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func measureSingleFunc(t *testing.T, src string) FuncShape {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "shape.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// Every line of the fixture carries code; the line-set intersection is
		// covered by TestCountGoCodeLines.
		codeLines := map[int]bool{}
		for line := 1; line <= fset.Position(file.End()).Line; line++ {
			codeLines[line] = true
		}
		return FuncShape{
			CodeLines: bodyCodeLines(fset, fn, codeLines),
			Params:    countParams(fn),
			Nesting:   maxNesting(fn.Body),
		}
	}
	t.Fatal("fixture declared no function")
	return FuncShape{}
}

func TestCountParams(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"grouped params count once each", "func f(a, b, c int) {}", 3},
		{"results are not parameters", "func f(a int) (int, error) { return a, nil }", 1},
		{"the receiver is not a parameter", "func (r *T) f(a int) {}", 1},
		{"an unnamed parameter still counts", "func f(int, string) {}", 2},
		{"variadic counts as one", "func f(a int, rest ...string) {}", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := measureSingleFunc(t, tc.src).Params; got != tc.want {
				t.Errorf("params = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMaxNesting(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "straight line code has no nesting",
			src:  "func f() {\nx := 1\n_ = x\n}",
			want: 0,
		},
		{
			name: "an else-if chain stays at one level",
			src:  "func f(n int) {\nif n == 1 {\n} else if n == 2 {\n} else if n == 3 {\n} else {\n}\n}",
			want: 1,
		},
		{
			name: "a switch is one level regardless of how many cases it has",
			src:  "func f(n int) {\nswitch n {\ncase 1:\ncase 2:\ncase 3:\n}\n}",
			want: 1,
		},
		{
			name: "statements inside a case nest below the switch",
			src:  "func f(n int) {\nswitch n {\ncase 1:\nfor i := 0; i < n; i++ {\n}\n}\n}",
			want: 2,
		},
		{
			name: "a bare block scopes variables without adding a level",
			src:  "func f() {\n{\nx := 1\n_ = x\n}\n}",
			want: 0,
		},
		{
			name: "a closure keeps the depth it appears at",
			src:  "func f(items []int) {\nfor range items {\ngo func() {\nif true {\n}\n}()\n}\n}",
			want: 2,
		},
		{
			name: "range inside if inside for is three levels",
			src:  "func f(items []int) {\nfor range items {\nif len(items) > 0 {\nfor range items {\n}\n}\n}\n}",
			want: 3,
		},
		{
			name: "select counts like a switch",
			src:  "func f(c chan int) {\nselect {\ncase <-c:\ncase <-c:\n}\n}",
			want: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := measureSingleFunc(t, tc.src).Nesting; got != tc.want {
				t.Errorf("nesting = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBodyCodeLinesExcludesTheSignature(t *testing.T) {
	src := "func f(\na int,\nb int,\n) {\nx := a + b\n_ = x\n}"
	if got := measureSingleFunc(t, src).CodeLines; got != 2 {
		t.Errorf("code lines = %d, want 2; a multi-line signature must not inflate its body", got)
	}
}
