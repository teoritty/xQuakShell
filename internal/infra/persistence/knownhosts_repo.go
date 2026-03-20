package persistence

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
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
func (r *KnownHostsRepo) Check(host string, remoteKey ssh.PublicKey) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("known hosts check: %w", err)
	}

	normalizedHost := normalizeHost(host)
	remoteKeyBytes := remoteKey.Marshal()

	for _, line := range data.KnownHosts {
		lineHost, lineKey, err := parseKnownHostLine(line)
		if err != nil {
			continue
		}
		if normalizeHost(lineHost) != normalizedHost {
			continue
		}
		if lineKey.Type() == remoteKey.Type() && bytesEqual(lineKey.Marshal(), remoteKeyBytes) {
			return nil
		}
		return fmt.Errorf("host %s key mismatch: %w", host, domain.ErrHostKeyMismatch)
	}

	return fmt.Errorf("host %s: %w", host, domain.ErrUnknownHost)
}

// Add stores a new host key entry and persists the vault.
func (r *KnownHostsRepo) Add(ctx context.Context, host string, key ssh.PublicKey) error {
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("known hosts add get data: %w", err)
	}

	line := formatKnownHostLine(host, key)
	data.KnownHosts = append(data.KnownHosts, line)

	return r.vault.SaveData(ctx, data)
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
	data, err := r.vault.GetData()
	if err != nil {
		return fmt.Errorf("known hosts remove get data: %w", err)
	}

	normalizedHost := normalizeHost(host)
	filtered := make([]string, 0, len(data.KnownHosts))
	for _, line := range data.KnownHosts {
		lineHost, _, err := parseKnownHostLine(line)
		if err != nil || normalizeHost(lineHost) == normalizedHost {
			continue
		}
		filtered = append(filtered, line)
	}
	data.KnownHosts = filtered

	return r.vault.SaveData(ctx, data)
}

// Replace removes existing entries for the host and adds the new key.
func (r *KnownHostsRepo) Replace(ctx context.Context, host string, newKey ssh.PublicKey) error {
	if err := r.Remove(ctx, host); err != nil {
		return fmt.Errorf("known hosts replace remove: %w", err)
	}
	return r.Add(ctx, host, newKey)
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

// bytesEqual compares two byte slices for equality.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
