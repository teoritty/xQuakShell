package main

import (
	"strings"
	"testing"
)

// The vault is unrecoverable once deleted, so the confirmation must be exact:
// only a deliberate "yes" may proceed.
func TestReadConfirmation(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "exact yes", input: "yes\n", want: true},
		{name: "surrounding whitespace", input: "  yes  \n", want: true},
		{name: "no trailing newline", input: "yes", want: true},
		{name: "uppercase is not consent", input: "YES\n", want: false},
		{name: "shorthand is not consent", input: "y\n", want: false},
		{name: "explicit refusal", input: "no\n", want: false},
		{name: "empty line", input: "\n", want: false},
		{name: "closed stream", input: "", want: false},
		{name: "yes with extra words", input: "yes please\n", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readConfirmation(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("readConfirmation(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("readConfirmation(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
