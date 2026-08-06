package architecture_test

import (
	"testing"

	"xquakshell/test/unit/architecture"
)

func TestComments(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	issues, err := architecture.CheckComments(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		t.Error(issue.String())
	}
}
