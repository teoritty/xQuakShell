package synctrust_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
)

// Every AppSettings field must be classified. A setting added without a decision fails here, so
// nobody can ship a field that silently syncs - or silently does not.
func TestEveryAppSettingIsClassified(t *testing.T) {
	rt := reflect.TypeOf(domain.AppSettings{})
	for i := 0; i < rt.NumField(); i++ {
		name := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if _, ok := synctrust.SettingClassOf(name); !ok {
			t.Errorf("AppSettings.%s (json %q) has no sync classification", rt.Field(i).Name, name)
		}
	}
}

func sampleSettings() domain.AppSettings {
	s := domain.NewVaultData().Settings
	s.ExternalEditorPath = `C:\editor.exe`
	s.UIScalePercent = 125
	s.LocalTerminal.ShellID = "pwsh"
	s.Plugins.AllowUnsandboxedFallback = true
	s.Debug.LogLevel = "debug"
	s.Theme = "light"
	s.Language = "ru"
	s.Terminal.FontSize = 17
	return *s
}

func TestDeviceLocalSettingsNeverLeave(t *testing.T) {
	sections, err := synctrust.SplitSettings(sampleSettings())
	if err != nil {
		t.Fatal(err)
	}
	all, _ := json.Marshal(sections)
	for _, secret := range []string{`C:\\editor.exe`, `pwsh`, `allowUnsandboxedFallback`, `uiScalePercent`, `logLevel`, `"debug"`, `auditLog`} {
		if strings.Contains(string(all), secret) {
			t.Errorf("device-local setting %s is in the synced sections", secret)
		}
	}
	if !strings.Contains(string(sections["appearance"]), `"light"`) || !strings.Contains(string(sections["terminal"]), "17") {
		t.Fatalf("synced settings missing: %s", all)
	}
}

func TestApplyingASectionKeepsDeviceLocalFields(t *testing.T) {
	remote := sampleSettings()
	remote.Terminal.FontSize = 20
	remote.Plugins.RequireSignedPlugins = false
	sections, _ := synctrust.SplitSettings(remote)
	local := *domain.NewVaultData().Settings
	local.LocalTerminal.ShellID = "bash"
	local.ExternalEditorPath = "/usr/bin/vim"
	local.UIScalePercent = 150
	remote.Plugins.AllowUnsandboxedFallback = false
	local.Plugins.AllowUnsandboxedFallback = true
	for section, data := range sections {
		var err error
		local, err = synctrust.ApplySection(local, section, data)
		if err != nil {
			t.Fatalf("%s: %v", section, err)
		}
	}
	if local.Terminal.FontSize != 20 || local.Theme != "light" || local.Language != "ru" {
		t.Fatal("synced settings were not applied")
	}
	if local.LocalTerminal.ShellID != "bash" || local.ExternalEditorPath != "/usr/bin/vim" || !local.Plugins.AllowUnsandboxedFallback || local.UIScalePercent != 150 {
		t.Fatalf("a device-local setting was overwritten: %+v", local)
	}
	if local.Plugins.RequireSignedPlugins {
		t.Fatal("ApplySection must apply the data as given; trust gating is Classify's job, not this one's")
	}
}

func TestApplySectionRefusesUnknownSectionsAndFields(t *testing.T) {
	local := *domain.NewVaultData().Settings
	if _, err := synctrust.ApplySection(local, "nonexistent", json.RawMessage(`{}`)); err == nil {
		t.Error("an unknown section was applied")
	}
	if _, err := synctrust.ApplySection(local, "terminal", json.RawMessage(`{"fontSize":12,"evil":1}`)); err == nil {
		t.Error("an unknown field was applied")
	}
	if _, err := synctrust.ApplySection(local, "localTerminal", json.RawMessage(`{"shellId":"evil"}`)); err == nil {
		t.Error("a device-local field arriving in a section was applied; a hostile device could pick this machine's shell")
	}
}

// The registry's section list and the codec table are two lists of the same thing; a section in one
// and not the other is either a record type nobody can apply or a section nobody can read.
func TestSettingsSectionsMatchTheRegistry(t *testing.T) {
	sections, _ := synctrust.SplitSettings(*domain.NewVaultData().Settings)
	known := synctrust.KnownSchemas()
	n := 0
	for ty := range known {
		if section, ok := synctrust.IsSettingsType(synctrust.RecordType(ty)); ok {
			n++
			if _, ok := sections[section]; !ok {
				t.Errorf("registry section %s has no codec", section)
			}
		}
	}
	if n != len(sections) {
		t.Errorf("registry has %d settings sections, the codec table %d", n, len(sections))
	}
}
