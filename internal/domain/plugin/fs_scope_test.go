package plugin

import (
	"strings"
	"testing"
)

// resolveRoots validates nothing: it expands ${pluginData}, takes filepath.Abs and cleans. So a
// manifest asking for the filesystem root got the filesystem root, while the consent screen said
// "Read files in declared sandbox paths" - the word "sandbox" doing the reassuring while the paths
// did the opposite.
func TestIsBroadFilesystemPatternCatchesWholeFilesystemGrants(t *testing.T) {
	broad := []string{
		"/",
		"//",
		"C:\\",
		"C:/",
		"C:",
		"d:\\",
		"~",
		"~/",
		"..",
		`..\..`,
		"../..",
		"../../etc",
		"data/../..",
	}
	for _, pattern := range broad {
		t.Run(pattern, func(t *testing.T) {
			if !IsBroadFilesystemPattern(pattern) {
				t.Errorf("IsBroadFilesystemPattern(%q) = false; this is not a sandbox path and must not be described as one", pattern)
			}
		})
	}
}

// The ordinary shapes must stay quiet, or the warning appears on every plugin and stops meaning
// anything - which is the failure mode of every security banner that cries wolf.
func TestIsBroadFilesystemPatternLeavesScopedGrantsAlone(t *testing.T) {
	scoped := []string{
		"${pluginData}",
		"${pluginData}/cache",
		"${pluginData}\\cache",
		"/etc/hosts",
		"/var/log/app",
		"C:\\ProgramData\\xquakshell",
		"~/.config/xquakshell",
		"data",
		"data/cache",
		"",
		"   ",
	}
	for _, pattern := range scoped {
		t.Run(pattern, func(t *testing.T) {
			if IsBroadFilesystemPattern(pattern) {
				t.Errorf("IsBroadFilesystemPattern(%q) = true; a scoped path must not raise the warning", pattern)
			}
		})
	}
}

// A pattern anchored on the placeholder cannot escape the plugin's own directory once resolveRoots
// has expanded and cleaned it, so it stays scoped even with a ".." in it.
func TestIsBroadFilesystemPatternTrustsThePluginDataPlaceholder(t *testing.T) {
	if IsBroadFilesystemPattern(PluginDataPlaceholder + "/../sibling") {
		t.Error("a ${pluginData}-anchored pattern was reported as broad")
	}
}

func TestBroadFilesystemPatternsCoversReadAndWriteWithoutDuplicates(t *testing.T) {
	m := &Manifest{}
	m.Capabilities.FS = &FSCaps{
		Read:  []string{"${pluginData}", "/", "~"},
		Write: []string{"/", "${pluginData}/out"},
	}

	got := m.BroadFilesystemPatterns()

	if len(got) != 2 || got[0] != "/" || got[1] != "~" {
		t.Fatalf("BroadFilesystemPatterns() = %v, want [/ ~] in declaration order, deduplicated", got)
	}
	if !m.RequiresBroadFilesystemAccess() {
		t.Error("RequiresBroadFilesystemAccess() = false while broad patterns exist")
	}
}

func TestManifestWithoutFSCapsIsNotBroad(t *testing.T) {
	m := &Manifest{}
	if m.RequiresBroadFilesystemAccess() {
		t.Error("a manifest with no fs capability was reported as requiring broad filesystem access")
	}
}

// The consent screen is the only thing standing between the user and this, so it has to name what
// is being granted. The outbound network line has always listed its patterns; the fs line did not.
func TestPermissionSummaryNamesTheFilesystemPaths(t *testing.T) {
	m := &Manifest{}
	m.Capabilities.FS = &FSCaps{
		Read:  []string{"/", "${pluginData}"},
		Write: []string{"${pluginData}/out"},
	}

	summary := strings.Join(m.PermissionSummary(), "\n")

	for _, want := range []string{"/", "${pluginData}", "${pluginData}/out"} {
		if !strings.Contains(summary, want) {
			t.Errorf("permission summary does not name %q:\n%s", want, summary)
		}
	}
	if !strings.Contains(summary, "whole filesystem") {
		t.Errorf("a grant of / is not called out as covering the whole filesystem:\n%s", summary)
	}
}

func TestPermissionSummaryDoesNotCryWolfOnScopedPaths(t *testing.T) {
	m := &Manifest{}
	m.Capabilities.FS = &FSCaps{Read: []string{"${pluginData}"}}

	summary := strings.Join(m.PermissionSummary(), "\n")

	if strings.Contains(summary, "whole filesystem") {
		t.Errorf("a ${pluginData}-scoped grant raised the whole-filesystem warning:\n%s", summary)
	}
}
