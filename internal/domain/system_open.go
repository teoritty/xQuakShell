package domain

import (
	"path/filepath"
	"strings"
)

// executableOpenExtensions are the file types the OS runs rather than displays when asked to open
// them with their default application.
//
// The list is not exhaustive and cannot be: Windows decides what is executable from PATHEXT and the
// registry, both of which a machine's own configuration changes. It is still worth having. Every
// entry here is a shape an attacker reaches for first, and removing them turns "ask the app to open
// this and it runs" into "find a file type this machine has been configured to run", which is a
// much smaller and much less portable trick.
//
// .lnk and .url earn their place separately: they are the ones that look like documents. A shortcut
// carries its own target and icon, so what the user sees in a listing says nothing about what
// opening it starts.
var executableOpenExtensions = map[string]struct{}{
	// Windows executables and installers.
	".exe": {}, ".com": {}, ".scr": {}, ".pif": {}, ".msi": {}, ".msp": {}, ".cpl": {},
	// Windows script hosts.
	".bat": {}, ".cmd": {}, ".ps1": {}, ".psm1": {}, ".vbs": {}, ".vbe": {}, ".js": {},
	".jse": {}, ".wsf": {}, ".wsh": {}, ".hta": {}, ".msc": {},
	// Indirection: these carry a target rather than being one.
	".lnk": {}, ".url": {}, ".scf": {}, ".settingcontent-ms": {},
	// Registry edits apply on open.
	".reg": {},
	// Cross-platform runtimes and the Linux equivalent of a shortcut.
	".jar": {}, ".desktop": {},
	// Unix scripts. xdg-open usually shows these in an editor, but "usually" is a property of the
	// desktop environment rather than a guarantee.
	".sh": {}, ".bash": {}, ".zsh": {}, ".run": {}, ".appimage": {},
}

// IsExecutableForSystemOpen reports whether handing this path to the operating system's default
// application would run it rather than show it.
//
// Only the final extension is consulted, which is the point: `invoice.pdf.exe` is an executable and
// the double extension is the oldest trick for making one look like a document.
func IsExecutableForSystemOpen(path string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimRight(path, " .")))
	_, executable := executableOpenExtensions[ext]
	return executable
}
