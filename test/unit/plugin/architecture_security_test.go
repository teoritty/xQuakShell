package plugin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCapabilityAndDomainDoNotImportIPC guards the layering the channel bus depends on: the
// capability proxy and the domain reach the bus through the ChannelDataPathOpener port, and the
// implementation is handed to them by the composition root alone.
//
// Wiring the proxy straight to ipc.Conn would work and would nail it to one transport forever.
// The port costs a few lines; this test is what keeps someone from spending them and then
// quietly taking the shortcut later.
func TestCapabilityAndDomainDoNotImportIPC(t *testing.T) {
	const ipcPkg = `"ssh-client/internal/infra/plugin/ipc"`
	for _, root := range []string{
		filepath.Join("..", "..", "..", "internal", "infra", "plugin", "capability"),
		filepath.Join("..", "..", "..", "internal", "domain"),
		filepath.Join("..", "..", "..", "internal", "usecase"),
	} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), ipcPkg) {
				t.Fatalf("%s imports the ipc frame layer; it must reach the bus through the "+
					"domain's ChannelDataPathOpener port, wired by the composition root", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestInfraHasNoSessionAuthorizationLogic(t *testing.T) {
	root := filepath.Join("..", "..", "..", "internal", "infra", "plugin")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		if strings.Contains(content, "AuthorizeSessionRPC") ||
			strings.Contains(content, "ScopedSessionProxy") ||
			strings.Contains(content, "enforceMultiSessionPolicy") {
			t.Fatalf("infra must not contain session authorization logic: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
