package usecase

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

// deployFS is a remote filesystem holding one authorized_keys file, recording the modes it was
// asked for so the test can prove sshd would accept the result.
type deployFS struct {
	files    map[string][]byte
	modes    map[string]os.FileMode
	mkdirs   []string
	home     string
	readErr  error
	writeErr error
}

func newDeployFS() *deployFS {
	return &deployFS{files: map[string][]byte{}, modes: map[string]os.FileMode{}, home: "/home/user"}
}

func (f *deployFS) GetWorkingDirectory(context.Context) (string, error) { return f.home, nil }
func (f *deployFS) ReadSmallFile(_ context.Context, path string, _ int64) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	data, ok := f.files[path]
	if !ok {
		return nil, domain.ErrRemoteFileNotFound
	}
	return data, nil
}
func (f *deployFS) WriteSmallFile(_ context.Context, path string, data []byte, mode os.FileMode) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.files[path] = append([]byte(nil), data...)
	f.modes[path] = mode
	return nil
}
func (f *deployFS) Mkdir(_ context.Context, path string) error {
	f.mkdirs = append(f.mkdirs, path)
	return nil
}
func (f *deployFS) Chmod(_ context.Context, path string, mode os.FileMode) error {
	f.modes[path] = mode
	return nil
}

func (f *deployFS) List(context.Context, string) ([]domain.RemoteNode, error) { return nil, nil }
func (f *deployFS) Upload(context.Context, string, string, domain.ProgressFunc) error {
	return nil
}
func (f *deployFS) Download(context.Context, string, string, domain.ProgressFunc) error {
	return nil
}
func (f *deployFS) UploadRecursive(context.Context, string, string, domain.ProgressFunc) error {
	return nil
}
func (f *deployFS) DownloadRecursive(context.Context, string, string, domain.ProgressFunc) error {
	return nil
}
func (f *deployFS) CreateFile(context.Context, string) error        { return nil }
func (f *deployFS) Remove(context.Context, string) error            { return nil }
func (f *deployFS) RemoveAll(context.Context, string, func()) error { return nil }
func (f *deployFS) Rename(context.Context, string, string) error    { return nil }
func (f *deployFS) Chown(context.Context, string, int, int) error   { return nil }
func (f *deployFS) Close() error                                    { return nil }
func (f *deployFS) CountTree(context.Context, string, domain.ApplyTarget, func()) (int64, error) {
	return 0, nil
}
func (f *deployFS) ChmodRecursive(context.Context, string, os.FileMode, domain.ApplyTarget, func()) error {
	return nil
}
func (f *deployFS) ChownRecursive(context.Context, string, int, int, domain.ApplyTarget, func()) error {
	return nil
}

type deploySessions struct {
	fs  domain.RemoteFS
	err error
}

func (s deploySessions) RemoteFSForSession(string) (domain.RemoteFS, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.fs, nil
}

const keysPath = "/home/user/.ssh/authorized_keys"

// deployFixture builds a key manager holding one key, plus a deploy service over fs.
func deployFixture(t *testing.T, fs *deployFS) (*KeyDeployService, string, string) {
	t.Helper()
	svc, _, _ := newKeyManager(t, nil)
	identity, err := svc.Generate(context.Background(), ed25519Spec("laptop"), "", KeyOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return NewKeyDeployService(deploySessions{fs: fs}, svc, nil), identity.ID, identity.PublicKey
}

func TestDeployCreatesAuthorizedKeysWithTheModesSSHDInsistsOn(t *testing.T) {
	fs := newDeployFS()
	deploy, id, publicKey := deployFixture(t, fs)

	result, err := deploy.Deploy(context.Background(), "s1", id)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if !result.Added || result.AlreadyPresent {
		t.Errorf("result = %+v, want Added", result)
	}
	if got := strings.TrimSpace(string(fs.files[keysPath])); got != publicKey {
		t.Errorf("authorized_keys = %q, want %q", got, publicKey)
	}
	if fs.modes["/home/user/.ssh"] != 0o700 {
		t.Errorf(".ssh mode = %v, want 0700; sshd ignores the file outright above that and the failure is invisible from the client", fs.modes["/home/user/.ssh"])
	}
	if fs.modes[keysPath] != 0o600 {
		t.Errorf("authorized_keys mode = %v, want 0600", fs.modes[keysPath])
	}
}

func TestDeployKeepsTheKeysAlreadyInTheFile(t *testing.T) {
	fs := newDeployFS()
	existing := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKeyThatIsAlreadyThere colleague@host\n"
	fs.files[keysPath] = []byte(existing)
	deploy, id, publicKey := deployFixture(t, fs)

	if _, err := deploy.Deploy(context.Background(), "s1", id); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	got := string(fs.files[keysPath])
	if !strings.Contains(got, existing) {
		t.Fatalf("authorized_keys = %q; the key that was already there is gone, which locks whoever owned it out of their own server", got)
	}
	if !strings.Contains(got, publicKey) {
		t.Error("the new key was not added")
	}
}

func TestDeployDoesNotSpliceOntoAFileWithNoTrailingNewline(t *testing.T) {
	fs := newDeployFS()
	fs.files[keysPath] = []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKeyThatIsAlreadyThere colleague@host")
	deploy, id, publicKey := deployFixture(t, fs)

	if _, err := deploy.Deploy(context.Background(), "s1", id); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(fs.files[keysPath]), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %q, want 2; appending onto an unterminated last line splices the two keys together and destroys both", lines)
	}
	if lines[1] != publicKey {
		t.Errorf("second line = %q, want the new key", lines[1])
	}
}

