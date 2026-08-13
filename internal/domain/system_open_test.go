package domain

import "testing"

// The chain this closes: anything holding the Wails bridge can put a file on disk (a download, a
// local copy) and then ask for it to be opened. The default association is the operating system
// deciding what to do, and for an executable it decides to run it - so "open this file" was a way
// to run arbitrary code as the user.
func TestExecutablesAreRefusedForSystemOpen(t *testing.T) {
	executables := []string{
		// Windows executables and installers.
		`C:\Users\me\Downloads\payload.exe`, "payload.com", "screensaver.scr",
		"old.pif", "installer.msi", "patch.msp", "panel.cpl",
		// Script hosts.
		"run.bat", "run.cmd", "run.ps1", "mod.psm1", "s.vbs", "s.vbe",
		"s.js", "s.jse", "s.wsf", "s.wsh", "page.hta", "console.msc",
		// Indirection: these carry a target rather than being one.
		"invoice.lnk", "link.url", "shell.scf", "config.settingcontent-ms",
		// Applies on open.
		"tweak.reg",
		// Cross-platform and Linux.
		"app.jar", "app.desktop", "install.sh", "x.bash", "x.zsh", "x.run", "tool.appimage",
	}
	for _, path := range executables {
		t.Run(path, func(t *testing.T) {
			if !IsExecutableForSystemOpen(path) {
				t.Errorf("IsExecutableForSystemOpen(%q) = false; the OS would run this, not show it", path)
			}
		})
	}
}

// The double extension is the oldest trick for making an executable look like a document, and only
// the final extension decides what runs.
func TestDoubleExtensionsAreJudgedByTheFinalOne(t *testing.T) {
	if !IsExecutableForSystemOpen("invoice.pdf.exe") {
		t.Error("invoice.pdf.exe was treated as a PDF")
	}
	if IsExecutableForSystemOpen("archive.exe.pdf") {
		t.Error("archive.exe.pdf was treated as an executable; the final extension is what opens")
	}
}

// Windows strips trailing dots and spaces, so `payload.exe.` and `payload.exe ` open the same file.
// Judging the name as written would let either through.
func TestTrailingDotsAndSpacesDoNotHideAnExecutable(t *testing.T) {
	for _, path := range []string{"payload.exe.", "payload.exe ", "payload.exe . ", "payload.bat.."} {
		t.Run(path, func(t *testing.T) {
			if !IsExecutableForSystemOpen(path) {
				t.Errorf("IsExecutableForSystemOpen(%q) = false; Windows opens this as payload.exe", path)
			}
		})
	}
}

// Case is not a signal. FAT and NTFS are case-insensitive and the shell does not care.
func TestExtensionMatchingIsCaseInsensitive(t *testing.T) {
	for _, path := range []string{"PAYLOAD.EXE", "Run.Bat", "s.Vbs", "app.JAR"} {
		t.Run(path, func(t *testing.T) {
			if !IsExecutableForSystemOpen(path) {
				t.Errorf("IsExecutableForSystemOpen(%q) = false", path)
			}
		})
	}
}

// The half that keeps the feature working. Opening a downloaded document in its default viewer is
// the entire point of the method, and a check that refused these would have removed a feature
// instead of a hole.
func TestOrdinaryDocumentsAreStillOpened(t *testing.T) {
	documents := []string{
		"report.pdf", "notes.txt", "photo.png", "photo.jpeg", "sheet.xlsx",
		"page.html", "data.json", "config.yaml", "archive.zip", "video.mp4",
		"README", "Makefile", ".gitignore", "notes", "a.b.c.txt",
		// Not executables despite the leading letters.
		"executable-notes.txt", "batch-report.csv", "command.log",
	}
	for _, path := range documents {
		t.Run(path, func(t *testing.T) {
			if IsExecutableForSystemOpen(path) {
				t.Errorf("IsExecutableForSystemOpen(%q) = true; an ordinary document was refused", path)
			}
		})
	}
}

func TestAPathWithNoExtensionIsNotExecutable(t *testing.T) {
	for _, path := range []string{"", " ", ".", "..", "dir/", `C:\dir\`} {
		t.Run(path, func(t *testing.T) {
			if IsExecutableForSystemOpen(path) {
				t.Errorf("IsExecutableForSystemOpen(%q) = true", path)
			}
		})
	}
}
