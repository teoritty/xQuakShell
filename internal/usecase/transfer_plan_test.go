package usecase

import (
	"testing"
	"time"

	"ssh-client/internal/domain"
)

// planFakeFS models both the source tree (walk) and the target tree (listing)
// for buildPlan tests, in a slash namespace (remote ops).
type planFakeFS struct {
	// walk results keyed by root path.
	walk map[string][]sourceEntry
	// target directory listings keyed by dir path.
	target map[string]map[string]domain.FileStat
}

func (f *planFakeFS) ports() planPorts {
	return planPorts{
		walkRoot: func(root string) ([]sourceEntry, error) { return f.walk[root], nil },
		listTargetDir: func(dir string) map[string]domain.FileStat {
			return f.target[dir]
		},
		ops: remotePathOps{},
	}
}

func TestBuildPlanSingleFileNoConflict(t *testing.T) {
	f := &planFakeFS{
		walk: map[string][]sourceEntry{
			"/src/a.txt": {{AbsPath: "/src/a.txt", Rel: "a.txt", Size: 10, ModTime: time.Unix(1, 0)}},
		},
		target: map[string]map[string]domain.FileStat{},
	}
	plan, err := buildPlan(transferKindUpload, "/dst", []string{"/src/a.txt"}, f.ports())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 {
		t.Fatalf("want 1 file, got %d", len(plan.Files))
	}
	if plan.Files[0].Target != "/dst/a.txt" {
		t.Fatalf("target = %q", plan.Files[0].Target)
	}
	if plan.Files[0].HasConflict() {
		t.Fatal("unexpected conflict")
	}
	if len(plan.Conflicts()) != 0 {
		t.Fatal("expected no conflicts")
	}
}

func TestBuildPlanDetectsConflict(t *testing.T) {
	f := &planFakeFS{
		walk: map[string][]sourceEntry{
			"/src/a.txt": {{AbsPath: "/src/a.txt", Rel: "a.txt", Size: 10, ModTime: time.Unix(5, 0)}},
		},
		target: map[string]map[string]domain.FileStat{
			"/dst": {"a.txt": {Exists: true, Size: 99, ModTime: time.Unix(1, 0)}},
		},
	}
	plan, err := buildPlan(transferKindUpload, "/dst", []string{"/src/a.txt"}, f.ports())
	if err != nil {
		t.Fatal(err)
	}
	conflicts := plan.Conflicts()
	if len(conflicts) != 1 {
		t.Fatalf("want 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Conflict.Size != 99 {
		t.Fatalf("target size = %d", conflicts[0].Conflict.Size)
	}
}

func TestBuildPlanExpandsDirectoryAndTargets(t *testing.T) {
	// Dropping directory /src/proj with one nested file. The existing target has
	// proj/keep.txt already, so only that file is a conflict.
	f := &planFakeFS{
		walk: map[string][]sourceEntry{
			"/src/proj": {
				{AbsPath: "/src/proj", Rel: "proj", IsDir: true},
				{AbsPath: "/src/proj/sub", Rel: "proj/sub", IsDir: true},
				{AbsPath: "/src/proj/sub/new.txt", Rel: "proj/sub/new.txt", Size: 3, ModTime: time.Unix(2, 0)},
				{AbsPath: "/src/proj/keep.txt", Rel: "proj/keep.txt", Size: 4, ModTime: time.Unix(2, 0)},
			},
		},
		target: map[string]map[string]domain.FileStat{
			"/dst/proj": {"keep.txt": {Exists: true, Size: 40, ModTime: time.Unix(1, 0)}},
		},
	}
	plan, err := buildPlan(transferKindUpload, "/dst", []string{"/src/proj"}, f.ports())
	if err != nil {
		t.Fatal(err)
	}
	// Directories: /dst/proj and /dst/proj/sub, in walk order.
	if len(plan.Dirs) != 2 || plan.Dirs[0] != "/dst/proj" || plan.Dirs[1] != "/dst/proj/sub" {
		t.Fatalf("dirs = %v", plan.Dirs)
	}
	if len(plan.Files) != 2 {
		t.Fatalf("want 2 files, got %d", len(plan.Files))
	}
	conflicts := plan.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Target != "/dst/proj/keep.txt" {
		t.Fatalf("conflicts = %+v", conflicts)
	}
}

func TestBuildPlanProbesEachDirOnce(t *testing.T) {
	calls := map[string]int{}
	ports := planPorts{
		walkRoot: func(root string) ([]sourceEntry, error) {
			return []sourceEntry{
				{AbsPath: "/s/a.txt", Rel: "a.txt", Size: 1},
				{AbsPath: "/s/b.txt", Rel: "b.txt", Size: 1},
			}, nil
		},
		listTargetDir: func(dir string) map[string]domain.FileStat {
			calls[dir]++
			return nil
		},
		ops: remotePathOps{},
	}
	if _, err := buildPlan(transferKindUpload, "/dst", []string{"/s"}, ports); err != nil {
		t.Fatal(err)
	}
	if calls["/dst"] != 1 {
		t.Fatalf("expected /dst probed once, got %d", calls["/dst"])
	}
}

// The UI reloads the destination directory when a batch finishes. It used to
// derive that directory from the progress label, which for multi-file batches
// is "N items" and not a path at all — listing it failed. DestDir carries the
// real directory instead, so it must survive planning even for many roots.
func TestBuildPlanRecordsDestDir(t *testing.T) {
	f := &planFakeFS{
		walk: map[string][]sourceEntry{
			"/src/a.txt": {{AbsPath: "/src/a.txt", Rel: "a.txt", Size: 1, ModTime: time.Unix(1, 0)}},
			"/src/b.txt": {{AbsPath: "/src/b.txt", Rel: "b.txt", Size: 2, ModTime: time.Unix(1, 0)}},
			"/src/c.txt": {{AbsPath: "/src/c.txt", Rel: "c.txt", Size: 3, ModTime: time.Unix(1, 0)}},
		},
		target: map[string]map[string]domain.FileStat{},
	}
	plan, err := buildPlan(transferKindUpload, "/dst", []string{"/src/a.txt", "/src/b.txt", "/src/c.txt"}, f.ports())
	if err != nil {
		t.Fatal(err)
	}
	if plan.DestDir != "/dst" {
		t.Fatalf("DestDir = %q, want /dst", plan.DestDir)
	}
	if got := planLabel(plan); got != "3 items" {
		t.Fatalf("planLabel = %q, want a non-path label", got)
	}
}
