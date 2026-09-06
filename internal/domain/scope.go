package domain

import (
	"errors"
	"fmt"
	"slices"
)

// ErrInvalidScope indicates a scope declaration the core refuses to interpret.
var ErrInvalidScope = errors.New("invalid plugin scope")

// ScopeRoot marks one folder as the exposure root of one plugin (ADR-022).
//
// It is created by the core when a plugin declaring a scope is installed, and never by a plugin.
// Letting a plugin create one would let it create a second folder resembling the first, and personal
// connections would be dropped into it by the user's own hand.
type ScopeRoot struct {
	FolderID string `json:"folderId"`
	PluginID string `json:"pluginId"`

	// Version is how much of each device's history this scope has seen, and it lives here because
	// it describes exactly this folder on exactly this machine (T2). It is what decides whether an
	// arriving replica is newer, older or divergent - a question no timestamp can answer, because
	// the clocks belong to different machines and one of them may be a hostile server's.
	//
	// Empty on a scope that has never synchronised, which is not the same as one that has
	// synchronised and seen nothing.
	Version VersionVector `json:"version,omitempty"`
}

// ScopeIndex answers containment questions about the folder tree.
//
// Exposure is positional rather than a flag on each object, which is what the folder buys: there is
// no field to forge, no setter to reach, and no RPC a plugin could call to put itself in scope. It
// is a function of a tree the user edits with ordinary gestures.
//
// It is built once per read and holds no I/O, so the containment rules are testable without a vault.
type ScopeIndex struct {
	parent map[string]string
	roots  map[string]string
}

// NewScopeIndex builds the index from a folder tree and the scopes declared over it.
//
// A cycle in the parent chain is refused rather than walked. The vault is a file and can be edited
// by hand, so a chain that loops is a state this must survive - and it is precisely the state where
// "keep walking up" runs forever inside a permission check.
func NewScopeIndex(folders []ConnectionFolder, roots []ScopeRoot) (ScopeIndex, error) {
	index := ScopeIndex{
		parent: make(map[string]string, len(folders)),
		roots:  make(map[string]string, len(roots)),
	}
	for _, folder := range folders {
		index.parent[folder.ID] = folder.ParentID
	}
	for _, root := range roots {
		if root.FolderID == "" || root.PluginID == "" {
			return ScopeIndex{}, fmt.Errorf("%w: a scope needs both a folder and a plugin", ErrInvalidScope)
		}
		if owner, taken := index.roots[root.FolderID]; taken {
			return ScopeIndex{}, fmt.Errorf("%w: folder %s is claimed by both %s and %s",
				ErrInvalidScope, root.FolderID, owner, root.PluginID)
		}
		index.roots[root.FolderID] = root.PluginID
	}
	if err := index.refuseCycles(); err != nil {
		return ScopeIndex{}, err
	}
	return index, nil
}

// refuseCycles walks every folder to its root once, bounded by the number of folders: a chain longer
// than the tree has revisited something.
func (s ScopeIndex) refuseCycles() error {
	for start := range s.parent {
		current := start
		for step := 0; step <= len(s.parent); step++ {
			next, known := s.parent[current]
			if !known || next == "" {
				break
			}
			current = next
			if step == len(s.parent) {
				return fmt.Errorf("%w: folder %s sits in a parent cycle", ErrInvalidScope, start)
			}
		}
	}
	return nil
}

// PluginFor returns the plugin whose scope contains this folder, walking to the root.
//
// A folder whose parent is missing is orphaned rather than exposed: the walk stops, and stopping
// means "in no scope" rather than "in the last scope seen".
func (s ScopeIndex) PluginFor(folderID string) (string, bool) {
	current := folderID
	for range len(s.parent) + 1 {
		if current == "" {
			return "", false
		}
		if pluginID, isRoot := s.roots[current]; isRoot {
			return pluginID, true
		}
		next, known := s.parent[current]
		if !known {
			return "", false
		}
		current = next
	}
	return "", false
}

// Contains reports whether a folder lies inside one plugin's scope.
func (s ScopeIndex) Contains(pluginID, folderID string) bool {
	owner, ok := s.PluginFor(folderID)
	return ok && owner == pluginID
}

// WithFolders returns an index that also knows about folders which are not in the vault yet.
//
// An arriving replica brings the folders the user made on their other device, and one of them can
// be nested inside the scope root. Checked against the index as it stands, such a folder resolves
// to nothing and reads as out of scope - so the tree it arrives with has to be part of the walk.
//
// The copy is what makes this safe to call on an untrusted document: cycles are refused here, so a
// document naming two folders as each other's parent is rejected rather than walked.
func (s ScopeIndex) WithFolders(folders []ConnectionFolder) (ScopeIndex, error) {
	out := ScopeIndex{
		parent: make(map[string]string, len(s.parent)+len(folders)),
		roots:  make(map[string]string, len(s.roots)),
	}
	for id, parent := range s.parent {
		out.parent[id] = parent
	}
	for id, pluginID := range s.roots {
		out.roots[id] = pluginID
	}
	for _, folder := range folders {
		out.parent[folder.ID] = folder.ParentID
	}
	if err := out.refuseCycles(); err != nil {
		return ScopeIndex{}, err
	}
	return out, nil
}

// revokeScopeRoot drops a plugin's scope, reporting whether there was one.
//
// The folder and everything in it stay where they are: uninstalling a plugin must not destroy data,
// and what has to stop is the exposure rather than the connections. A reinstall gets a fresh folder,
// so nothing the user left behind is exposed again without being moved there on purpose.
func (p *PluginSettings) revokeScopeRoot(pluginID string) bool {
	before := len(p.ScopeRoots)
	p.ScopeRoots = slices.DeleteFunc(p.ScopeRoots, func(root ScopeRoot) bool {
		return root.PluginID == pluginID
	})
	return len(p.ScopeRoots) != before
}
