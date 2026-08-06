package architecture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountGoCodeLines(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "blank lines and standalone comments are free",
			src: "package p\n" +
				"\n" +
				"// A long rationale block.\n" +
				"// Second line of it.\n" +
				"\n" +
				"/* and a block comment\n" +
				"   spanning lines */\n" +
				"\n" +
				"var x = 1\n",
			want: 2, // package clause + var
		},
		{
			name: "a trailing comment does not exempt the code it sits on",
			src: "package p\n" +
				"var x = 1 // why x is 1\n",
			want: 2,
		},
		{
			name: "a raw string literal counts every line it spans",
			src: "package p\n" +
				"var q = `select 1\n" +
				"from t\n" +
				"where a = 2`\n",
			want: 4,
		},
		{
			name: "code sharing a line with the end of a block comment still counts",
			src: "package p\n" +
				"/* note */ var x = 1\n",
			want: 2,
		},
	}

	dir := t.TempDir()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, "src.go")
			if err := os.WriteFile(path, []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := countGoCodeLines(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("countGoCodeLines = %d, want %d", got, tc.want)
			}
		})
	}
}
