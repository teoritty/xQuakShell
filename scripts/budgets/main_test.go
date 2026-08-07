package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xquakshell/test/unit/architecture"
)

func TestOwnsBaselineKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"internal/usecase/x.go", true},
		{"internal/usecase/x.go::Service.Method", true},
		{"main_plugins.go", true},
		{"frontend/src/api/audit.ts", false},
		{"frontend/src/api/audit.ts::searchAuditLog", false},
		{"frontend/src/lib/FileTree.svelte", false},
		{"frontend/src/lib/FileTree.svelte::handleDrop", false},
		// A symbol whose name happens to end in .go must still be judged on its
		// path half, or the two updaters would fight over it.
		{"frontend/src/lib/x.ts::parseGo", false},
	}
	for _, tt := range tests {
		if got := ownsBaselineKey(tt.key); got != tt.want {
			t.Errorf("ownsBaselineKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

// The absent half of this pair was the bug: a Go-only run deleted every
// frontend function entry, and with it the ratchet on fifteen functions.
func TestCarryForeignFuncsKeepsTheFrontendHalf(t *testing.T) {
	base := map[string]architecture.FuncMeasurement{
		"frontend/src/api/audit.ts::searchAuditLog":    {CodeLines: 8, Params: 6, Nesting: 1},
		"frontend/src/lib/FileTree.svelte::handleDrop": {CodeLines: 29, Params: 2, Nesting: 5},
		"internal/usecase/x.go::Rebuilt":               {CodeLines: 90, Params: 1, Nesting: 2},
	}

	got := carryForeignFuncs(base)

	if len(got) != 2 {
		t.Fatalf("carried %d entries, want 2; the frontend half must survive a Go-only rewrite", len(got))
	}
	if got["frontend/src/api/audit.ts::searchAuditLog"] != base["frontend/src/api/audit.ts::searchAuditLog"] {
		t.Error("a frontend function entry was dropped or altered")
	}
	if _, carried := got["internal/usecase/x.go::Rebuilt"]; carried {
		t.Error("a Go function was carried through; this command re-measures those instead")
	}
}

func TestCarryForeignFilesKeepsTheFrontendHalf(t *testing.T) {
	base := map[string]architecture.FileMeasurement{
		"frontend/src/lib/FileTree.svelte": {CodeLines: 500, ScriptCodeLines: 210},
		"internal/usecase/x.go":            {CodeLines: 400},
	}

	got := carryForeignFiles(base)

	if len(got) != 1 {
		t.Fatalf("carried %d entries, want 1", len(got))
	}
	if got["frontend/src/lib/FileTree.svelte"].ScriptCodeLines != 210 {
		t.Error("the svelte script measurement was lost in the carry-through")
	}
}

func TestForeignEntryLossIsRefused(t *testing.T) {
	before := architecture.Baseline{
		Files:     map[string]architecture.FileMeasurement{"frontend/src/a.ts": {CodeLines: 400}},
		Functions: map[string]architecture.FuncMeasurement{"frontend/src/a.ts::f": {CodeLines: 90}},
	}

	lostFunc := architecture.Baseline{
		Files:     before.Files,
		Functions: map[string]architecture.FuncMeasurement{},
	}
	err := assertForeignPreserved(before, lostFunc)
	if err == nil {
		t.Fatal("a rewrite that deleted a frontend function entry was allowed through")
	}
	if !strings.Contains(err.Error(), "frontend/src/a.ts::f") {
		t.Errorf("error %q does not name the entry that would be lost", err)
	}

	lostFile := architecture.Baseline{
		Files:     map[string]architecture.FileMeasurement{},
		Functions: before.Functions,
	}
	if err := assertForeignPreserved(before, lostFile); err == nil {
		t.Error("a rewrite that deleted a frontend file entry was allowed through")
	}
}

func TestForeignEntryPreservationAllowsOwnedEntriesToGo(t *testing.T) {
	before := architecture.Baseline{
		Files: map[string]architecture.FileMeasurement{
			"frontend/src/a.ts":     {CodeLines: 400},
			"internal/usecase/x.go": {CodeLines: 400},
		},
		Functions: map[string]architecture.FuncMeasurement{
			"internal/usecase/x.go::f": {CodeLines: 90},
		},
	}
	// A Go file that fell back under budget is supposed to leave the baseline;
	// that is the ratchet tightening, not a loss.
	after := architecture.Baseline{
		Files:     map[string]architecture.FileMeasurement{"frontend/src/a.ts": {CodeLines: 400}},
		Functions: map[string]architecture.FuncMeasurement{},
	}
	if err := assertForeignPreserved(before, after); err != nil {
		t.Errorf("dropping this command's own entries must be allowed, got %v", err)
	}
}

// The unit tests above cover the carry-through helpers; this one covers that
// rebuildFuncBaseline actually calls one. Restoring the original bug - an empty
// starting map - passes every helper test and fails only here, which is why it
// exists: the defect was never in the helper, it was in its absence.
func TestRebuildFuncBaselineKeepsForeignEntries(t *testing.T) {
	root := t.TempDir()
	// budgetScanRoots must all exist or the measurement walk errors out; empty
	// ones make the measured set empty, isolating the carry-through.
	for _, dir := range []string{"internal", "scripts", filepath.Join("test", "unit")} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}

	cfg := architecture.BudgetConfig{
		Baseline: architecture.Baseline{
			Functions: map[string]architecture.FuncMeasurement{
				"frontend/src/api/audit.ts::searchAuditLog": {CodeLines: 8, Params: 6, Nesting: 1},
				"internal/gone.go::Shrunk":                  {CodeLines: 90, Params: 1, Nesting: 2},
			},
		},
	}

	if _, err := rebuildFuncBaseline(root, &cfg); err != nil {
		t.Fatalf("rebuildFuncBaseline: %v", err)
	}

	if _, kept := cfg.Baseline.Functions["frontend/src/api/audit.ts::searchAuditLog"]; !kept {
		t.Error("a Go-only rebuild deleted the frontend function entry; that silently drops its ratchet")
	}
	if _, kept := cfg.Baseline.Functions["internal/gone.go::Shrunk"]; kept {
		t.Error("a Go function that no longer exceeds any limit stayed in the baseline")
	}
}