func TestDeployTwiceDoesNotDuplicateTheKey(t *testing.T) {
	fs := newDeployFS()
	deploy, id, _ := deployFixture(t, fs)
	ctx := context.Background()

	if _, err := deploy.Deploy(ctx, "s1", id); err != nil {
		t.Fatalf("first deploy: %v", err)
	}
	first := string(fs.files[keysPath])

	result, err := deploy.Deploy(ctx, "s1", id)
	if err != nil {
		t.Fatalf("second deploy: %v", err)
	}
	if !result.AlreadyPresent || result.Added {
		t.Errorf("result = %+v, want AlreadyPresent; reporting a fresh add for a key that was already there tells the user something untrue", result)
	}
	if string(fs.files[keysPath]) != first {
		t.Errorf("authorized_keys changed on the second deploy: %q", fs.files[keysPath])
	}
}

// The same key normally arrives with a different comment on a second machine, and may be
// authorised under an options prefix. Either would defeat a textual comparison.
func TestDeployRecognisesTheSameKeyUnderADifferentCommentOrOptions(t *testing.T) {
	for _, tc := range []struct{ name, decorate string }{
		{"different comment", "%s someone-else@elsewhere"},
		{"options prefix", `no-agent-forwarding,from="10.0.0.0/8" %s`},
	} {
		fs := newDeployFS()
		deploy, id, publicKey := deployFixture(t, fs)
		fields := strings.Fields(publicKey)
		bare := fields[0] + " " + fields[1]
		fs.files[keysPath] = []byte(strings.Replace(tc.decorate, "%s", bare, 1) + "\n")
		before := string(fs.files[keysPath])

		result, err := deploy.Deploy(context.Background(), "s1", id)
		if err != nil {
			t.Fatalf("%s: deploy: %v", tc.name, err)
		}
		if !result.AlreadyPresent {
			t.Errorf("%s: result = %+v, want AlreadyPresent", tc.name, result)
		}
		if string(fs.files[keysPath]) != before {
			t.Errorf("%s: the file was rewritten: %q", tc.name, fs.files[keysPath])
		}
	}
}

// A read that fails for any reason other than "no such file" must abort. Rewriting a file whose
// contents could not be read would delete every key in it.
func TestDeployRefusesWhenAuthorizedKeysCannotBeRead(t *testing.T) {
	fs := newDeployFS()
	fs.readErr = errors.New("permission denied")
	deploy, id, _ := deployFixture(t, fs)

	if _, err := deploy.Deploy(context.Background(), "s1", id); err == nil {
		t.Fatal("deploy succeeded despite an unreadable authorized_keys; the write would have destroyed whatever was in it")
	}
	if _, written := fs.files[keysPath]; written {
		t.Error("the file was written anyway")
	}
}

func TestDeployRefusesAKeyWhoseMigrationNeverFinished(t *testing.T) {
	fs := newDeployFS()
	svc, repo, _ := newKeyManager(t, nil)
	ctx := context.Background()
	if err := repo.Save(ctx, domain.SSHIdentity{ID: "pending", Comment: "old", MigrationPending: true}, domain.IdentityBlob{Legacy: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	deploy := NewKeyDeployService(deploySessions{fs: fs}, svc, nil)

	if _, err := deploy.Deploy(ctx, "s1", "pending"); !errors.Is(err, domain.ErrMigrationPending) {
		t.Fatalf("deploy of an unmigrated key = %v, want ErrMigrationPending; it has no derived public key to publish", err)
	}
}

func TestDeployReportsAnUnknownSession(t *testing.T) {
	svc, _, _ := newKeyManager(t, nil)
	identity, err := svc.Generate(context.Background(), ed25519Spec("laptop"), "", KeyOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	deploy := NewKeyDeployService(deploySessions{err: domain.ErrSessionNotFound}, svc, nil)

	if _, err := deploy.Deploy(context.Background(), "gone", identity.ID); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Errorf("deploy over a dead session = %v, want ErrSessionNotFound", err)
	}
}
