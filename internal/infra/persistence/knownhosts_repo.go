package persistence

import (
	"bytes"
	"context"
	"crypto/hmac"
	// #nosec G505 -- the known_hosts hashed-host format is defined as HMAC-SHA1; see
	// hashedKnownHostMatches for why nothing here depends on SHA-1 being collision resistant.
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
)

// KnownHostsRepo implements domain.KnownHostsRepository backed by the vault.
type KnownHostsRepo struct {
	vault domain.VaultRepository
}

// NewKnownHostsRepo creates a KnownHostsRepo backed by the given VaultRepository.
func NewKnownHostsRepo(v domain.VaultRepository) *KnownHostsRepo {
	return &KnownHostsRepo{vault: v}
}

// Check verifies the remote host key against the known_hosts entries in the vault.
// Returns nil on match, ErrUnknownHost if missing, ErrHostKeyMismatch if different.
//
// Every entry for the host is examined before a mismatch is declared. This used to return on the
// first line whose host matched, which is wrong for the ordinary case: OpenSSH keeps several keys
// per host - an Ed25519 and an RSA one - and whichever the server negotiated might be recorded
// second. That produced "the host key has changed, you may be under attack" on a perfectly good
// connection, and the damage is not the false alarm. It is that the user learns to click through
// that banner, and that accepting it runs Replace.
func (r *KnownHostsRepo) Check(host string, remoteKey ssh.PublicKey) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("known hosts check: %w", err)
	}

	remoteKeyBytes := remoteKey.Marshal()
	hostIsKnown := false

	for _, line := range data.KnownHosts {
		lineHost, lineKey, err := parseKnownHostLine(line)
		if err != nil || !knownHostMatches(lineHost, host) {
			continue
		}
		hostIsKnown = true
		if lineKey.Type() == remoteKey.Type() && bytes.Equal(lineKey.Marshal(), remoteKeyBytes) {
			return nil
		}
	}

	if hostIsKnown {
		return fmt.Errorf("host %s key mismatch: %w", host, domain.ErrHostKeyMismatch)
	}
	return fmt.Errorf("host %s: %w", host, domain.ErrUnknownHost)
}

// Add stores a new host key entry and persists the vault.
func (r *KnownHostsRepo) Add(ctx context.Context, host string, key ssh.PublicKey) error {
	line := formatKnownHostLine(host, key)
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		data.KnownHosts = append(data.KnownHosts, line)
		return nil
	})
}

// List returns all known host entries for UI display.
func (r *KnownHostsRepo) List() ([]domain.KnownHostEntry, error) {
	data, err := r.vault.GetData()
	if err != nil {
		return nil, fmt.Errorf("known hosts list: %w", err)
	}

	var entries []domain.KnownHostEntry
	for _, line := range data.KnownHosts {
		host, key, err := parseKnownHostLine(line)
		if err != nil {
			continue
		}
		entries = append(entries, domain.KnownHostEntry{
			Host:        host,
			KeyType:     key.Type(),
			Fingerprint: ssh.FingerprintSHA256(key),
			Line:        line,
		})
	}
	return entries, nil
}

// Remove deletes a known host entry matching the given host pattern.
func (r *KnownHostsRepo) Remove(ctx context.Context, host string) error {
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		data.KnownHosts = filterKnownHosts(data.KnownHosts, func(lineHost string, _ ssh.PublicKey) bool {
			return knownHostMatches(lineHost, host)
		})
		return nil
	})
}

// Replace swaps the host's key of one type for newKey, leaving its other keys alone.
//
// Dropping every entry for the host, as this used to, destroys working trust: a host that offers
// both an Ed25519 and an RSA key has two entries, and accepting a change to one is not a statement
// about the other. Combined with the first-match bug in Check - which reported a mismatch whenever
// the negotiated key was recorded second - that turned one spurious warning into the permanent
// loss of a key the user had verified.
func (r *KnownHostsRepo) Replace(ctx context.Context, host string, newKey ssh.PublicKey) error {
	line := formatKnownHostLine(host, newKey)
	return r.vault.UpdateData(ctx, func(data *domain.VaultData) error {
		kept := filterKnownHosts(data.KnownHosts, func(lineHost string, lineKey ssh.PublicKey) bool {
			return knownHostMatches(lineHost, host) && lineKey.Type() == newKey.Type()
		})
		data.KnownHosts = append(kept, line)
		return nil
	})
}

// filterKnownHosts returns the lines drop did not select. An unparseable line is dropped either
// way: it can never match a host, so keeping it only carries corruption forward.
func filterKnownHosts(lines []string, drop func(host string, key ssh.PublicKey) bool) []string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		lineHost, lineKey, err := parseKnownHostLine(line)
		if err != nil || drop(lineHost, lineKey) {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}

// parseKnownHostLine extracts the host and public key from a known_hosts formatted line.
func parseKnownHostLine(line string) (host string, key ssh.PublicKey, err error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil, fmt.Errorf("empty or comment line")
	}

	_, hosts, pubKey, _, _, err := ssh.ParseKnownHosts([]byte(line + "\n"))
	if err != nil {
		return "", nil, fmt.Errorf("parse known host: %w", err)
	}
	if len(hosts) == 0 {
		return "", nil, fmt.Errorf("no hosts in line")
	}
	return hosts[0], pubKey, nil
}

// formatKnownHostLine creates a known_hosts line for the given host and key.
func formatKnownHostLine(host string, key ssh.PublicKey) string {
	return fmt.Sprintf("%s %s", normalizeHost(host), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))))
}

// normalizeHost strips surrounding brackets and ensures consistent formatting.
func normalizeHost(host string) string {
	return strings.TrimSpace(host)
}

// knownHostMatches reports whether a known_hosts host field designates host.
//
// It handles the hashed form OpenSSH writes when HashKnownHosts is on, which a plain string
// comparison can never match: `|1|<base64 salt>|<base64 HMAC-SHA1(salt, host)>`. Before this, a
// known_hosts file imported from OpenSSH with hashing enabled contributed nothing - every entry
// silently failed to match, the host came back as unknown, and the user was walked through
// trust-on-first-use again for a host they had already verified. Re-establishing trust from
// scratch is exactly the moment an attacker wants.
func knownHostMatches(lineHost, host string) bool {
	if strings.HasPrefix(lineHost, "|1|") {
		return hashedKnownHostMatches(lineHost, normalizeHost(host))
	}
	return normalizeHost(lineHost) == normalizeHost(host)
}

func hashedKnownHostMatches(entry, host string) bool {
	parts := strings.Split(entry, "|")
	// A hashed entry splits as ["", "1", salt, hash]: the leading empty field is the text before
	// the first separator.
	if len(parts) != 4 || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	// SHA-1 is not a choice here: HMAC-SHA1 is what the known_hosts hashed-host format specifies,
	// and this reads files OpenSSH wrote. Nothing is being authenticated by this digest - a match
	// only selects which lines to compare the host key against, and the key comparison is what
	// decides trust.
	// #nosec G401 -- required by the OpenSSH known_hosts wire format, not a security decision here
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(host))
	return hmac.Equal(mac.Sum(nil), want)
}
