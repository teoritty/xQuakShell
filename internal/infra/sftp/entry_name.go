package sftp

import (
	"runtime"
	"strings"
)

// safeEntryName validates a filename taken from a remote directory listing.
// The SFTP server controls these bytes completely, so a name is accepted only
// if it is a single, inert path segment. Rejecting here — at the adapter
// boundary — is what lets every consumer of RemoteFS treat RemoteNode.Name as
// a trusted component. pkg/sftp already applies path.Base, but path.Base only
// understands forward slashes: a name like `..\..\evil.exe` reaches us intact
// and escapes once filepath.Join cleans it on Windows.
func safeEntryName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	// Windows resolves ADS and drive-relative syntax inside a single segment.
	if strings.ContainsRune(name, ':') {
		return false
	}
	if strings.ContainsRune(name, 0) {
		return false
	}
	// The three above answer "is this one path segment". These two answer a different question:
	// does the name mean, to the machine that will write it or to the human reading it, what it
	// says. See entry_name.go.
	if hasBidiOverride(name) {
		return false
	}
	if unsafeOnWindows(name) {
		return false
	}
	return true
}

// bidiOverrides are the Unicode formatting characters whose only function is to reorder how the
// text around them is displayed.
//
// A name carrying U+202E renders `gnp.<RLO>exe` as `exe.png` — the extension the user reads is not
// the extension the file has. The characters have no meaning in a filename beyond that trick, so
// unlike the Windows rules below this is refused on every platform: the deception is in the
// rendering, and every platform renders.
// bidiOverrides are the Unicode formatting characters whose only function is to reorder how the
// text around them is displayed.
//
// Written as escapes, never as the characters themselves: a source file carrying them renders as
// misleadingly as the filenames it is here to reject, which is the Trojan Source problem gosec's
// G116 exists to catch. The escape is also the only spelling a reader can actually see.
var bidiOverrides = []rune{
	'\u202a', // LEFT-TO-RIGHT EMBEDDING
	'\u202b', // RIGHT-TO-LEFT EMBEDDING
	'\u202c', // POP DIRECTIONAL FORMATTING
	'\u202d', // LEFT-TO-RIGHT OVERRIDE
	'\u202e', // RIGHT-TO-LEFT OVERRIDE
	'\u2066', // LEFT-TO-RIGHT ISOLATE
	'\u2067', // RIGHT-TO-LEFT ISOLATE
	'\u2068', // FIRST STRONG ISOLATE
	'\u2069', // POP DIRECTIONAL ISOLATE
}

// windowsReservedNames are the DOS device names Windows still resolves ahead of any file, in every
// directory, with or without an extension: CON.txt is the console, not a file.
var windowsReservedNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

// hasBidiOverride reports whether a name carries a character that reorders its own display.
func hasBidiOverride(name string) bool {
	return strings.ContainsAny(name, string(bidiOverrides))
}

// unsafeOnWindows reports whether a name means something different to the Windows filesystem than
// it says.
//
// Two shapes, both of which turn "the file the user was shown" into "some other file":
//
//   - a DOS device name. Writing a download to CON writes to the console, and the transfer reports
//     success having stored nothing.
//   - a trailing dot or space. Windows silently strips them, so `report.txt.` and `report.txt` are
//     the same file. A server that lists both hands the user a name that overwrites a different one
//     than it reads as.
//
// This is deliberately conditional on the running platform rather than applied everywhere. Both
// shapes are ordinary, legitimate filenames on Linux and macOS, and hiding a real file from the
// user who is looking at their own server would be a worse bug than the one being prevented.
func unsafeOnWindows(name string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return isWindowsUnsafeName(name)
}

// isWindowsUnsafeName is unsafeOnWindows without the platform check, so the rule itself can be
// exercised from a test on any host. A Linux CI runner asserting Windows behaviour is the point:
// the bug it guards happens on the user's machine, not on the runner's.
func isWindowsUnsafeName(name string) bool {
	if name == "" {
		return false
	}
	if last := name[len(name)-1]; last == '.' || last == ' ' {
		return true
	}
	stem := name
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	_, reserved := windowsReservedNames[strings.ToLower(strings.TrimRight(stem, " "))]
	return reserved
}
