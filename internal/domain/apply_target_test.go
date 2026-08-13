package domain

import "testing"

// ApplyBoth used to be the zero value, so the widest possible scope was what a caller got by
// forgetting to set the field — on a recursive chmod or chown, where "the files under this
// directory" and "everything under this directory" are different operations.
func TestApplyTargetZeroValueIsNotAChoice(t *testing.T) {
	var unset ApplyTarget

	if unset != ApplyTargetUnspecified {
		t.Fatalf("the zero ApplyTarget is %v, want ApplyTargetUnspecified", unset)
	}
	if unset == ApplyBoth || unset == ApplyFilesOnly || unset == ApplyDirsOnly {
		t.Error("the zero value coincides with a real choice; forgetting the field would pick it")
	}
}

// Anything unrecognised changes nothing. The previous default returned true, so an unspecified
// target applied the operation to every entry the walk reached.
func TestApplyTargetMatchesNothingWhenUnrecognised(t *testing.T) {
	unrecognised := []ApplyTarget{ApplyTargetUnspecified, ApplyTarget(99), ApplyTarget(-1)}

	for _, target := range unrecognised {
		for _, isDir := range []bool{true, false} {
			if target.Matches(isDir) {
				t.Errorf("ApplyTarget(%d).Matches(isDir=%v) = true; an unrecognised target must "+
					"change nothing, not everything", target, isDir)
			}
		}
	}
}

// Each real choice still means what it says, in both directions — a filter that matched everything
// or nothing would pass a one-sided test.
func TestApplyTargetMatchesTheNamedChoices(t *testing.T) {
	tests := []struct {
		target      ApplyTarget
		wantForDir  bool
		wantForFile bool
	}{
		{ApplyBoth, true, true},
		{ApplyFilesOnly, false, true},
		{ApplyDirsOnly, true, false},
	}
	for _, tt := range tests {
		if got := tt.target.Matches(true); got != tt.wantForDir {
			t.Errorf("ApplyTarget(%d).Matches(isDir=true) = %v, want %v", tt.target, got, tt.wantForDir)
		}
		if got := tt.target.Matches(false); got != tt.wantForFile {
			t.Errorf("ApplyTarget(%d).Matches(isDir=false) = %v, want %v", tt.target, got, tt.wantForFile)
		}
	}
}
