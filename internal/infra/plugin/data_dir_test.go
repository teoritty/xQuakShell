package plugin

import (
	"path/filepath"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestPluginInstanceDataDirSanitizesSessionID(t *testing.T) {
	root := t.TempDir()
	dir := PluginInstanceDataDir(root, "com.test", "../../evil", domainplugin.IsolationPerSession)
	if filepath.Base(dir) != "______evil" {
		t.Fatalf("expected sanitized session segment, got %q", filepath.Base(dir))
	}
}
