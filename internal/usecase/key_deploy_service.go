package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xquakshell/internal/domain"
)

// KeyDeploySessionPort resolves the remote filesystem of an already-authenticated session.
type KeyDeploySessionPort interface {
	GetRemoteFS(sessionID string) (domain.RemoteFS, error)
}

// KeyDeployService appends a public key to a server's authorized_keys.
//
// It works over a session the user already opened, so no second authentication path exists and no
// password is asked for twice. It also means the operation is impossible against a host the user
// has not already reached, which is the correct restriction: publishing a key is exactly the
// action that should not be available to something that cannot log in yet.
type KeyDeployService struct {
	sessions KeyDeploySessionPort
	keys     *KeyManagerService
	audit    KeyManagerAudit
}

// NewKeyDeployService wires the deploy service. A nil audit recorder is accepted so a build whose
// audit database failed to open can still publish a key, the same trade-off the key manager makes.
func NewKeyDeployService(sessions KeyDeploySessionPort, keys *KeyManagerService, audit KeyManagerAudit) *KeyDeployService {
	return &KeyDeployService{sessions: sessions, keys: keys, audit: audit}
}

// Filesystem layout sshd insists on. Anything more permissive and sshd ignores the file entirely,
// with a message that only appears in the server's log — from the client it looks like the key
// simply does not work, which is the most confusing failure this feature can produce.
const (
	sshDirMode          = 0o700
	authorizedKeysMode  = 0o600
	authorizedKeysPath  = ".ssh/authorized_keys"
	sshDirPath          = ".ssh"
	maxAuthorizedKeysKB = 512 * 1024
)

// DeployResult reports what happened, so the UI can distinguish a key that was added from one
// that was already there — both are success, and telling the user which is the difference between
// a confirmation and a lie.
type DeployResult struct {
	Added          bool   `json:"added"`
	AlreadyPresent bool   `json:"alreadyPresent"`
	Path           string `json:"path"`
}

// Deploy appends the identity's public key to the session user's authorized_keys.
func (s *KeyDeployService) Deploy(ctx context.Context, sessionID, identityID string) (*DeployResult, error) {
	publicKey, err := s.keys.PublicKey(ctx, identityID)
	if err != nil {
		return nil, err
	}
	fs, err := s.sessions.GetRemoteFS(sessionID)
	if err != nil {
		return nil, err
	}

	home, err := fs.GetWorkingDirectory(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	sshDir := joinRemote(home, sshDirPath)
	keysPath := joinRemote(home, authorizedKeysPath)

	existing, err := s.readExisting(ctx, fs, keysPath)
	if err != nil {
		return nil, err
	}
	if containsAuthorizedKey(existing, publicKey) {
		return &DeployResult{AlreadyPresent: true, Path: keysPath}, nil
	}

	target := deployTarget{fs: fs, sshDir: sshDir, keysPath: keysPath}
	if err := target.write(ctx, existing, publicKey); err != nil {
		return nil, err
	}
	s.record(ctx, identityID)
	return &DeployResult{Added: true, Path: keysPath}, nil
}

// readExisting returns the current authorized_keys, treating a missing file as empty. Every other
// failure is fatal: rewriting a file whose contents could not be read would delete whatever keys
// were in it, which would lock the user out of their own server.
func (s *KeyDeployService) readExisting(ctx context.Context, fs domain.RemoteFS, keysPath string) ([]byte, error) {
	existing, err := fs.ReadSmallFile(ctx, keysPath, maxAuthorizedKeysKB)
	if err == nil {
		return existing, nil
	}
	if errors.Is(err, domain.ErrRemoteFileNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("read authorized_keys: %w", err)
}

// deployTarget is where a key is being published: the remote filesystem and the two paths on it.
// Grouping them keeps write within the parameter budget and stops the directory and the file from
// drifting apart at a call site.
type deployTarget struct {
	fs       domain.RemoteFS
	sshDir   string
	keysPath string
}

// write creates ~/.ssh if needed and appends the key, preserving what was already there.
func (t deployTarget) write(ctx context.Context, existing []byte, publicKey string) error {
	if err := t.fs.Mkdir(ctx, t.sshDir); err != nil {
		return fmt.Errorf("create %s: %w", t.sshDir, err)
	}
	if err := t.fs.Chmod(ctx, t.sshDir, sshDirMode); err != nil {
		return fmt.Errorf("chmod %s: %w", t.sshDir, err)
	}

	updated := appendAuthorizedKey(existing, publicKey)
	if err := t.fs.WriteSmallFile(ctx, t.keysPath, updated, authorizedKeysMode); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	return nil
}

func (s *KeyDeployService) record(ctx context.Context, identityID string) {
	if s.audit == nil {
		return
	}
	s.audit.RecordKeyEvent(ctx, KeyEventDeployed, identityID, "")
}

// appendAuthorizedKey adds a line, guaranteeing exactly one newline before it and one after.
//
// A file whose last line has no trailing newline is the case that matters: appending straight
// onto it would splice the new key onto the end of the previous one and silently destroy both.
func appendAuthorizedKey(existing []byte, publicKey string) []byte {
	var out strings.Builder
	body := strings.TrimRight(string(existing), "\n")
	if body != "" {
		out.WriteString(body)
		out.WriteString("\n")
	}
	out.WriteString(strings.TrimSpace(publicKey))
	out.WriteString("\n")
	return []byte(out.String())
}

// containsAuthorizedKey reports whether the key is already authorised.
//
// It compares parsed keys rather than text. The same key deployed twice normally arrives with a
// different trailing comment, and a line may carry an options prefix, so a string match would
// append a duplicate on every deploy. Parsing is also the only way to be right about the shapes
// authorized_keys allows — options containing quoted spaces, certificate types, sk- keys — which
// a hand-rolled field split gets wrong in ways that show up as a duplicated key, not an error.
//
// A line that will not parse is skipped rather than treated as a match: an unreadable line is not
// evidence that this key is present, and the file is left byte-for-byte intact regardless.
func containsAuthorizedKey(existing []byte, publicKey string) bool {
	want, err := domain.ParseAuthorizedSSHKey(publicKey)
	if err != nil {
		return false
	}
	wanted := string(want.Marshal())
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		got, err := domain.ParseAuthorizedSSHKey(line)
		if err != nil {
			continue
		}
		if string(got.Marshal()) == wanted {
			return true
		}
	}
	return false
}

// joinRemote joins remote path segments with forward slashes. filepath.Join would use the host
// separator, which on Windows produces a backslash the remote server reads as part of the name.
func joinRemote(base, rest string) string {
	return strings.TrimRight(base, "/") + "/" + rest
}
