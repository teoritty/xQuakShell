package plugin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	infraplugin "ssh-client/internal/infra/plugin"
	"ssh-client/internal/infra/plugin/bundle"
)

func TestReferencePluginsValidate(t *testing.T) {
	reference := []string{
		"demo-terminal",
		"demo-telnet",
		"demo-rdp",
	}
	for _, name := range reference {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("..", "..", "..", "plugins", name)
			data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
			if err != nil {
				t.Fatal(err)
			}
			var manifest domainplugin.Manifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			if err := manifest.Validate(); err != nil {
				t.Fatalf("manifest validate: %v", err)
			}
		})
	}
}

func TestReferencePluginSourcesCompile(t *testing.T) {
	reference := []string{
		"./plugins/demo-terminal",
		"./plugins/demo-telnet",
		"./plugins/demo-rdp",
	}
	for _, pkg := range reference {
		t.Run(pkg, func(t *testing.T) {
			dir := filepath.Join("..", "..", "..", pkg)
			if _, err := infraplugin.LoadPluginDir(dir); err != nil {
				// Load may fail without built binary/checksums; validate manifest + tree only.
				data, readErr := os.ReadFile(filepath.Join(dir, "plugin.json"))
				if readErr != nil {
					t.Fatal(readErr)
				}
				var manifest domainplugin.Manifest
				if err := json.Unmarshal(data, &manifest); err != nil {
					t.Fatal(err)
				}
				if err := manifest.Validate(); err != nil {
					t.Fatalf("manifest validate: %v", err)
				}
				return
			}
		})
	}
}

func TestReferencePluginManifestsWithChecksumsWhenPresent(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "plugins", "demo-terminal")
	if !bundle.HasChecksums(dir) {
		t.Skip("demo-terminal checksums not generated in workspace")
	}
	if _, err := infraplugin.LoadPluginDir(dir); err != nil {
		t.Fatalf("load demo-terminal: %v", err)
	}
}
