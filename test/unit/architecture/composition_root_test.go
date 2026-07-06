package architecture_test

import (
	"testing"

	"ssh-client/test/unit/architecture"
)

func TestCompositionRoot(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	issues, err := architecture.CheckCompositionRoot(architecture.CheckOptions{RepoRoot: repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		t.Error(issue.String())
	}
}
