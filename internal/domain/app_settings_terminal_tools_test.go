package domain

import "testing"

// A vault written before the terminal tools existed has neither binding, and an empty binding
// removes the shortcut rather than restoring it.
func TestTerminalToolsEmptyBindingsTakeTheDefaults(t *testing.T) {
	for _, blank := range []string{"", "   "} {
		got := TerminalToolsSettings{SearchHotkey: blank, MultiInputHotkey: blank}.WithDefaults()
		want := DefaultTerminalToolsSettings()
		if got != want {
			t.Errorf("WithDefaults on %q bindings = %+v, want %+v", blank, got, want)
		}
	}
}

func TestTerminalToolsChosenBindingsAreKept(t *testing.T) {
	chosen := TerminalToolsSettings{SearchHotkey: "Ctrl+F", MultiInputHotkey: "Ctrl+Shift+M"}
	if got := chosen.WithDefaults(); got != chosen {
		t.Errorf("WithDefaults rewrote the user's bindings: %+v, want %+v", got, chosen)
	}
}

// Ctrl+F belongs to the program inside the terminal - forward-char in readline, page down in vim
// and less - so it must not be the default the whole install base inherits.
func TestTerminalSearchDoesNotDefaultToCtrlF(t *testing.T) {
	if got := DefaultTerminalToolsSettings().SearchHotkey; got == "Ctrl+F" {
		t.Errorf("search defaults to %q, which every shell and pager already uses", got)
	}
}

func TestFreshVaultCarriesTheTerminalToolBindings(t *testing.T) {
	got := NewVaultData().Settings.TerminalTools
	if got != DefaultTerminalToolsSettings() {
		t.Errorf("fresh vault terminal tools = %+v, want the defaults", got)
	}
}
