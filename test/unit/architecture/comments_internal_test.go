package architecture

import (
	"reflect"
	"testing"
)

func TestSplitIdentifier(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"ErrSessionNotFound", []string{"err", "session", "not", "found"}},
		{"loadDir", []string{"load", "dir"}},
		{"AppAPI.GetSessionState", []string{"app", "a", "p", "i", "get", "session", "state"}},
		{"max_lines", []string{"max", "lines"}},
	}
	for _, tc := range tests {
		if got := splitIdentifier(tc.name); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitIdentifier(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNewWordCount(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		doc        string
		wantFewer  bool // fewer than minNewWords, i.e. the gate should fire
	}{
		{
			name:       "a comment that only restates the name",
			identifier: "EvaluateExpression",
			doc:        "EvaluateExpression evaluates an expression.",
			wantFewer:  true,
		},
		{
			name:       "plural and singular of the same word do not count twice",
			identifier: "LoadDir",
			doc:        "LoadDir loads the dir.",
			wantFewer:  true,
		},
		{
			name:       "grammar filler alone is not content",
			identifier: "Close",
			doc:        "Close closes it.",
			wantFewer:  true,
		},
		{
			name:       "a comment that says why passes",
			identifier: "Stop",
			doc:        "Stop is safe to call twice: the running flag guards a close that would otherwise panic.",
			wantFewer:  false,
		},
		{
			name:       "a sentinel error explaining when it is returned passes",
			identifier: "ErrSessionNotFound",
			doc:        "ErrSessionNotFound is reported when the id names no live session.",
			wantFewer:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newWordCount(tc.doc, tc.identifier)
			if fewer := got < minNewWords; fewer != tc.wantFewer {
				t.Errorf("newWordCount = %d (minNewWords = %d), want the gate to fire = %v", got, minNewWords, tc.wantFewer)
			}
		})
	}
}

func TestProcessResidueMatches(t *testing.T) {
	fires := []string{"// Stage 3 tests inject a fake", "// added in Task 2.6b", "/* Phase 5 */", "// see Tasks 3.1-3.4"}
	for _, s := range fires {
		if !processResidue.MatchString(s) {
			t.Errorf("expected %q to be flagged as process residue", s)
		}
	}

	// Numbered steps explain an algorithm in order, which is the kind of
	// comment the rules want more of, and a bare word is not a stage marker.
	quiet := []string{"// Step 2 decodes the header", "// the staging directory", "// phase shift"}
	for _, s := range quiet {
		if processResidue.MatchString(s) {
			t.Errorf("expected %q not to be flagged", s)
		}
	}
}
