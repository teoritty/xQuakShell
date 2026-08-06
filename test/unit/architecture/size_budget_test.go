package architecture_test

import (
	"testing"

	"xquakshell/test/unit/architecture"
)

func TestGoFileBudgets(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := architecture.LoadBudgetConfig(repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	issues, err := architecture.CheckGoFileBudgets(repoRoot, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		t.Error(issue.String())
	}
}
