package domain

import "testing"

func TestApplyTargetMatches(t *testing.T) {
	cases := []struct {
		target ApplyTarget
		isDir  bool
		want   bool
	}{
		{ApplyBoth, true, true},
		{ApplyBoth, false, true},
		{ApplyFilesOnly, true, false},
		{ApplyFilesOnly, false, true},
		{ApplyDirsOnly, true, true},
		{ApplyDirsOnly, false, false},
	}
	for _, c := range cases {
		if got := c.target.Matches(c.isDir); got != c.want {
			t.Fatalf("ApplyTarget(%v).Matches(isDir=%v) = %v, want %v", c.target, c.isDir, got, c.want)
		}
	}
}
