package plugin_test

import (
	"strings"
	"testing"

	infraplugin "ssh-client/internal/infra/plugin"
)

func TestPluginProcessEnvBlocksSecretsAndProfilePaths(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\secret`)
	t.Setenv("APPDATA", `C:\Users\secret\AppData\Roaming`)
	t.Setenv("HOME", `/home/secret`)
	t.Setenv("AWS_SECRET_ACCESS_KEY", "super-secret")
	t.Setenv("API_KEY", "leak")
	t.Setenv("SystemRoot", `C:\Windows`)

	env := infraplugin.PluginProcessEnv(t.TempDir(), "com.example.plugin", "sess-1")
	joined := strings.Join(env, "\n")

	for _, forbidden := range []string{
		"USERPROFILE=", "APPDATA=", "HOME=", "AWS_SECRET_ACCESS_KEY=", "API_KEY=",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("forbidden env leaked: %s in %q", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "XQS_PLUGIN=1") {
		t.Fatal("expected XQS_PLUGIN marker")
	}
	if !strings.Contains(joined, "XQS_PLUGIN_ID=com.example.plugin") {
		t.Fatal("expected plugin id marker")
	}
	if !strings.Contains(joined, "XQS_PLUGIN_SESSION_ID=sess-1") {
		t.Fatal("expected session id marker")
	}
	if !strings.Contains(joined, "SystemRoot=C:\\Windows") {
		t.Fatal("expected allowlisted SystemRoot")
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
