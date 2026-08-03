package sshconfig

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

func TestParseResolvesIdentityFilePaths(t *testing.T) {
	home := fakeHome(t)
	keyPath := writeFile(t, filepath.Join(home, ".ssh"), "id_ed25519", "KEY")
	path := writeFile(t, t.TempDir(), "config", "Host web\n  IdentityFile ~/.ssh/id_ed25519\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := hostByAlias(t, result, "web").IdentityFiles
	if len(got) != 1 || got[0] != filepath.Clean(keyPath) {
		t.Errorf("IdentityFiles = %v, want [%s]", got, keyPath)
	}
}

func TestParseExpandsIdentityFileTokens(t *testing.T) {
	home := fakeHome(t)
	keyPath := writeFile(t, filepath.Join(home, "keys"), "web.example.com.key", "KEY")
	path := writeFile(t, t.TempDir(), "config", `
Host web
    HostName web.example.com
    IdentityFile %d/keys/%h.key
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := hostByAlias(t, result, "web").IdentityFiles
	if len(got) != 1 || got[0] != filepath.Clean(keyPath) {
		t.Errorf("IdentityFiles = %v, want [%s] — %%d and %%h must expand", got, keyPath)
	}
}

func TestParseResolvesRelativeIdentityFileAgainstHome(t *testing.T) {
	home := fakeHome(t)
	keyPath := writeFile(t, filepath.Join(home, ".ssh"), "relative_key", "KEY")
	path := writeFile(t, t.TempDir(), "config", "Host web\n  IdentityFile .ssh/relative_key\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := hostByAlias(t, result, "web").IdentityFiles
	if len(got) != 1 || got[0] != filepath.Clean(keyPath) {
		t.Errorf("IdentityFiles = %v, want [%s]", got, keyPath)
	}
}

func TestParseDropsMissingIdentityFile(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", "Host web\n  IdentityFile ~/.ssh/absent_key\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := hostByAlias(t, result, "web").IdentityFiles; len(got) != 0 {
		t.Errorf("IdentityFiles = %v, want none — a dangling reference must not be imported", got)
	}
	if !hasNotice(result, domain.SSHConfigNoticeIdentityFileMissing, "web") {
		t.Errorf("a missing key must be reported; notices = %+v", result.Notices)
	}
}

func TestParseAccumulatesIdentityFilesInOrder(t *testing.T) {
	home := fakeHome(t)
	writeFile(t, filepath.Join(home, ".ssh"), "first", "KEY")
	writeFile(t, filepath.Join(home, ".ssh"), "second", "KEY")
	path := writeFile(t, t.TempDir(), "config", `
Host web
    IdentityFile ~/.ssh/first

Host *
    IdentityFile ~/.ssh/second
    IdentityFile ~/.ssh/first
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := hostByAlias(t, result, "web").IdentityFiles
	if len(got) != 2 {
		t.Fatalf("IdentityFiles = %v, want 2 entries — IdentityFile accumulates but does not repeat", got)
	}
	if filepath.Base(got[0]) != "first" || filepath.Base(got[1]) != "second" {
		t.Errorf("IdentityFiles = %v, want [first second] in declaration order", got)
	}
}

func TestParseIgnoresIdentityFileNone(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", "Host web\n  IdentityFile none\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := hostByAlias(t, result, "web").IdentityFiles; len(got) != 0 {
		t.Errorf("IdentityFiles = %v, want none", got)
	}
	if hasNotice(result, domain.SSHConfigNoticeIdentityFileMissing, "web") {
		t.Error(`"IdentityFile none" is deliberate, not a missing file`)
	}
}

func TestReadKeyFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "id_key", "PRIVATE KEY BYTES")

	got, err := ReadKeyFile(keyPath)
	if err != nil {
		t.Fatalf("ReadKeyFile: %v", err)
	}
	if string(got) != "PRIVATE KEY BYTES" {
		t.Errorf("got %q, want the file contents", got)
	}
}

func TestReadKeyFileRejects(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name string
		path string
		want error
	}{
		{name: "missing", path: filepath.Join(dir, "absent"), want: domain.ErrSSHConfigNotFound},
		{name: "directory", path: dir, want: domain.ErrSSHConfigUnreadable},
		{
			name: "oversize",
			path: writeFile(t, dir, "huge", strings.Repeat("x", maxKeyFileSize+1)),
			want: domain.ErrSSHConfigTooLarge,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadKeyFile(tc.path)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), dir) {
				t.Errorf("error text leaks an absolute path: %v", err)
			}
		})
	}
}

func TestDefaultPathReturnsEmptyWhenAbsent(t *testing.T) {
	fakeHome(t)

	got, err := DefaultPath()

	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty — a missing config is an ordinary state", got)
	}
}

func TestDefaultPathFindsUserConfig(t *testing.T) {
	home := fakeHome(t)
	want := writeFile(t, filepath.Join(home, ".ssh"), "config", "Host web\n")

	got, err := DefaultPath()

	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
