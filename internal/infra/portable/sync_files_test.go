package portable

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"xquakshell/internal/domain"
)

func newStore(t *testing.T) (*DataStore, string) {
	t.Helper()
	root := t.TempDir()
	return NewDataStore(root, "", nil), root
}

// symlinkOrSkip plants a link, or skips where the OS refuses unprivileged links (Windows without
// developer mode).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
}

func TestWriteFileAtomicCreatesDirectoriesAndReplaces(t *testing.T) {
	s, root := newStore(t)
	if err := s.WriteFileAtomic("plugins/p/state/a.json", []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFileAtomic("plugins/p/state/a.json", []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "plugins", "p", "state", "a.json"))
	if err != nil || string(got) != "two" {
		t.Fatalf("content = %q, %v; the second write replaces the first", got, err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "plugins", "p", "state"))
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries; no temp file is left behind", len(entries))
	}
}

func TestWriteFileAtomicRefusesEscape(t *testing.T) {
	s, _ := newStore(t)
	outside := filepath.Join(t.TempDir(), "x")
	for _, p := range []string{"../escape", "a/../../escape", outside} {
		if err := s.WriteFileAtomic(p, []byte("x"), 0o600); !errors.Is(err, domain.ErrPortablePathDenied) {
			t.Errorf("WriteFileAtomic(%q) = %v, want ErrPortablePathDenied", p, err)
		}
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("a write outside the root created a file")
	}
}

func TestWriteFileAtomicRefusesASymlinkedDirectory(t *testing.T) {
	s, root := newStore(t)
	elsewhere := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, elsewhere, filepath.Join(root, "plugins", "p"))
	if err := s.WriteFileAtomic("plugins/p/state.json", []byte("x"), 0o600); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Fatalf("write through a symlinked directory: %v, want ErrPortablePathDenied", err)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatal("a write through a symlinked directory landed outside the root")
	}
}

func TestWriteFileAtomicRefusesToReplaceASymlinkOrDirectory(t *testing.T) {
	s, root := newStore(t)
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFileAtomic("dir", []byte("x"), 0o600); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Fatalf("replacing a directory: %v, want ErrPortablePathDenied", err)
	}
	symlinkOrSkip(t, secret, filepath.Join(root, "link"))
	if err := s.WriteFileAtomic("link", []byte("x"), 0o600); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Fatalf("replacing a symlink: %v, want ErrPortablePathDenied", err)
	}
	if got, _ := os.ReadFile(secret); string(got) != "keep" {
		t.Fatal("a write through a symlink changed its target")
	}
}

func TestWriteFileAtomicRefusesAnAlternateDataStream(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("alternate data streams are an NTFS feature")
	}
	s, root := newStore(t)
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFileAtomic("state.json:hidden", []byte("x"), 0o600); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Fatalf("write to an alternate data stream: %v, want ErrPortablePathDenied", err)
	}
}

func TestWalkListsOnlyRegularFilesUnderTheDirectory(t *testing.T) {
	s, root := newStore(t)
	base := filepath.Join(root, "plugins", "p")
	for _, f := range []string{"a.json", "state/b.json", "tmp/c"} {
		full := filepath.Join(base, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("1234"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := true
	if err := os.Symlink(elsewhere, filepath.Join(base, "linkdir")); err != nil {
		linked = false
	}
	if linked {
		if err := os.Symlink(filepath.Join(elsewhere, "secret"), filepath.Join(base, "linkfile")); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	err := s.Walk("plugins/p", func(rel string, size int64) error {
		if size != 4 {
			t.Errorf("%s size = %d, want 4", rel, size)
		}
		got = append(got, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{"a.json", "state/b.json", "tmp/c"}; !slices.Equal(got, want) {
		t.Fatalf("walk = %v, want %v; symlinks are neither listed nor followed", got, want)
	}
	if err := s.Walk("plugins/none", func(string, int64) error { t.Fatal("called"); return nil }); err != nil {
		t.Fatalf("walking a directory that does not exist: %v, want nil", err)
	}
	if err := s.Walk("..", func(string, int64) error { return nil }); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Fatalf("walking outside the root: %v, want ErrPortablePathDenied", err)
	}
}

func TestReadRegularFileRefusesSymlinksAndOversize(t *testing.T) {
	s, root := newStore(t)
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ReadRegularFile("a", 5); err != nil || string(got) != "12345" {
		t.Fatalf("read at the limit = %q, %v", got, err)
	}
	if _, err := s.ReadRegularFile("a", 4); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Fatalf("read over the limit: %v, want ErrPortablePathDenied", err)
	}
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, secret, filepath.Join(root, "link"))
	if _, err := s.ReadRegularFile("link", 10); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Fatalf("read through a symlink: %v, want ErrPortablePathDenied", err)
	}
}
