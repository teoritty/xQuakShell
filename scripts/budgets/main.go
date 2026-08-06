// Command budgets reports and re-records the size baselines in
// code-budgets.json.
//
// It shares the measuring code with the gate in test/unit/architecture rather
// than reimplementing it, because a regenerator that counts differently from
// the checker produces a baseline that fails the moment it is written.
//
// Usage:
//
//	go run ./scripts/budgets           report drift, exit 1 if any
//	go run ./scripts/budgets -update   rewrite the baseline from the tree
//
// -update never touches limits or exemptions. Those are human decisions; only
// the debt numbers are machine-owned.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"xquakshell/test/unit/architecture"
)

func main() {
	update := flag.Bool("update", false, "rewrite the baseline from the working tree")
	flag.Parse()

	repoRoot, err := os.Getwd()
	if err != nil {
		fail(err)
	}

	cfg, err := architecture.LoadBudgetConfig(repoRoot)
	if err != nil {
		fail(err)
	}

	if *update {
		if err := rewriteBaseline(repoRoot, cfg); err != nil {
			fail(err)
		}
		return
	}

	fileIssues, err := architecture.CheckGoFileBudgets(repoRoot, cfg)
	if err != nil {
		fail(err)
	}
	funcIssues, err := architecture.CheckGoFuncBudgets(repoRoot, cfg)
	if err != nil {
		fail(err)
	}
	if len(fileIssues) == 0 && len(funcIssues) == 0 {
		fmt.Println("budgets: no drift")
		return
	}
	for _, issue := range fileIssues {
		fmt.Fprintln(os.Stderr, issue.String())
	}
	for _, issue := range funcIssues {
		fmt.Fprintln(os.Stderr, issue.String())
	}
	os.Exit(1)
}

func rewriteBaseline(repoRoot string, cfg architecture.BudgetConfig) error {
	measured, err := architecture.MeasureGoFiles(repoRoot)
	if err != nil {
		return err
	}
	tracked, err := gitTrackedFiles(repoRoot)
	if err != nil {
		return err
	}

	exempt := cfg.ExemptFiles()
	limit := cfg.Limits.Go.MaxCodeLines

	next := map[string]architecture.FileMeasurement{}
	// Frontend entries are measured by frontend/src/architecture.test.ts and
	// carried through untouched; this command only owns the Go numbers.
	for path, m := range cfg.Baseline.Files {
		if !strings.HasSuffix(path, ".go") {
			next[path] = m
		}
	}

	var added []string
	for path, count := range measured {
		if count <= limit {
			continue
		}
		if _, ok := exempt[path]; ok {
			continue
		}
		if _, existed := cfg.Baseline.Files[path]; !existed {
			if !tracked[path] {
				return fmt.Errorf("%s is over budget at %d code lines and is not tracked by git. "+
					"A file that has never been committed cannot be debt: split it, or commit it first "+
					"and record it deliberately", path, count)
			}
			added = append(added, path)
		}
		next[path] = architecture.FileMeasurement{CodeLines: count}
	}

	cfg.Baseline.Files = next
	addedFuncs, err := rebuildFuncBaseline(repoRoot, &cfg)
	if err != nil {
		return err
	}
	if err := writeConfig(repoRoot, cfg); err != nil {
		return err
	}

	added = append(added, addedFuncs...)
	sort.Strings(added)
	for _, id := range added {
		fmt.Fprintf(os.Stderr, "WARNING: added %s to the baseline. New debt should be paid, not recorded.\n", id)
	}
	fmt.Printf("budgets: %d Go files and %d functions baselined\n", countGo(next), len(cfg.Baseline.Functions))
	return nil
}

func rebuildFuncBaseline(repoRoot string, cfg *architecture.BudgetConfig) ([]string, error) {
	measured, err := architecture.MeasureGoFuncs(repoRoot)
	if err != nil {
		return nil, err
	}
	exempt := cfg.ExemptFunctions()
	limit := cfg.Limits.GoFunc

	next := map[string]architecture.FuncMeasurement{}
	var added []string
	for symbol, shape := range measured {
		if !architecture.ShapeExceeds(shape, limit) {
			continue
		}
		if _, ok := exempt[symbol]; ok {
			continue
		}
		if _, existed := cfg.Baseline.Functions[symbol]; !existed {
			added = append(added, symbol)
		}
		next[symbol] = architecture.FuncMeasurement{
			CodeLines: shape.CodeLines,
			Params:    shape.Params,
			Nesting:   shape.Nesting,
		}
	}
	cfg.Baseline.Functions = next
	return added, nil
}

func countGo(files map[string]architecture.FileMeasurement) int {
	n := 0
	for path := range files {
		if strings.HasSuffix(path, ".go") {
			n++
		}
	}
	return n
}

func writeConfig(repoRoot string, cfg architecture.BudgetConfig) error {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(cfg); err != nil {
		return err
	}
	path := filepath.Join(repoRoot, architecture.BudgetConfigFile)
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func gitTrackedFiles(repoRoot string) (map[string]bool, error) {
	cmd := exec.Command("git", "-C", repoRoot, "ls-files")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	tracked := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			tracked[line] = true
		}
	}
	return tracked, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "budgets:", err)
	os.Exit(1)
}
