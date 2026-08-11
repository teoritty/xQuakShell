package plugin_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite the frozen plugin API golden file")

const goldenPath = "testdata/host_descriptor.golden.json"

// TestFrozenAPISurface pins the entire advertised plugin API surface (envelope version + every
// capability version and feature list) to a checked-in golden file. Any change to the 1.0
// contract fails here until the golden is regenerated (go test -run TestFrozenAPISurface
// -update-golden) AND the diff is reviewed — making an accidental contract change impossible to
// merge silently (ADR-012).
func TestFrozenAPISurface(t *testing.T) {
	got, err := json.MarshalIndent(domainplugin.HostDescriptor(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote golden %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update-golden to create): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("plugin API surface changed.\n--- got ---\n%s\n--- want ---\n%s\nIf intentional, regenerate with -update-golden and review the diff.", got, want)
	}
}

// removedFeatures records features deliberately dropped without a major bump on their capability,
// keyed by "<capability>.<feature>", with the reason.
//
// The escape hatch exists because the alternative is worse than the rule. A capability major is
// matched exactly, so bumping one refuses every plugin that grants that capability until its
// manifest names the new version — including plugins that never touched the removed feature and
// whose behaviour is identical either way. The plugins that genuinely depended on it are the ones
// that name it in requires.features, and checkCapability refuses those on HasFeature regardless of
// version. So the bump costs every author a release and catches nothing extra.
//
// An entry here is the reviewable step that a version number would otherwise stand in for. Anything
// not listed still fails below.
var removedFeatures = map[string]string{
	"session.localEmbedServer": "Mode B removed; the host-side embed broker (ADR-008 Mode A) is the " +
		"only embed path, and it never functioned — HandlePluginReportLocalEmbed returned " +
		"ErrLocalEmbedNotSupported unconditionally, so no plugin can have depended on it working",
}

// TestAPISurfaceAdditiveOnly enforces the additive-only stability contract within a major: no
// capability may drop a feature or lower its version relative to the golden without a major bump,
// or an entry in removedFeatures above.
func TestAPISurfaceAdditiveOnly(t *testing.T) {
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var golden domainplugin.APIDescriptor
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	current := domainplugin.HostDescriptor()

	goldenEnv, _ := domainplugin.ParseSemver(golden.PluginAPI)
	currentEnv, _ := domainplugin.ParseSemver(current.PluginAPI)
	if currentEnv.Major != goldenEnv.Major {
		t.Skip("envelope major changed; additive-only guard does not apply across majors")
	}

	for name, goldCap := range golden.Capabilities {
		curCap, ok := current.Capabilities[name]
		if !ok {
			t.Fatalf("capability %q was removed within a major (requires a major bump)", name)
		}
		gv, _ := domainplugin.ParseSemver(goldCap.Version)
		cv, _ := domainplugin.ParseSemver(curCap.Version)
		if cv.Major != gv.Major {
			continue // this capability had a major bump; removals are allowed there
		}
		have := map[domainplugin.FeatureID]bool{}
		for _, f := range curCap.Features {
			have[f] = true
		}
		for _, f := range goldCap.Features {
			if have[f] {
				continue
			}
			if reason, recorded := removedFeatures[string(name)+"."+string(f)]; recorded {
				t.Logf("%s.%s is a recorded removal: %s", name, f, reason)
				continue
			}
			t.Fatalf("capability %q dropped feature %q within a major (requires a major bump)", name, f)
		}
	}
}
