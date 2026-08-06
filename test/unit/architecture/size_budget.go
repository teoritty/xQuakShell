package architecture

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// budgetScanRoots are the trees whose production Go files carry a size budget.
// The composition root sits at ".", which is scanned one level deep so the
// walk does not have to re-exclude every sibling tree.
//
// test/unit is in scope because the gates themselves are production code for
// this purpose: a checker that grew into a God Object stops being read, and an
// unread checker is an unenforced rule. test/fixtures and test/bench are out -
// they are test support, and D4 exempts tests from size limits entirely.
var budgetScanRoots = []string{".", "internal", "scripts", "test/unit"}

// budgetPrunedDirs are skipped anywhere they appear under a scan root.
// testdata holds files that are deliberately malformed or oversized because a
// test asserts against them.
var budgetPrunedDirs = map[string]bool{
	"testdata":     true,
	"vendor":       true,
	"node_modules": true,
	".git":         true,
	"build":        true,
}

// GoBudgetFiles lists every production Go file that carries a size budget,
// as slash-separated repo-relative paths. Exported for scripts/budgets.
func GoBudgetFiles(repoRoot string) ([]string, error) {
	var files []string
	for _, root := range budgetScanRoots {
		found, err := goFilesUnder(repoRoot, root)
		if err != nil {
			return nil, err
		}
		files = append(files, found...)
	}
	sort.Strings(files)
	return files, nil
}

func goFilesUnder(repoRoot, root string) ([]string, error) {
	if root == "." {
		entries, err := os.ReadDir(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("read repo root: %w", err)
		}
		var files []string
		for _, entry := range entries {
			if entry.IsDir() || !isBudgetedGoFile(entry.Name()) {
				continue
			}
			files = append(files, entry.Name())
		}
		return files, nil
	}

	var files []string
	absRoot := filepath.Join(repoRoot, filepath.FromSlash(root))
	err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if budgetPrunedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !isBudgetedGoFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		files = append(files, filepathToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	return files, nil
}

func isBudgetedGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

// MeasureGoFiles reports the code-line count of every budgeted Go file.
func MeasureGoFiles(repoRoot string) (map[string]int, error) {
	files, err := GoBudgetFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(files))
	for _, rel := range files {
		count, err := countGoCodeLines(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		out[rel] = count
	}
	return out, nil
}

// CheckGoFileBudgets enforces the repo-wide code-line limit on Go files, with
// the exemption list for shapes that should not be split and the baseline
// ratchet for debt that predates the gate.
func CheckGoFileBudgets(repoRoot string, cfg BudgetConfig) ([]FileSizeIssue, error) {
	measured, err := MeasureGoFiles(repoRoot)
	if err != nil {
		return nil, err
	}

	limit := cfg.Limits.Go.MaxCodeLines
	exempt := cfg.ExemptFiles()
	var issues []FileSizeIssue

	for _, rel := range sortedFileKeys(measured) {
		count := measured[rel]
		recorded, baselined := cfg.Baseline.Files[rel]

		if _, isExempt := exempt[rel]; isExempt {
			if count <= limit {
				issues = append(issues, FileSizeIssue{
					File: rel,
					Rule: fmt.Sprintf("%d code lines is within the limit of %d, so its exemption in %s is stale; delete the entry", count, limit, BudgetConfigFile),
				})
			}
			continue
		}

		if !baselined {
			if count > limit {
				issues = append(issues, FileSizeIssue{
					File: rel,
					Rule: fmt.Sprintf("%d code lines exceeds the limit of %d; split it, or - if it is wiring, a port surface or a data table - add an exemption with a kind and a reason to %s", count, limit, BudgetConfigFile),
				})
			}
			continue
		}

		issues = append(issues, baselineDrift(rel, count, recorded.CodeLines, limit)...)
	}

	issues = append(issues, missingBaselineEntries(measured, cfg)...)
	issues = append(issues, missingExemptionTargets(measured, cfg)...)
	return issues, nil
}

// baselineDrift reports a baselined file that grew, that fell below its
// recorded number, or that is now inside the limit. All three are failures:
// a ratchet that only catches growth silently loses every improvement, so the
// number has to be re-recorded the moment the file shrinks.
func baselineDrift(rel string, count, recorded, limit int) []FileSizeIssue {
	switch {
	case count > recorded:
		return []FileSizeIssue{{
			File: rel,
			Rule: fmt.Sprintf("grew to %d code lines from the recorded %d. Baselined files may shrink, never grow; move the new code elsewhere or split the file", count, recorded),
		}}
	case count <= limit:
		return []FileSizeIssue{{
			File: rel,
			Rule: fmt.Sprintf("is down to %d code lines and now meets the limit of %d; drop it from the %s baseline (`go run ./scripts/budgets -update`)", count, limit, BudgetConfigFile),
		}}
	case count < recorded:
		return []FileSizeIssue{{
			File: rel,
			Rule: fmt.Sprintf("shrank to %d code lines from the recorded %d; re-record it so the ratchet tightens (`go run ./scripts/budgets -update`)", count, recorded),
		}}
	}
	return nil
}

// missingBaselineEntries catches entries left behind by a deleted or renamed
// file, which would otherwise sit in the config forever as false debt.
func missingBaselineEntries(measured map[string]int, cfg BudgetConfig) []FileSizeIssue {
	var issues []FileSizeIssue
	for _, rel := range sortedFileKeys(cfg.Baseline.Files) {
		if !strings.HasSuffix(rel, ".go") {
			continue // frontend entries belong to frontend/src/architecture.test.ts
		}
		if _, ok := measured[rel]; !ok {
			issues = append(issues, FileSizeIssue{
				File: rel,
				Rule: fmt.Sprintf("is baselined in %s but is not a budgeted Go file (deleted, renamed, or out of scope); remove the entry", BudgetConfigFile),
			})
		}
	}
	return issues
}

func missingExemptionTargets(measured map[string]int, cfg BudgetConfig) []FileSizeIssue {
	var issues []FileSizeIssue
	for _, e := range cfg.Exemptions.Files {
		if !strings.HasSuffix(e.Path, ".go") {
			continue
		}
		if _, ok := measured[e.Path]; !ok {
			issues = append(issues, FileSizeIssue{
				File: e.Path,
				Rule: fmt.Sprintf("is exempted in %s but is not a budgeted Go file (deleted, renamed, or out of scope); remove the entry", BudgetConfigFile),
			})
		}
	}
	return issues
}

func sortedFileKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// goTestFiles lists the test files under the budget scan roots. Tests carry no
// size budget, but the comment rules apply to them: a stale process note
// misleads a reader wherever it sits.
func goTestFiles(repoRoot string) ([]string, error) {
	var files []string
	for _, root := range budgetScanRoots {
		found, err := testFilesUnder(repoRoot, root)
		if err != nil {
			return nil, err
		}
		files = append(files, found...)
	}
	sort.Strings(files)
	return files, nil
}

func testFilesUnder(repoRoot, root string) ([]string, error) {
	isTest := func(name string) bool { return strings.HasSuffix(name, "_test.go") }

	if root == "." {
		entries, err := os.ReadDir(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("read repo root: %w", err)
		}
		var files []string
		for _, entry := range entries {
			if !entry.IsDir() && isTest(entry.Name()) {
				files = append(files, entry.Name())
			}
		}
		return files, nil
	}

	var files []string
	err := filepath.WalkDir(filepath.Join(repoRoot, filepath.FromSlash(root)), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if budgetPrunedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !isTest(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		files = append(files, filepathToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	return files, nil
}
