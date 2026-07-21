package usecase

import (
	"context"
	"testing"
	"time"

	"ssh-client/internal/domain"
)

type movedFile struct {
	source string
	target string
}

// fakeMover records executor actions; existing seeds per-directory name sets.
type fakeMover struct {
	existing map[string]map[string]bool
	moved    []movedFile
	removed  []string
	ensured  []string
}

func (m *fakeMover) ensureDir(_ context.Context, dir string) error {
	m.ensured = append(m.ensured, dir)
	return nil
}
func (m *fakeMover) existingNames(_ context.Context, dir string) map[string]bool {
	return m.existing[dir]
}
func (m *fakeMover) removeTarget(_ context.Context, p string) error {
	m.removed = append(m.removed, p)
	return nil
}
func (m *fakeMover) moveFile(_ context.Context, source, target string, progress domain.ProgressFunc) error {
	m.moved = append(m.moved, movedFile{source, target})
	if progress != nil {
		progress(1, 1)
	}
	return nil
}

type reportSink struct {
	lastState string
	lastDone  int64
	lastTotal int64
}

func (r *reportSink) fn(done, total int64, state string) {
	r.lastDone, r.lastTotal = done, total
	if state != "active" {
		r.lastState = state
	}
}

func fileWithConflict(target string, size int64, tgt *domain.FileStat) PlannedFile {
	return PlannedFile{Source: "/s" + target, Target: target, Size: size, SrcModTime: time.Unix(100, 0), Conflict: tgt}
}

func run(t *testing.T, plan *TransferPlan, res map[string]ResolvedAction, mover *fakeMover) *reportSink {
	t.Helper()
	sink := &reportSink{}
	if err := executePlanCore(context.Background(), plan, res, mover, sink.fn); err != nil {
		t.Fatalf("executePlanCore: %v", err)
	}
	return sink
}

func TestExecuteOverwriteWritesTarget(t *testing.T) {
	tgt := &domain.FileStat{Exists: true, Size: 50, ModTime: time.Unix(1, 0)}
	plan := &TransferPlan{Kind: transferKindUpload, Files: []PlannedFile{fileWithConflict("/dst/a.txt", 10, tgt)}}
	mover := &fakeMover{}
	sink := run(t, plan, map[string]ResolvedAction{"/dst/a.txt": {Action: domain.ConflictOverwrite}}, mover)

	if len(mover.moved) != 1 || mover.moved[0].target != "/dst/a.txt" {
		t.Fatalf("moved = %+v", mover.moved)
	}
	if sink.lastState != "completed" || sink.lastTotal != 10 || sink.lastDone != 10 {
		t.Fatalf("report = %+v", sink)
	}
}

func TestExecuteSkipDoesNotWrite(t *testing.T) {
	tgt := &domain.FileStat{Exists: true, Size: 50}
	plan := &TransferPlan{Kind: transferKindUpload, Files: []PlannedFile{fileWithConflict("/dst/a.txt", 10, tgt)}}
	mover := &fakeMover{}
	sink := run(t, plan, map[string]ResolvedAction{"/dst/a.txt": {Action: domain.ConflictSkip}}, mover)

	if len(mover.moved) != 0 {
		t.Fatalf("expected no move, got %+v", mover.moved)
	}
	if sink.lastTotal != 0 {
		t.Fatalf("expected total 0, got %d", sink.lastTotal)
	}
}

func TestExecuteUnresolvedConflictSkips(t *testing.T) {
	tgt := &domain.FileStat{Exists: true, Size: 50}
	plan := &TransferPlan{Kind: transferKindUpload, Files: []PlannedFile{fileWithConflict("/dst/a.txt", 10, tgt)}}
	mover := &fakeMover{}
	run(t, plan, map[string]ResolvedAction{}, mover) // no resolution
	if len(mover.moved) != 0 {
		t.Fatalf("unresolved conflict must not write, got %+v", mover.moved)
	}
}

func TestExecuteRenamePicksFreeName(t *testing.T) {
	tgt := &domain.FileStat{Exists: true, Size: 50}
	plan := &TransferPlan{Kind: transferKindUpload, Files: []PlannedFile{fileWithConflict("/dst/a.txt", 10, tgt)}}
	mover := &fakeMover{existing: map[string]map[string]bool{"/dst": {"a.txt": true}}}
	run(t, plan, map[string]ResolvedAction{"/dst/a.txt": {Action: domain.ConflictRename}}, mover)

	if len(mover.moved) != 1 || mover.moved[0].target != "/dst/a (1).txt" {
		t.Fatalf("rename target = %+v", mover.moved)
	}
}

