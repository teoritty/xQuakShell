package architecture

import (
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The local terminal is a loader for arbitrary code. That is fine for the person at the keyboard,
// who already has a shell on this machine, and catastrophic for anything else that can reach it.
// ADR-018 confines plugin processes precisely so a plugin cannot execute what it likes - Landlock,
// AppContainer, the Linux shim, the escape tests - and a plugin able to ask the host for a shell
// would walk around all of it with one call.
//
// So the rule is structural rather than reviewed: the two subsystems must not name each other.
// Giving a plugin a path to a shell has to break one of these tests first, which is a much harder
// thing to do by accident than adding an entry to a capability registry.
//
// These run on the AST with comments dropped, which matters in both directions: the prose in
// localshell explains why it does not share the plugin spawner's code, and naming the thing you
// are explaining must not fail the test that enforces the explanation.
const (
	localShellPkg  = "xquakshell/internal/infra/localshell"
	pluginInfraPkg = "xquakshell/internal/infra/plugin"
)

var pluginPackageRoots = []string{
	"internal/domain/plugin",
	"internal/infra/plugin",
}

// TestPluginPackagesCannotReachTheLocalTerminal fails if plugin-facing code imports the local
// shell or names its use case. A capability, a host method, a channel purpose or a stray import
// all surface the same way here.
func TestPluginPackagesCannotReachTheLocalTerminal(t *testing.T) {
	root := localTerminalRepoRoot(t)

	for _, pkgRoot := range pluginPackageRoots {
		err := walkGoFiles(root, pkgRoot, func(f parsedFile) error {
			for _, imp := range f.File.Imports {
				if strings.Trim(imp.Path.Value, `"`) == localShellPkg {
					t.Errorf("%s imports %s; a plugin must not be able to start a shell",
						f.RelPath, localShellPkg)
				}
			}
			ast.Inspect(f.File, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if ok && strings.Contains(ident.Name, "LocalTerminal") {
					t.Errorf("%s names %s; exposing this to plugins is an ADR-020 decision, not an identifier",
						f.RelPath, ident.Name)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scanning %s: %v", pkgRoot, err)
		}
	}
}

// TestLocalShellSharesNoCodeWithThePluginSpawner fails if the local shell borrows from the plugin
// process machinery.
//
// Both start processes and the duplication is deliberate. The plugin spawner confines an
// untrusted binary; this one runs the user's own shell unconfined. Sharing an implementation puts
// one switch between those two behaviours, and the day it is flipped the wrong way is the day the
// plugin sandbox silently stops being one. A duplicated process-group kill is the cheaper mistake.
func TestLocalShellSharesNoCodeWithThePluginSpawner(t *testing.T) {
	root := localTerminalRepoRoot(t)

	err := walkGoFiles(root, "internal/infra/localshell", func(f parsedFile) error {
		for _, imp := range f.File.Imports {
			if strings.HasPrefix(strings.Trim(imp.Path.Value, `"`), pluginInfraPkg) {
				t.Errorf("%s imports %s; the confined spawner and the unconfined one must stay apart",
					f.RelPath, pluginInfraPkg)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scanning the local shell package: %v", err)
	}
}

// localTerminalRepoRoot climbs to the directory holding go.mod so the test runs from anywhere.
func localTerminalRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the working directory")
		}
		dir = parent
	}
}
