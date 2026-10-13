package portable

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"xquakshell/internal/domain"
)

// junction plants an NTFS junction, which unlike a symlink needs no privilege to create: the
// redirection an unprivileged process on Windows can actually plant.
func junction(t *testing.T, target, link string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("mklink /J unavailable: %v: %s", err, out)
	}
}

func TestTheZoneRefusesAJunctionedDirectory(t *testing.T) {
	s, root := newStore(t)
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, "secret"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}
	junction(t, elsewhere, filepath.Join(root, "plugins", "p"))

	if err := s.WriteFileAtomic("plugins/p/secret", []byte("x"), 0o600); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Errorf("write through a junction: %v, want ErrPortablePathDenied", err)
	}
	if got, _ := os.ReadFile(filepath.Join(elsewhere, "secret")); string(got) != "keep" {
		t.Error("a write through a junction changed a file outside the root")
	}
	if err := s.RemoveFile("plugins/p/secret"); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Errorf("remove through a junction: %v, want ErrPortablePathDenied", err)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "secret")); err != nil {
		t.Errorf("a remove through a junction deleted a file outside the root: %v", err)
	}
	if _, err := s.ReadRegularFile("plugins/p/secret", 10); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Errorf("read through a junction: %v, want ErrPortablePathDenied", err)
	}
	if err := s.Walk("plugins/p", func(rel string, _ int64) error { t.Errorf("walked %s through a junction", rel); return nil }); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Errorf("walking a junction: %v, want ErrPortablePathDenied", err)
	}
	if err := os.MkdirAll(filepath.Join(elsewhere, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Walk("plugins/p/sub", func(rel string, _ int64) error { return nil }); !errors.Is(err, domain.ErrPortablePathDenied) {
		t.Errorf("walking below a junction: %v, want ErrPortablePathDenied", err)
	}
	var listed []string
	if err := s.Walk("plugins", func(rel string, _ int64) error { listed = append(listed, rel); return nil }); err != nil || len(listed) != 0 {
		t.Errorf("walking past a junction listed %v, %v; it is neither listed nor followed", listed, err)
	}
}
