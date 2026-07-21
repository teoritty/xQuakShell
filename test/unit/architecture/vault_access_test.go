package architecture_test

import (
	"testing"

	"xquakshell/test/unit/architecture"
)

func TestPresentationVaultAccess(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	issues, err := architecture.CheckPresentationVaultAccess(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		t.Error(issue.String())
	}
}
