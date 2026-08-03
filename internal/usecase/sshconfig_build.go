package usecase

import (
	"context"
	"path"
	"strings"

	"xquakshell/internal/domain"
)

// buildConnection turns one parsed host into a vault connection, importing the
// private keys it references when the caller asked for that.
func (s *SSHConfigImportService) buildConnection(
	ctx context.Context,
	host domain.SSHConfigHost,
	folderID string,
	order int,
	keys *keyImporter,
) domain.Connection {
	conn := domain.Connection{
		FolderID: folderID,
		Name:     host.Alias,
		Host:     host.HostName,
		Port:     effectivePort(host.Port),
		Order:    order,
		Protocol: domain.ProtocolSSH,
	}
	if host.User != "" {
		identityIDs := keys.importAll(ctx, host.IdentityFiles)
		user := domain.ConnectionUser{
			ID:       "u-" + randomHex(4),
			Username: host.User,
			Auth:     domain.AuthMethodKey,
			KeyAuth:  keyAuthFor(identityIDs),
		}
		conn.Users = []domain.ConnectionUser{user}
		conn.DefaultUserID = user.ID
	}
	conn.JumpChain = domain.JumpChainConfig{Hops: s.buildHops(ctx, host.JumpHops, keys)}
	return conn
}

// buildHops converts a resolved ProxyJump chain into vault jump hops.
func (s *SSHConfigImportService) buildHops(ctx context.Context, hops []domain.SSHConfigHop, keys *keyImporter) []domain.JumpHop {
	if len(hops) == 0 {
		return nil
	}
	out := make([]domain.JumpHop, 0, len(hops))
	for _, hop := range hops {
		identityIDs := keys.importAll(ctx, hop.IdentityFiles)
		out = append(out, domain.JumpHop{
			ID:       "h-" + randomHex(4),
			Host:     hop.HostName,
			Port:     effectivePort(hop.Port),
			Username: hop.User,
			Auth:     domain.AuthMethodKey,
			KeyAuth:  keyAuthFor(identityIDs),
		})
	}
	return out
}

// keyAuthFor returns a key auth config, or nil when no identity was imported.
//
// A nil KeyAuth leaves the connection incomplete on purpose: the import fills
// in what the config stated and nothing more, so the user is prompted to
// choose an identity rather than silently connecting with the wrong one.
func keyAuthFor(identityIDs []string) *domain.KeyAuthConfig {
	if len(identityIDs) == 0 {
		return nil
	}
	return &domain.KeyAuthConfig{IdentityIDs: identityIDs}
}

// keyImporter stores referenced private keys in the vault, at most once each.
//
// Deduplication is by absolute path within a single import run: a config where
// forty hosts share ~/.ssh/id_ed25519 should produce one vault identity, not
// forty copies of the same key material.
type keyImporter struct {
	identRepo domain.IdentityRepository
	reader    domain.SSHConfigImporter
	enabled   bool
	// byPath maps an already-imported key file to its vault identity ID.
	byPath map[string]string
	// failed records paths that could not be read or stored, so a key that
	// fails for several hosts is reported once.
	failed map[string]bool
}

func newKeyImporter(identRepo domain.IdentityRepository, reader domain.SSHConfigImporter, enabled bool) *keyImporter {
	return &keyImporter{
		identRepo: identRepo,
		reader:    reader,
		enabled:   enabled && identRepo != nil && reader != nil,
		byPath:    map[string]string{},
		failed:    map[string]bool{},
	}
}

// importAll stores every key file in paths and returns the resulting identity
// IDs, in order. Paths that fail are skipped, not fatal.
func (k *keyImporter) importAll(ctx context.Context, paths []string) []string {
	if !k.enabled || len(paths) == 0 {
		return nil
	}
	var ids []string
	for _, p := range paths {
		if id, ok := k.importOne(ctx, p); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// importOne stores a single key file, reusing an earlier import of the same
// path and refusing to retry one that already failed.
func (k *keyImporter) importOne(ctx context.Context, keyPath string) (string, bool) {
	if id, done := k.byPath[keyPath]; done {
		return id, true
	}
	if k.failed[keyPath] {
		return "", false
	}
	// The path comes from the parse result the reader itself produced, which
	// is what keeps ReadKeyFile from being reachable with a caller-chosen path.
	data, err := k.reader.ReadKeyFile(keyPath)
	if err != nil {
		k.failed[keyPath] = true
		return "", false
	}
	identity, err := k.identRepo.Import(ctx, data, identityComment(keyPath))
	if err != nil {
		k.failed[keyPath] = true
		return "", false
	}
	k.byPath[keyPath] = identity.ID
	return identity.ID, true
}

// counts reports how many distinct keys were stored and how many failed.
func (k *keyImporter) counts() (int, int) {
	if k == nil {
		return 0, 0
	}
	return len(k.byPath), len(k.failed)
}

// identityComment labels a stored key with its file name.
//
// Only the base name is used: the comment is shown in the Identities list and
// in audit-adjacent UI, and a full path there would expose the layout of the
// user's home directory for no benefit.
func identityComment(keyPath string) string {
	normalized := strings.ReplaceAll(keyPath, `\`, "/")
	base := path.Base(normalized)
	if base == "." || base == "/" || base == "" {
		return "ssh_config import"
	}
	return base + " (ssh_config)"
}
