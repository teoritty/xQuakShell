package usecase

import (
	"errors"
	"path"
	"path/filepath"
	"time"

	"xquakshell/internal/domain"
)

// errHostFSUnavailable is returned when a local-filesystem operation is
// attempted but no host FS is wired.
var errHostFSUnavailable = errors.New("local file service unavailable")

// PlannedFile is one file a drop will transfer, with the metadata needed to
// resolve a conflict at its destination. Conflict is non-nil exactly when the
// target path already exists.
type PlannedFile struct {
	Source     string
	Target     string
	Size       int64
	SrcModTime time.Time
	Conflict   *domain.FileStat
}

// HasConflict reports whether the target already exists.
func (f PlannedFile) HasConflict() bool { return f.Conflict != nil }

// TransferPlan is the fully-enumerated work of a drop: the target directories to
// ensure and every file to transfer. It carries no bytes and no decisions — the
// executor applies conflict resolutions to it.
type TransferPlan struct {
	Kind string
	// OpID is the operation identifier assigned when the plan is built. The
	// executor reuses it so the scanning phase and the byte-transfer phase share
	// a single Transfers-panel item (scan → progress → completed).
	OpID string
	// DestDir is the directory everything lands in. It is the authoritative
	// answer to "which directory should the UI refresh when this finishes" —
	// unlike the per-transfer display label, which is not a path.
	DestDir string
	Dirs    []string
	Files   []PlannedFile
}

// newOpID mints a unique operation identifier, shared by the planner's scan
// phase (which stamps it onto TransferPlan.OpID) and the executor's byte phase.
// The kind prefix is there to keep logs readable; the identifier itself is
// opaque and is never parsed — neither here nor in the frontend.
func newOpID(kind string) string { return kind + "-" + newRandomID() }

// Conflicts returns the subset of files whose target already exists.
func (p *TransferPlan) Conflicts() []PlannedFile {
	var out []PlannedFile
	for _, f := range p.Files {
		if f.HasConflict() {
			out = append(out, f)
		}
	}
	return out
}

// sourceEntry is one node discovered while walking a dropped source root. Rel is
// slash-normalized and relative to the target directory (it includes the root's
// own base name), so the target path is join(targetDir, Rel).
type sourceEntry struct {
	AbsPath string
	Rel     string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// pathOps abstracts join/split so the planner and executor work against either a
// remote (slash) or local (OS) target namespace without branching.
type pathOps interface {
	Join(dir, rel string) string
	Split(p string) (dir, name string)
}

type remotePathOps struct{}

func (remotePathOps) Join(dir, rel string) string     { return path.Join(dir, rel) }
func (remotePathOps) Split(p string) (string, string) { return path.Dir(p), path.Base(p) }

type localPathOps struct{}

func (localPathOps) Join(dir, rel string) string     { return filepath.Join(dir, filepath.FromSlash(rel)) }
func (localPathOps) Split(p string) (string, string) { return filepath.Dir(p), filepath.Base(p) }

// targetPathOps returns the path namespace of a transfer kind's *target*: remote
// for uploads, local for downloads and local copies.
func targetPathOps(kind string) pathOps {
	if kind == transferKindUpload {
		return remotePathOps{}
	}
	return localPathOps{}
}

const (
	transferKindUpload    = "upload"
	transferKindDownload  = "download"
	transferKindLocalCopy = "localcopy"
)

// planPorts are the filesystem operations buildPlan needs, abstracted so the one
// algorithm serves upload/download/localcopy and stays unit-testable.
type planPorts struct {
	// walkRoot returns every node beneath a dropped source root (including the
	// root itself), each Rel-prefixed with the root's base name.
	walkRoot func(root string) ([]sourceEntry, error)
	// listTargetDir returns name->stat for a target directory. A missing
	// directory yields an empty map (no conflicts); it never errors, so a target
	// we cannot enumerate simply produces no conflict prompts (write-through,
	// matching the pre-existing overwrite behavior).
	listTargetDir func(dir string) map[string]domain.FileStat
	ops           pathOps
}

// buildPlan enumerates roots into a TransferPlan, detecting conflicts by probing
// each distinct target directory once (not once per file).
func buildPlan(kind, targetDir string, roots []string, p planPorts) (*TransferPlan, error) {
	plan := &TransferPlan{Kind: kind, DestDir: targetDir}

	type pendingFile struct {
		entry  sourceEntry
		target string
		parent string
		name   string
	}
	var pending []pendingFile
	dirSeen := map[string]bool{}

	for _, root := range roots {
		entries, err := p.walkRoot(root)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			target := p.ops.Join(targetDir, e.Rel)
			if e.IsDir {
				if !dirSeen[target] {
					dirSeen[target] = true
					plan.Dirs = append(plan.Dirs, target)
				}
				continue
			}
			parent, name := p.ops.Split(target)
			pending = append(pending, pendingFile{entry: e, target: target, parent: parent, name: name})
		}
	}

	// Probe each distinct target directory at most once.
	probeCache := map[string]map[string]domain.FileStat{}
	dirIndex := func(dir string) map[string]domain.FileStat {
		if m, ok := probeCache[dir]; ok {
			return m
		}
		m := p.listTargetDir(dir)
		if m == nil {
			m = map[string]domain.FileStat{}
		}
		probeCache[dir] = m
		return m
	}

	for _, f := range pending {
		pf := PlannedFile{
			Source:     f.entry.AbsPath,
			Target:     f.target,
			Size:       f.entry.Size,
			SrcModTime: f.entry.ModTime,
		}
		if st, ok := dirIndex(f.parent)[f.name]; ok && st.Exists {
			conflict := st
			pf.Conflict = &conflict
		}
		plan.Files = append(plan.Files, pf)
	}
	return plan, nil
}
