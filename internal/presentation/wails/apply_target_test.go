package wails

import (
	"testing"

	"xquakshell/internal/domain"
)

// The wire values the frontend's ApplyTarget union can hold. All three must map, or a legitimate
// recursive chmod starts failing.
func TestApplyTargetFromStringAcceptsEveryWireValue(t *testing.T) {
	tests := map[string]domain.ApplyTarget{
		"files": domain.ApplyFilesOnly,
		"dirs":  domain.ApplyDirsOnly,
		"both":  domain.ApplyBoth,
	}
	for wire, want := range tests {
		t.Run(wire, func(t *testing.T) {
			got, err := applyTargetFromString(wire)
			if err != nil {
				t.Fatalf("applyTargetFromString(%q) err = %v, want nil", wire, err)
			}
			if got != want {
				t.Errorf("applyTargetFromString(%q) = %v, want %v", wire, got, want)
			}
		})
	}
}

// An unrecognised value used to become ApplyBoth: a typo, or a frontend built against a newer set
// of choices, silently got the widest scope on a recursive permission change. The two sides
// disagreeing is exactly the moment not to guess.
func TestApplyTargetFromStringRefusesAnythingElse(t *testing.T) {
	for _, wire := range []string{"", "all", "Both", "BOTH", "file", "directories", " both ", "everything"} {
		t.Run(wire, func(t *testing.T) {
			got, err := applyTargetFromString(wire)
			if err == nil {
				t.Fatalf("applyTargetFromString(%q) = %v with no error; an unknown value must not "+
					"become a scope", wire, got)
			}
			if got != domain.ApplyTargetUnspecified {
				t.Errorf("applyTargetFromString(%q) returned %v alongside its error; a refused "+
					"value must not carry a usable target", wire, got)
			}
		})
	}
}
