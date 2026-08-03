package plugin_test

import (
	"strings"
	"testing"

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

func TestPluginProcessEnvUsesPortableTemp(t *testing.T) {
	dataRoot := t.TempDir()
	env := infraplugin.PluginProcessEnv(dataRoot, "com.example.plugin", "")
	wantTemp := strings.ReplaceAll(dataRoot+`\tmp`, `\`, `/`)
	joined := strings.Join(env, "\n")
	if !strings.Contains(strings.ReplaceAll(joined, `\`, `/`), wantTemp) {
		t.Fatalf("expected portable TEMP/TMP under data root, got %q", joined)
	}
}
