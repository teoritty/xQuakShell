package architecture

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileSizeIssue is a file that outgrew its budget.
type FileSizeIssue struct {
	File string
	Line int
	Rule string
}

func (i FileSizeIssue) String() string {
	if i.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", i.File, i.Line, i.Rule)
	}
	return fmt.Sprintf("%s: %s", i.File, i.Rule)
}

// fileSizeBudget caps every non-test file matching Pattern inside Dir.
type fileSizeBudget struct {
	Dir      string
	Pattern  string
	MaxLines int
}

// fileSizeBudgets keep the decomposed services from silently re-merging.
var fileSizeBudgets = []fileSizeBudget{
	{Dir: "internal/usecase", Pattern: "github_*.go", MaxLines: 300},
	{Dir: "internal/infra/plugin", Pattern: "process_*.go", MaxLines: 300},
}

// facadeBudget caps a single file that must stay a thin delegate.
type facadeBudget struct {
	Path     string
	MaxLines int
}

var facadeBudgets = []facadeBudget{
	{Path: "internal/usecase/github_plugin_service.go", MaxLines: 100},
	{Path: "internal/infra/plugin/process_host.go", MaxLines: 100},
}

// CheckFileSizes enforces the per-file line budgets. Counts are code lines
// (see countGoCodeLines) and test files are exempt: the budgets exist to stop
// production services from re-growing into God Objects, and a long test is not
// a God Object.
//
// The budgets below were originally calibrated against non-blank counts, which
// included comments. Switching to code lines only loosened them, and they were
// left at their old numbers rather than re-tightened: every budgeted file is
// far enough inside its limit that the difference is noise, and moving the
// numbers in the same commit as the metric would have hidden a real regression
// behind a bookkeeping change.
func CheckFileSizes(repoRoot string) ([]FileSizeIssue, error) {
	var issues []FileSizeIssue

	for _, budget := range fileSizeBudgets {
		found, err := checkBudgetedDir(repoRoot, budget)
		if err != nil {
			return nil, err
		}
		issues = append(issues, found...)
	}

	for _, facade := range facadeBudgets {
		absPath := filepath.Join(repoRoot, filepath.FromSlash(facade.Path))
		if _, err := os.Stat(absPath); err != nil {
			issues = append(issues, FileSizeIssue{
				File: facade.Path,
				Rule: "expected facade file is missing",
			})
			continue
		}
		count, err := countGoCodeLines(absPath)
		if err != nil {
			return nil, err
		}
		if count > facade.MaxLines {
			issues = append(issues, FileSizeIssue{
				File: facade.Path,
				Rule: fmt.Sprintf("%d lines exceeds the facade budget of %d; it must stay a thin delegate", count, facade.MaxLines),
			})
		}
	}
	return issues, nil
}

func checkBudgetedDir(repoRoot string, budget fileSizeBudget) ([]FileSizeIssue, error) {
	dir := filepath.Join(repoRoot, filepath.FromSlash(budget.Dir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", budget.Dir, err)
	}

	var issues []FileSizeIssue
	matched := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		ok, err := filepath.Match(budget.Pattern, name)
		if err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", budget.Pattern, err)
		}
		if !ok {
			continue
		}
		matched++

		count, err := countGoCodeLines(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if count > budget.MaxLines {
			issues = append(issues, FileSizeIssue{
				File: budget.Dir + "/" + name,
				Rule: fmt.Sprintf("%d lines exceeds the budget of %d", count, budget.MaxLines),
			})
		}
	}

	// The PowerShell gate this replaced matched zero files for years because of
	// a wildcard-recursion quirk, so a budget that covers nothing is an error.
	if matched == 0 {
		return nil, fmt.Errorf("budget %s/%s matched no files; the rule is stale", budget.Dir, budget.Pattern)
	}
	return issues, nil
}
