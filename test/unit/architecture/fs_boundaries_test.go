package architecture_test

import (
	"testing"

	"xquakshell/test/unit/architecture"
)

func TestFSBoundaries(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	issues, err := architecture.CheckFSBoundaries(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		t.Error(issue.String())
	}
}
