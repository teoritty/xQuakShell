package domain

import (
	"fmt"
	"strings"
	"time"
)

// ConflictAction is the choice applied when a transfer's target already exists.
// It mirrors FileZilla's "file exists action" set, minus Resume.
type ConflictAction int

const (
	// ConflictAsk is a settings-only sentinel meaning "prompt the user". It is
	// never a concrete per-file decision; the executor never receives it.
	ConflictAsk ConflictAction = iota
	// ConflictOverwrite always replaces the target.
	ConflictOverwrite
	// ConflictOverwriteIfNewer replaces only when the source is newer.
	ConflictOverwriteIfNewer
	// ConflictOverwriteIfDifferentSize replaces only when the sizes differ.
	ConflictOverwriteIfDifferentSize
	// ConflictOverwriteIfNewerOrDifferentSize replaces when the source is newer
	// or the sizes differ.
	ConflictOverwriteIfNewerOrDifferentSize
	// ConflictRename writes the source under a non-conflicting name.
	ConflictRename
	// ConflictSkip leaves the target untouched.
	ConflictSkip
)

// conflictActionNames is the single source of truth for the wire vocabulary,
// used by both String and ParseConflictAction so they can never drift.
var conflictActionNames = map[ConflictAction]string{
	ConflictAsk:                             "ask",
	ConflictOverwrite:                       "overwrite",
	ConflictOverwriteIfNewer:                "overwrite_if_newer",
	ConflictOverwriteIfDifferentSize:        "overwrite_if_different_size",
	ConflictOverwriteIfNewerOrDifferentSize: "overwrite_if_newer_or_different_size",
	ConflictRename:                          "rename",
	ConflictSkip:                            "skip",
}

// String returns the stable wire name of the action.
func (a ConflictAction) String() string {
	if s, ok := conflictActionNames[a]; ok {
		return s
	}
	return fmt.Sprintf("conflictAction(%d)", int(a))
}

// ParseConflictAction maps a wire name to a ConflictAction. The empty string is
// treated as ConflictAsk (an unset default). ok is false for unknown names.
func ParseConflictAction(s string) (ConflictAction, bool) {
	if s == "" {
		return ConflictAsk, true
	}
	for action, name := range conflictActionNames {
		if name == s {
			return action, true
		}
	}
	return ConflictAsk, false
}

// FileStat is the minimal metadata the conflict rules need about a path. Exists
// is false when the path is absent (no conflict).
type FileStat struct {
	Exists  bool
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// ConflictOutcome is what to do with one source file at execution time.
type ConflictOutcome int

const (
	// OutcomeWrite transfers the source over the target path.
	OutcomeWrite ConflictOutcome = iota
	// OutcomeSkip leaves the target untouched.
	OutcomeSkip
	// OutcomeRename transfers the source under a fresh non-conflicting name.
	OutcomeRename
)

// ResolveConflict decides the outcome for one file given the chosen action and
// the source/target metadata. It is pure: the same inputs always yield the same
// outcome, and it performs no I/O. When the target does not exist there is no
// conflict and the file is always written.
func ResolveConflict(action ConflictAction, src, tgt FileStat) ConflictOutcome {
	if !tgt.Exists {
		return OutcomeWrite
	}
	switch action {
	case ConflictOverwrite:
		return OutcomeWrite
	case ConflictRename:
		return OutcomeRename
	case ConflictSkip:
		return OutcomeSkip
	case ConflictOverwriteIfNewer:
		return writeIf(src.ModTime.After(tgt.ModTime))
	case ConflictOverwriteIfDifferentSize:
		return writeIf(src.Size != tgt.Size)
	case ConflictOverwriteIfNewerOrDifferentSize:
		return writeIf(src.ModTime.After(tgt.ModTime) || src.Size != tgt.Size)
	default:
		// ConflictAsk or any unknown action must never reach execution; skip
		// rather than silently overwrite.
		return OutcomeSkip
	}
}

func writeIf(cond bool) ConflictOutcome {
	if cond {
		return OutcomeWrite
	}
	return OutcomeSkip
}

// NextAvailableName returns the first name in the sequence base, "base (1)",
// "base (2)", … for which exists reports false, inserting the counter before the
// file extension ("file.txt" → "file (1).txt"). A leading dot is treated as part
// of the name, not an extension separator (".bashrc" → ".bashrc (1)").
func NextAvailableName(base string, exists func(name string) bool) string {
	if !exists(base) {
		return base
	}
	stem, ext := splitExt(base)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if !exists(candidate) {
			return candidate
		}
	}
}

// splitExt splits name into its stem and extension (including the dot). A dot at
// position 0 does not count as an extension separator.
func splitExt(name string) (stem, ext string) {
	idx := strings.LastIndex(name, ".")
	if idx <= 0 {
		return name, ""
	}
	return name[:idx], name[idx:]
}