func TestExecuteRenameExplicitName(t *testing.T) {
	tgt := &domain.FileStat{Exists: true, Size: 50}
	plan := &TransferPlan{Kind: transferKindUpload, Files: []PlannedFile{fileWithConflict("/dst/a.txt", 10, tgt)}}
	mover := &fakeMover{existing: map[string]map[string]bool{"/dst": {"a.txt": true}}}
	run(t, plan, map[string]ResolvedAction{"/dst/a.txt": {Action: domain.ConflictRename, NewName: "copy.txt"}}, mover)

	if mover.moved[0].target != "/dst/copy.txt" {
		t.Fatalf("explicit rename target = %q", mover.moved[0].target)
	}
}

func TestExecuteTwoRenamesDoNotCollide(t *testing.T) {
	tgt := &domain.FileStat{Exists: true, Size: 1}
	plan := &TransferPlan{Kind: transferKindUpload, Files: []PlannedFile{
		fileWithConflict("/dst/a.txt", 1, tgt),
		fileWithConflict("/dst/a.txt", 1, tgt),
	}}
	mover := &fakeMover{existing: map[string]map[string]bool{"/dst": {"a.txt": true}}}
	run(t, plan, map[string]ResolvedAction{"/dst/a.txt": {Action: domain.ConflictRename}}, mover)

	if len(mover.moved) != 2 || mover.moved[0].target == mover.moved[1].target {
		t.Fatalf("renames collided: %+v", mover.moved)
	}
}

func TestExecuteTypeMismatchRemovesFirst(t *testing.T) {
	tgt := &domain.FileStat{Exists: true, IsDir: true}
	plan := &TransferPlan{Kind: transferKindUpload, Files: []PlannedFile{fileWithConflict("/dst/a", 10, tgt)}}
	mover := &fakeMover{}
	run(t, plan, map[string]ResolvedAction{"/dst/a": {Action: domain.ConflictOverwrite}}, mover)

	if len(mover.removed) != 1 || mover.removed[0] != "/dst/a" {
		t.Fatalf("expected removeTarget for dir mismatch, got %+v", mover.removed)
	}
	if len(mover.moved) != 1 {
		t.Fatalf("expected file write after removal, got %+v", mover.moved)
	}
}

func TestExecuteNonConflictWrites(t *testing.T) {
	plan := &TransferPlan{Kind: transferKindUpload, Files: []PlannedFile{fileWithConflict("/dst/new.txt", 7, nil)}}
	mover := &fakeMover{}
	sink := run(t, plan, nil, mover)
	if len(mover.moved) != 1 || sink.lastTotal != 7 {
		t.Fatalf("non-conflict should write; moved=%+v total=%d", mover.moved, sink.lastTotal)
	}
}

func TestExecuteConditionalNewerSkipsOlderSource(t *testing.T) {
	// Source older than target; "overwrite if newer" must skip.
	tgt := &domain.FileStat{Exists: true, Size: 1, ModTime: time.Unix(500, 0)}
	f := PlannedFile{Source: "/s/a.txt", Target: "/dst/a.txt", Size: 1, SrcModTime: time.Unix(100, 0), Conflict: tgt}
	plan := &TransferPlan{Kind: transferKindUpload, Files: []PlannedFile{f}}
	mover := &fakeMover{}
	run(t, plan, map[string]ResolvedAction{"/dst/a.txt": {Action: domain.ConflictOverwriteIfNewer}}, mover)
	if len(mover.moved) != 0 {
		t.Fatalf("older source should be skipped, got %+v", mover.moved)
	}
}

func TestExecuteEnsuresDirsFirst(t *testing.T) {
	plan := &TransferPlan{
		Kind:  transferKindUpload,
		Dirs:  []string{"/dst/proj", "/dst/proj/sub"},
		Files: []PlannedFile{fileWithConflict("/dst/proj/sub/f.txt", 3, nil)},
	}
	mover := &fakeMover{}
	run(t, plan, nil, mover)
	if len(mover.ensured) < 2 || mover.ensured[0] != "/dst/proj" || mover.ensured[1] != "/dst/proj/sub" {
		t.Fatalf("dirs not ensured in order: %+v", mover.ensured)
	}
}
