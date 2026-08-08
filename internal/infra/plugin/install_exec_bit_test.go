package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A plugin whose entry binary arrives without the execute bit cannot be started at all. The host
// reports it as `fork/exec …: permission denied` straight out of the OS, which names neither the
// plugin nor the install that produced it — and because every install path shares this copy, it
// takes down every plugin at once rather than one.
//
// Windows has no execute bit and its Stat reports 0666/0444 regardless, which is exactly why this
// went unnoticed: the whole install path looked correct on the platform it was developed on. CI
// runs these on ubuntu, so the assertion below is the one that would have caught it.
func TestCopyBundleKeepsTheEntryExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no execute bit on Windows; the mode this asserts is not represented there")
	}

	for _, tc := range []struct {
		name       string
		declared   string // manifest engine.entry
		onDisk     string // the file the bundle actually carries
		otherFiles []string
	}{
		{
			name:       "entry named exactly as the manifest declares it",
			declared:   "xqs-vnc",
			onDisk:     "xqs-vnc",
			otherFiles: []string{"plugin.json", "SHA256SUMS", "ui/index.html"},
		},
		{
			// One manifest is published for every platform and can only name the binary once, so a
			// Windows-built bundle carries the .exe while the manifest says the bare name.
			// Discovery accepts that pair; the copy has to as well.
			name:       "entry carrying the .exe alternate",
			declared:   "xqs-vnc",
			onDisk:     "xqs-vnc.exe",
			otherFiles: []string{"plugin.json"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := t.TempDir()
			dest := filepath.Join(t.TempDir(), "installed")

			write(t, filepath.Join(src, tc.onDisk), "#!/bin/sh\nexit 0\n")
			for _, f := range tc.otherFiles {
				write(t, filepath.Join(src, f), "x")
			}

			if err := CopyBundle(src, dest, tc.declared); err != nil {
				t.Fatalf("copy bundle: %v", err)
			}

			if perm := mode(t, filepath.Join(dest, tc.onDisk)); perm&0o100 == 0 {
				t.Errorf("entry %s installed as %04o, has no owner execute bit; the host cannot exec it",
					tc.onDisk, perm)
			}

			// Everything else stays 0600. An install that made the whole tree executable would pass
			// the assertion above while widening far more than it needed to.
			for _, f := range tc.otherFiles {
				if perm := mode(t, filepath.Join(dest, f)); perm != 0o600 {
					t.Errorf("%s installed as %04o, want 0600 — only the engine entry may be executable", f, perm)
				}
			}
		})
	}
}

// The repair path, which is what makes the fix reach plugins already installed by rc.3 and earlier.
// Without it the bug is only fixed for plugins installed after the update, and the ones already on
// disk stay dead with nothing on screen to suggest reinstalling would revive them.
func TestEnsureEntryExecutableRepairsAnInstalledPlugin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no execute bit on Windows; EnsureEntryExecutable is a documented no-op there")
	}

	dir := t.TempDir()
	entry := filepath.Join(dir, "xqs-vnc")
	write(t, entry, "#!/bin/sh\nexit 0\n") // write() lands 0600, exactly what a broken install left

	if perm := mode(t, entry); perm&0o100 != 0 {
		t.Fatalf("fixture is already executable at %04o; this test would prove nothing", perm)
	}
	if err := EnsureEntryExecutable(entry); err != nil {
		t.Fatalf("ensure executable: %v", err)
	}
	if perm := mode(t, entry); perm != 0o700 {
		t.Errorf("repaired entry is %04o, want 0700 — the owner-execute bit and nothing wider", perm)
	}

	// Idempotent, and it must not widen an already-correct file: the spawner calls this on every
	// plugin start.
	if err := EnsureEntryExecutable(entry); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if perm := mode(t, entry); perm != 0o700 {
		t.Errorf("a second call moved the mode to %04o", perm)
	}

	if err := EnsureEntryExecutable(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Error("a missing entry reported success; the spawner would then exec nothing and say so worse")
	}
}

// The naming half of the rule, asserted everywhere. The mode test above can only run where an
// execute bit exists, and Windows — the platform this bug survived on — is not one, so without this
// the rule would go unchecked on exactly the machine most likely to change it.
func TestExecutableEntryNamesCoversBothPlatformSpellings(t *testing.T) {
	for _, tc := range []struct {
		entry string
		want  []string
	}{
		{entry: "xqs-vnc", want: []string{"xqs-vnc", "xqs-vnc.exe"}},
		{entry: "xqs-vnc.exe", want: []string{"xqs-vnc.exe", "xqs-vnc"}},
		// engine.entry is bundle-relative and always slash-separated; lookups happen against
		// filepath.Rel output, which is not.
		{entry: "bin/xqs-vnc", want: []string{filepath.Join("bin", "xqs-vnc"), filepath.Join("bin", "xqs-vnc.exe")}},
	} {
		got := executableEntryNames(tc.entry)
		for _, want := range tc.want {
			if !got[want] {
				t.Errorf("entry %q: %q is not marked executable; a bundle carrying that spelling installs unrunnable",
					tc.entry, want)
			}
		}
		if len(got) != len(tc.want) {
			t.Errorf("entry %q marked %d names, want %d — nothing beyond the entry may become executable",
				tc.entry, len(got), len(tc.want))
		}
	}

	// A plugin.json or a ui/ asset must never match: the entry is the only file the host execs.
	for _, other := range []string{"plugin.json", "SHA256SUMS", filepath.Join("ui", "index.html")} {
		if executableEntryNames("xqs-vnc")[other] {
			t.Errorf("%q marked executable for entry xqs-vnc", other)
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}
