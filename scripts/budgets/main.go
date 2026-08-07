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

// ownsBaselineKey reports whether the Go updater owns this baseline entry.
//
// A key is either a path ("internal/x.go") or a symbol ("internal/x.go::Name"),
// so the extension test runs on the path half of both. One predicate for files
// and functions alike: the two halves used to decide this separately, and the
// function half simply forgot to.
func ownsBaselineKey(key string) bool {
	path, _, _ := strings.Cut(key, "::")
	return strings.HasSuffix(path, ".go")
}

// carryForeignFiles keeps the entries this command does not own. Frontend
// numbers are measured by frontend/src/architecture/updateBaseline.ts; each
// side rewrites its own and preserves the other's.
func carryForeignFiles(base map[string]architecture.FileMeasurement) map[string]architecture.FileMeasurement {
	next := map[string]architecture.FileMeasurement{}
	for path, m := range base {
		if !ownsBaselineKey(path) {
			next[path] = m
		}
	}
	return next
}

// carryForeignFuncs is carryForeignFiles for the function half. Its absence was
// the bug: rebuildFuncBaseline started from an empty map, so every frontend
// function entry was deleted by a Go-only run.
func carryForeignFuncs(base map[string]architecture.FuncMeasurement) map[string]architecture.FuncMeasurement {
	next := map[string]architecture.FuncMeasurement{}
	for symbol, m := range base {
		if !ownsBaselineKey(symbol) {
			next[symbol] = m
		}
	}
	return next
}

// assertForeignPreserved refuses to write a baseline that dropped an entry
// belonging to the other updater.
//
// This guards the class rather than the one bug: a lost entry is not a
// cosmetic diff, it is a lost ratchet. The file stops being tracked as debt and
// its next growth passes the gate unnoticed, which is the opposite of what the
// baseline is for. Silence is what made the original instance survive - it
// deleted fifteen frontend functions and reported success.
func assertForeignPreserved(before, after architecture.Baseline) error {
	for path := range before.Files {
		if !ownsBaselineKey(path) {
			if _, kept := after.Files[path]; !kept {
				return fmt.Errorf("rewrite would drop the baseline entry for %s, which this command does not own", path)
			}
		}
	}
	for symbol := range before.Functions {
		if !ownsBaselineKey(symbol) {
			if _, kept := after.Functions[symbol]; !kept {
				return fmt.Errorf("rewrite would drop the baseline entry for %s, which this command does not own", symbol)
			}
		}
	}
	return nil
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

	before := architecture.Baseline{Files: cfg.Baseline.Files, Functions: cfg.Baseline.Functions}
	exempt := cfg.ExemptFiles()
	limit := cfg.Limits.Go.MaxCodeLines

	next := carryForeignFiles(cfg.Baseline.Files)

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
	if err := assertForeignPreserved(before, cfg.Baseline); err != nil {
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
	fmt.Printf("budgets: %d Go files and %d Go functions baselined\n",
		countOwnedFiles(next), countOwnedFuncs(cfg.Baseline.Functions))
	return nil
}

func rebuildFuncBaseline(repoRoot string, cfg *architecture.BudgetConfig) ([]string, error) {
	measured, err := architecture.MeasureGoFuncs(repoRoot)
	if err != nil {
		return nil, err
	}
	exempt := cfg.ExemptFunctions()
	limit := cfg.Limits.GoFunc

	next := carryForeignFuncs(cfg.Baseline.Functions)
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

// countOwnedFiles and countOwnedFuncs count only this command's own entries, so
// the summary line does not report the frontend's numbers as if they were Go
// ones. Two functions rather than one generic: the rest of this repository is
// written without type parameters, and a counter is a poor reason to be the
// first.
func countOwnedFiles(files map[string]architecture.FileMeasurement) int {
	n := 0
	for path := range files {
		if ownsBaselineKey(path) {
			n++
		}
	}
	return n
}

func countOwnedFuncs(funcs map[string]architecture.FuncMeasurement) int {
	n := 0
	for symbol := range funcs {
		if ownsBaselineKey(symbol) {
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
