package main

import "xquakshell/internal/presentation/wails"

// Wails facade for local terminals (ADR-010): one-line delegates, no logic.

// OpenLocalTerminal starts a shell on this machine. It takes nothing, deliberately - see the
// handler's doc comment for why that must not change.
func (a *App) OpenLocalTerminal() (wails.LocalTerminalDTO, error) {
	return a.api.OpenLocalTerminal()
}

// CloseLocalTerminal ends a shell because its tab was closed.
func (a *App) CloseLocalTerminal(id string) error {
	return a.api.CloseLocalTerminal(id)
}

// SendLocalTerminalInput forwards keystrokes to a shell.
func (a *App) SendLocalTerminalInput(id string, dataBase64 string) error {
	return a.api.SendLocalTerminalInput(id, dataBase64)
}

// ResizeLocalTerminal reports a new character grid for a shell.
func (a *App) ResizeLocalTerminal(id string, cols int, rows int) error {
	return a.api.ResizeLocalTerminal(id, cols, rows)
}

// ListLocalShells reports the shells available for the settings picker.
func (a *App) ListLocalShells() ([]wails.ShellOptionDTO, error) {
	return a.api.ListLocalShells()
}
