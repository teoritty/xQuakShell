package domain_test

import (
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

// syncScope is a tree with one plugin scope and one folder outside it:
//
//	sync/            <- the scope root of com.example.sync
//	  prod/
//	    eu/
//	personal/
func syncScope(t *testing.T) domain.ScopeIndex {
	t.Helper()
	index, err := domain.NewScopeIndex(
		[]domain.ConnectionFolder{
			{ID: "sync", Name: "Sync"},
			{ID: "prod", Name: "Prod", ParentID: "sync"},
			{ID: "eu", Name: "EU", ParentID: "prod"},
			{ID: "personal", Name: "Personal"},
		},
		[]domain.ScopeRoot{{FolderID: "sync", PluginID: "com.example.sync"}},
	)
	if err != nil {
		t.Fatalf("NewScopeIndex: %v", err)
	}
	return index
}

// The user builds whatever tree they like inside the scope folder, and everything in it is exposed.
// A rule that only looked one level down would make the nesting a lie.
func TestEverythingBeneathTheScopeRootIsInScope(t *testing.T) {
	index := syncScope(t)

	for _, folderID := range []string{"sync", "prod", "eu"} {
		if !index.Contains("com.example.sync", folderID) {
			t.Errorf("folder %q is beneath the scope root and is not in scope", folderID)
		}
	}
}

// Default-deny is the whole feature. What the user did not put in the folder does not leave the
// machine, and a folder alongside it is not "nearly inside".
func TestAFolderOutsideTheScopeIsNotInIt(t *testing.T) {
	index := syncScope(t)

	if index.Contains("com.example.sync", "personal") {
		t.Error("a folder outside the scope reports as inside it")
	}
	if _, ok := index.PluginFor("personal"); ok {
		t.Error("a folder outside every scope named a plugin")
	}
}

// A connection at the top level belongs to no folder at all, so it belongs to no scope. This is the
// state of every connection in a vault that has never seen a scope folder.
func TestAConnectionWithNoFolderIsInNoScope(t *testing.T) {
	index := syncScope(t)

	if index.Contains("com.example.sync", "") {
		t.Error("an object with no folder reports as in scope")
	}
}

// One plugin's scope is not another's. Two sync plugins installed side by side must not see each
// other's connections just because both have a scope.
func TestOnePluginsScopeIsNotAnothers(t *testing.T) {
	index := syncScope(t)

	if index.Contains("com.example.other", "prod") {
		t.Error("a folder in one plugin's scope reports as in another's")
	}
	if id, _ := index.PluginFor("prod"); id != "com.example.sync" {
		t.Errorf("PluginFor = %q, want the owning plugin", id)
	}
}

// A folder whose parent no longer exists is orphaned, not exposed. The walk has to stop rather than
// keep looking, and stopping must mean "not in scope" rather than "in the last scope seen".
func TestAFolderWithAMissingParentIsInNoScope(t *testing.T) {
	index, err := domain.NewScopeIndex(
		[]domain.ConnectionFolder{{ID: "orphan", Name: "Orphan", ParentID: "gone"}},
		[]domain.ScopeRoot{{FolderID: "sync", PluginID: "com.example.sync"}},
	)
	if err != nil {
		t.Fatalf("NewScopeIndex: %v", err)
	}

	if _, ok := index.PluginFor("orphan"); ok {
		t.Error("an orphaned folder reports as in scope")
	}
}

// A cycle is a refusal rather than a walk that never ends. The vault is a file and can be edited by
// hand, so a parent chain that loops is a state the code must survive - and it is exactly the state
// where "keep walking up" runs forever inside a permission check.
func TestACycleInTheFolderTreeIsRefused(t *testing.T) {
	_, err := domain.NewScopeIndex(
		[]domain.ConnectionFolder{
			{ID: "a", Name: "A", ParentID: "b"},
			{ID: "b", Name: "B", ParentID: "a"},
		},
		nil,
	)

	if !errors.Is(err, domain.ErrInvalidScope) {
		t.Fatalf("err = %v, want ErrInvalidScope for a parent cycle", err)
	}
}

// A folder that is its own parent is the same hazard in its shortest form.
func TestAFolderThatIsItsOwnParentIsRefused(t *testing.T) {
	_, err := domain.NewScopeIndex(
		[]domain.ConnectionFolder{{ID: "a", Name: "A", ParentID: "a"}},
		nil,
	)

	if !errors.Is(err, domain.ErrInvalidScope) {
		t.Fatalf("err = %v, want ErrInvalidScope", err)
	}
}

// Two plugins claiming one folder would make "whose scope is this" unanswerable, and the answer
// decides who may read the credentials inside it.
func TestTwoScopesOnOneFolderAreRefused(t *testing.T) {
	_, err := domain.NewScopeIndex(
		[]domain.ConnectionFolder{{ID: "sync", Name: "Sync"}},
		[]domain.ScopeRoot{
			{FolderID: "sync", PluginID: "com.example.sync"},
			{FolderID: "sync", PluginID: "com.example.other"},
		},
	)

	if !errors.Is(err, domain.ErrInvalidScope) {
		t.Fatalf("err = %v, want ErrInvalidScope for two scopes on one folder", err)
	}
}

// A scope root naming a folder that no longer exists is inert rather than an error. Deleting a
// folder is an ordinary act, and a stale root exposes nothing because nothing is beneath it.
func TestAScopeRootForAMissingFolderExposesNothing(t *testing.T) {
	index, err := domain.NewScopeIndex(
		[]domain.ConnectionFolder{{ID: "personal", Name: "Personal"}},
		[]domain.ScopeRoot{{FolderID: "gone", PluginID: "com.example.sync"}},
	)
	if err != nil {
		t.Fatalf("NewScopeIndex: %v", err)
	}

	if index.Contains("com.example.sync", "personal") {
		t.Error("a stale scope root exposed an unrelated folder")
	}
}

// A scope root with no plugin, or no folder, names nothing. The vault is a file: such a row can be
// there, and it must not become a scope that matches by accident.
func TestAnIncompleteScopeRootIsRefused(t *testing.T) {
	for _, root := range []domain.ScopeRoot{
		{FolderID: "sync"},
		{PluginID: "com.example.sync"},
	} {
		if _, err := domain.NewScopeIndex(
			[]domain.ConnectionFolder{{ID: "sync", Name: "Sync"}},
			[]domain.ScopeRoot{root},
		); !errors.Is(err, domain.ErrInvalidScope) {
			t.Errorf("root %+v: err = %v, want ErrInvalidScope", root, err)
		}
	}
}

// An empty vault has no scopes, which is not an error - it is what every vault looks like until a
// plugin that wants one is installed.
func TestAVaultWithNoScopesIsFine(t *testing.T) {
	index, err := domain.NewScopeIndex(nil, nil)
	if err != nil {
		t.Fatalf("NewScopeIndex: %v", err)
	}

	if index.Contains("com.example.sync", "anything") {
		t.Error("an index with no scopes reported one")
	}
}
