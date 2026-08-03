package domain

import (
	"testing"
	"time"
)

func stat(size int64, mod time.Time) FileStat {
	return FileStat{Exists: true, Size: size, ModTime: mod}
}

func TestResolveConflictNoTargetAlwaysWrites(t *testing.T) {
	src := stat(10, time.Unix(100, 0))
	absent := FileStat{Exists: false}
	// Every action writes when the target does not exist — there is no conflict.
	for _, a := range []ConflictAction{
		ConflictOverwrite, ConflictOverwriteIfNewer, ConflictOverwriteIfDifferentSize,
		ConflictOverwriteIfNewerOrDifferentSize, ConflictRename, ConflictSkip,
	} {
		if got := ResolveConflict(a, src, absent); got != OutcomeWrite {
			t.Fatalf("action %v with no target: got %v, want write", a, got)
		}
	}
}

func TestResolveConflictOverwriteAndSkip(t *testing.T) {
	src := stat(10, time.Unix(200, 0))
	tgt := stat(20, time.Unix(100, 0))
	if got := ResolveConflict(ConflictOverwrite, src, tgt); got != OutcomeWrite {
		t.Fatalf("overwrite: got %v", got)
	}
	if got := ResolveConflict(ConflictSkip, src, tgt); got != OutcomeSkip {
		t.Fatalf("skip: got %v", got)
	}
	if got := ResolveConflict(ConflictRename, src, tgt); got != OutcomeRename {
		t.Fatalf("rename: got %v", got)
	}
}

func TestResolveConflictIfNewer(t *testing.T) {
	tgt := stat(5, time.Unix(100, 0))
	newer := stat(5, time.Unix(200, 0))
	older := stat(5, time.Unix(50, 0))
	same := stat(5, time.Unix(100, 0))
	if got := ResolveConflict(ConflictOverwriteIfNewer, newer, tgt); got != OutcomeWrite {
		t.Fatalf("newer source should write, got %v", got)
	}
	if got := ResolveConflict(ConflictOverwriteIfNewer, older, tgt); got != OutcomeSkip {
		t.Fatalf("older source should skip, got %v", got)
	}
	if got := ResolveConflict(ConflictOverwriteIfNewer, same, tgt); got != OutcomeSkip {
		t.Fatalf("equal mtime should skip, got %v", got)
	}
}

func TestResolveConflictIfDifferentSize(t *testing.T) {
	tgt := stat(5, time.Unix(100, 0))
	diff := stat(6, time.Unix(50, 0)) // older but different size
	same := stat(5, time.Unix(200, 0))
	if got := ResolveConflict(ConflictOverwriteIfDifferentSize, diff, tgt); got != OutcomeWrite {
		t.Fatalf("different size should write, got %v", got)
	}
	if got := ResolveConflict(ConflictOverwriteIfDifferentSize, same, tgt); got != OutcomeSkip {
		t.Fatalf("same size should skip, got %v", got)
	}
}

func TestResolveConflictIfNewerOrDifferentSize(t *testing.T) {
	tgt := stat(5, time.Unix(100, 0))
	a := ConflictOverwriteIfNewerOrDifferentSize
	// newer, same size → write
	if got := ResolveConflict(a, stat(5, time.Unix(200, 0)), tgt); got != OutcomeWrite {
		t.Fatalf("newer/same-size should write, got %v", got)
	}
	// older, different size → write
	if got := ResolveConflict(a, stat(9, time.Unix(50, 0)), tgt); got != OutcomeWrite {
		t.Fatalf("older/diff-size should write, got %v", got)
	}
	// older, same size → skip
	if got := ResolveConflict(a, stat(5, time.Unix(50, 0)), tgt); got != OutcomeSkip {
		t.Fatalf("older/same-size should skip, got %v", got)
	}
}

func TestParseConflictActionRoundTrip(t *testing.T) {
	for _, a := range []ConflictAction{
		ConflictAsk, ConflictOverwrite, ConflictOverwriteIfNewer, ConflictOverwriteIfDifferentSize,
		ConflictOverwriteIfNewerOrDifferentSize, ConflictRename, ConflictSkip,
	} {
		got, ok := ParseConflictAction(a.String())
		if !ok || got != a {
			t.Fatalf("round trip %q: got %v ok=%v", a.String(), got, ok)
		}
	}
	if _, ok := ParseConflictAction("bogus"); ok {
		t.Fatal("bogus action should not parse")
	}
	// Empty string is treated as "ask" (unset default).
	if got, ok := ParseConflictAction(""); !ok || got != ConflictAsk {
		t.Fatalf("empty should parse to ask, got %v ok=%v", got, ok)
	}
}

func TestNextAvailableName(t *testing.T) {
	existing := map[string]bool{
		"file.txt":     true,
		"file (1).txt": true,
		"noext":        true,
	}
	exists := func(n string) bool { return existing[n] }

	if got := NextAvailableName("file.txt", exists); got != "file (2).txt" {
		t.Fatalf("got %q, want file (2).txt", got)
	}
	// A free name is returned unchanged (the executor only renames on conflict).
	if got := NextAvailableName("fresh.txt", exists); got != "fresh.txt" {
		t.Fatalf("got %q, want fresh.txt", got)
	}
	if got := NextAvailableName("noext", exists); got != "noext (1)" {
		t.Fatalf("got %q, want noext (1)", got)
	}
	// Dotfile: no extension split on a leading dot.
	if got := NextAvailableName(".bashrc", func(string) bool { return false }); got != ".bashrc" {
		t.Fatalf("first free name should be returned unchanged, got %q", got)
	}
}
