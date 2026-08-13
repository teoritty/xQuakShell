package sftp

import (
	"runtime"
	"testing"
)

// A name carrying U+202E renders `gnp.<RLO>exe` as `exe.png`: the extension the user reads is not
// the extension the file has. These characters have no purpose in a filename beyond that, so they
// are refused on every platform — the deception is in the rendering, and every platform renders.
func TestBidiOverridesAreRefusedEverywhere(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{"right-to-left override", "gnp.\u202Eexe"},
		{"left-to-right override", "report\u202Dexe"},
		{"right-to-left embedding", "\u202Binvoice.exe"},
		{"first-strong isolate", "photo\u2068.exe"},
		{"pop directional isolate", "photo\u2069.exe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if safeEntryName(tt.entry) {
				t.Errorf("safeEntryName(%q) = true; the name reorders its own display", tt.entry)
			}
		})
	}
}

// Ordinary non-Latin filenames are not bidi overrides. Arabic and Hebrew are right-to-left by their
// own character properties and need no override to render correctly — refusing them would hide a
// user's own files from them, which is a worse bug than the one being prevented.
func TestOrdinaryRightToLeftNamesAreAccepted(t *testing.T) {
	for _, entry := range []string{"تقرير.txt", "דוח.pdf", "отчёт.txt", "报告.txt", "café ☕.md"} {
		t.Run(entry, func(t *testing.T) {
			if !safeEntryName(entry) {
				t.Errorf("safeEntryName(%q) = false; an ordinary name was refused", entry)
			}
		})
	}
}

// isWindowsUnsafeName rather than safeEntryName, so the rule is exercised on any host. A Linux CI
// runner asserting Windows behaviour is the point: the bug happens on the user's machine.
func TestWindowsDeviceNamesAreRefused(t *testing.T) {
	tests := []string{
		"CON", "con", "Con",
		"PRN", "AUX", "NUL",
		"COM1", "com9", "LPT1", "lpt9",
		// The device wins over any extension: CON.txt is still the console.
		"CON.txt", "nul.log", "COM1.tar.gz",
		// And over trailing space before the extension, which Windows also strips.
		"CON .txt",
	}
	for _, entry := range tests {
		t.Run(entry, func(t *testing.T) {
			if !isWindowsUnsafeName(entry) {
				t.Errorf("isWindowsUnsafeName(%q) = false; writing this opens a device, and the "+
					"transfer reports success having stored nothing", entry)
			}
		})
	}
}

// Windows strips a trailing dot or space, so `report.txt.` and `report.txt` are the same file. A
// server listing both hands the user a name that overwrites a different one than it reads as.
func TestWindowsTrailingDotOrSpaceIsRefused(t *testing.T) {
	for _, entry := range []string{"report.txt.", "report.txt ", "archive.", "notes ", "a."} {
		t.Run(entry, func(t *testing.T) {
			if !isWindowsUnsafeName(entry) {
				t.Errorf("isWindowsUnsafeName(%q) = false; Windows strips the trailing character "+
					"and the file collides with a different name", entry)
			}
		})
	}
}

// The rule must not swallow ordinary names, or every download of a normal directory starts losing
// files. CONFIG and console.log begin with the device names without being them.
func TestOrdinaryNamesPassTheWindowsRule(t *testing.T) {
	ordinary := []string{
		"CONFIG", "console.log", "connection.json", "aux-data.txt",
		"com.example.plugin", "lpt-report.csv", "nullable.go",
		"report.txt", ".gitignore", "a", "COM0", "LPT0", "COM10",
	}
	for _, entry := range ordinary {
		t.Run(entry, func(t *testing.T) {
			if isWindowsUnsafeName(entry) {
				t.Errorf("isWindowsUnsafeName(%q) = true; an ordinary filename was refused", entry)
			}
		})
	}
}

// The platform gate is the reason the Windows rules can be strict at all: these are legitimate
// filenames on Linux and macOS, and hiding a real file from the user looking at their own server
// would be the worse bug.
func TestTheWindowsRuleIsPlatformScoped(t *testing.T) {
	// unsafeOnWindows consults runtime.GOOS; isWindowsUnsafeName never does. Asserting they agree
	// only on Windows is what pins the gate in place.
	const deviceName = "CON"
	if !isWindowsUnsafeName(deviceName) {
		t.Fatal("the rule itself stopped recognising a device name")
	}
	if unsafeOnWindows(deviceName) != (runtimeIsWindows()) {
		t.Errorf("unsafeOnWindows(%q) = %v on this platform; the gate is not consulting GOOS",
			deviceName, unsafeOnWindows(deviceName))
	}
}

func runtimeIsWindows() bool { return runtime.GOOS == "windows" }
