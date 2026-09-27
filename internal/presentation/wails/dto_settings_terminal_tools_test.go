package wails

import (
	"testing"

	"xquakshell/internal/domain"
)

// The bindings travel to the dialog and back through two hand-written mappings; a field missed on
// either side saves as empty and silently resets the user's choice to the default.
func TestTerminalToolBindingsSurviveTheSettingsRoundTrip(t *testing.T) {
	in := domain.AppSettings{TerminalTools: domain.TerminalToolsSettings{
		SearchHotkey:         "Ctrl+F",
		MultiInputHotkey:     "Ctrl+Shift+M",
		MultiInputStopHotkey: "Ctrl+Alt+M",
	}}
	dto := AppSettingsToDTO(in)
	if dto.TerminalSearchHotkey != "Ctrl+F" || dto.MultiInputHotkey != "Ctrl+Shift+M" || dto.MultiInputStopHotkey != "Ctrl+Alt+M" {
		t.Fatalf("to DTO = %q / %q / %q, want Ctrl+F / Ctrl+Shift+M / Ctrl+Alt+M",
			dto.TerminalSearchHotkey, dto.MultiInputHotkey, dto.MultiInputStopHotkey)
	}
	if got := DTOToAppSettings(dto).TerminalTools; got != in.TerminalTools {
		t.Errorf("round trip = %+v, want %+v", got, in.TerminalTools)
	}
}
