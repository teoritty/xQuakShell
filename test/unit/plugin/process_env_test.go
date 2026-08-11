package plugin_test

import (
	"path/filepath"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	infraplugin "xquakshell/internal/infra/plugin"
)

// envLookup finds a variable in a process env slice. Windows env keys are
// case-insensitive and os.Environ() reports them upper-cased there, so matching
// must not depend on the OS's chosen casing.
func envLookup(env []string, key string) (string, bool) {
	for _, entry := range env {
		k, v, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

func TestPluginProcessEnvBlocksSecretsAndProfilePaths(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\secret`)
	t.Setenv("APPDATA", `C:\Users\secret\AppData\Roaming`)
	t.Setenv("HOME", `/home/secret`)
	t.Setenv("AWS_SECRET_ACCESS_KEY", "super-secret")
	t.Setenv("API_KEY", "leak")
	t.Setenv("SystemRoot", `C:\Windows`)

	env := infraplugin.PluginProcessEnv(t.TempDir(), "com.example.plugin", "sess-1")

	for _, forbidden := range []string{
		"USERPROFILE", "APPDATA", "HOME", "AWS_SECRET_ACCESS_KEY", "API_KEY",
	} {
		if v, ok := envLookup(env, forbidden); ok {
			t.Fatalf("forbidden env leaked: %s=%q", forbidden, v)
		}
	}
	if v, ok := envLookup(env, "XQS_PLUGIN"); !ok || v != "1" {
		t.Fatalf("expected XQS_PLUGIN marker, got %q (present=%v)", v, ok)
	}
	if v, ok := envLookup(env, "XQS_PLUGIN_ID"); !ok || v != "com.example.plugin" {
		t.Fatalf("expected plugin id marker, got %q (present=%v)", v, ok)
	}
	if v, ok := envLookup(env, "XQS_PLUGIN_SESSION_ID"); !ok || v != "sess-1" {
		t.Fatalf("expected session id marker, got %q (present=%v)", v, ok)
	}
	if got, ok := envLookup(env, "SystemRoot"); !ok || got != `C:\Windows` {
		t.Fatalf("expected allowlisted SystemRoot, got %q (present=%v)", got, ok)
	}
}

func TestPluginProcessEnvUsesInstanceTemp(t *testing.T) {
	instanceDataDir := t.TempDir()
	env := infraplugin.PluginProcessEnv(instanceDataDir, "com.example.plugin", "")

	want := filepath.Join(instanceDataDir, "tmp")
	for _, key := range []string{"TEMP", "TMP"} {
		got, ok := envLookup(env, key)
		if !ok {
			t.Fatalf("%s missing from plugin env", key)
		}
		if got != want {
			t.Errorf("%s = %q, want %q; a plugin's temp must live inside its own instance data "+
				"directory, the single root a sandbox can grant it", key, got, want)
		}
	}
}

// TestPluginProcessEnvTempIsNotTheHostStagingDir states the invariant this separation exists for.
// The host unpacks and verifies plugin downloads under <dataRoot>/tmp; handing a plugin process the
// same directory as its TEMP put an attacker-writable window inside the install path.
func TestPluginProcessEnvTempIsNotTheHostStagingDir(t *testing.T) {
	dataRoot := t.TempDir()
	hostStaging := filepath.Join(dataRoot, "tmp")

	instanceDataDir, err := infraplugin.EnsurePluginInstanceDataDir(
		dataRoot, "com.example.plugin", "sess-1", domainplugin.IsolationPerSession)
	if err != nil {
		t.Fatal(err)
	}
	env := infraplugin.PluginProcessEnv(instanceDataDir, "com.example.plugin", "sess-1")

	got, ok := envLookup(env, "TEMP")
	if !ok {
		t.Fatal("TEMP missing from plugin env")
	}
	if got == hostStaging {
		t.Fatalf("TEMP = %q, the host's own staging directory; a plugin able to write there can "+
			"replace a bundle the host has already verified", got)
	}
	if !strings.HasPrefix(got, instanceDataDir) {
		t.Errorf("TEMP = %q, want a path under the instance data dir %q", got, instanceDataDir)
	}
}
