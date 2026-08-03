package usecase

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"xquakshell/internal/domain"
)

// SSHConfigHostPreview is one parsed host as the import dialog shows it,
// before anything is written to the vault.
type SSHConfigHostPreview struct {
	Alias    string
	HostName string
	Port     int
	User     string
	// KeyCount is how many private keys this host references, counting its
	// jump hops. The dialog uses it for a badge, so the user can see which
	// rows will read key material before enabling key import.
	KeyCount int
	// JumpAliases names the ProxyJump chain in traversal order.
	JumpAliases []string
	// Duplicate marks a host that already exists in the vault under the same
	// host, port and user. Such rows start unselected: re-importing a config
	// after adding two hosts should offer the two, not forty copies.
	Duplicate bool
}

// SSHConfigPreview is the result of inspecting a config file.
type SSHConfigPreview struct {
	Path  string
	Hosts []SSHConfigHostPreview
	// KeyFileCount is the number of distinct key files across all hosts.
	KeyFileCount int
	Notices      []domain.SSHConfigNotice
}

// SSHConfigImportRequest describes one import action.
type SSHConfigImportRequest struct {
	// Path is the config file to import from. It is re-parsed rather than
	// trusted from a previous preview, so the backend derives every key path
	// itself and the caller cannot name a file of its own choosing.
	Path string
	// Aliases selects which parsed hosts to import. Unknown aliases are
	// ignored rather than rejected: the file may have changed since preview.
	Aliases []string
	// FolderID is the destination folder; empty means the tree root.
	FolderID string
	// ImportKeys enables reading the referenced private keys into the vault.
	ImportKeys bool
}

// SSHConfigImportResult reports what one import actually produced.
type SSHConfigImportResult struct {
	Connections []domain.Connection
	// ImportedKeys counts distinct key files stored as vault identities.
	ImportedKeys int
	// FailedKeys counts keys that could not be read or stored. A failing key
	// never blocks its connection: an unreadable key is a missing convenience,
	// while a missing connection is lost work.
	FailedKeys int
	// SkippedAliases lists requested hosts the config no longer declares.
	SkippedAliases []string
}

// SSHConfigImportService imports OpenSSH client configurations into the vault.
type SSHConfigImportService struct {
	connRepo  domain.ConnectionRepository
	identRepo domain.IdentityRepository
	importer  domain.SSHConfigImporter
}

// NewSSHConfigImportService wires the import service to its repositories.
func NewSSHConfigImportService(
	connRepo domain.ConnectionRepository,
	identRepo domain.IdentityRepository,
	importer domain.SSHConfigImporter,
) *SSHConfigImportService {
	return &SSHConfigImportService{connRepo: connRepo, identRepo: identRepo, importer: importer}
}

// DefaultPath returns the user's conventional ssh_config path, or an empty
// string when there is none.
func (s *SSHConfigImportService) DefaultPath() (string, error) {
	if s == nil || s.importer == nil {
		return "", fmt.Errorf("ssh config importer unavailable")
	}
	return s.importer.DefaultPath()
}

// Preview parses a config file and marks which hosts already exist.
func (s *SSHConfigImportService) Preview(ctx context.Context, path string) (SSHConfigPreview, error) {
	if s == nil || s.importer == nil {
		return SSHConfigPreview{}, fmt.Errorf("ssh config importer unavailable")
	}
	parsed, err := s.importer.Parse(path)
	if err != nil {
		return SSHConfigPreview{}, err
	}
	existing, err := s.existingKeys(ctx)
	if err != nil {
		return SSHConfigPreview{}, err
	}

	preview := SSHConfigPreview{Path: path, Notices: parsed.Notices}
	keyFiles := map[string]bool{}
	for _, host := range parsed.Hosts {
		for _, file := range allIdentityFiles(host) {
			keyFiles[file] = true
		}
		preview.Hosts = append(preview.Hosts, toHostPreview(host, existing))
	}
	preview.KeyFileCount = len(keyFiles)
	return preview, nil
}

// Import creates vault connections for the selected hosts.
//
// The config is re-parsed here rather than carried over from Preview: the file
// is the source of truth, and re-reading it is what keeps the caller from
// supplying host or key paths of its own.
func (s *SSHConfigImportService) Import(ctx context.Context, req SSHConfigImportRequest) (SSHConfigImportResult, error) {
	if s == nil || s.connRepo == nil || s.importer == nil {
		return SSHConfigImportResult{}, fmt.Errorf("connection repository unavailable")
	}
	parsed, err := s.importer.Parse(req.Path)
	if err != nil {
		return SSHConfigImportResult{}, err
	}

	selected, result := selectHosts(parsed.Hosts, req.Aliases)
	keys := newKeyImporter(s.identRepo, s.importer, req.ImportKeys)
	for i, host := range selected {
		conn := s.buildConnection(ctx, host, req.FolderID, i, keys)
		if err := s.connRepo.Save(ctx, &conn); err != nil {
			// Abort rather than continue: a failing Save is systemic (a locked
			// or unwritable vault), so the remaining hosts would fail the same
			// way. The connections already created are returned so the caller
			// can report exactly how far the import got.
			result.ImportedKeys, result.FailedKeys = keys.counts()
			return result, fmt.Errorf("save connection %s: %w", host.Alias, err)
		}
		result.Connections = append(result.Connections, conn)
	}
	result.ImportedKeys, result.FailedKeys = keys.counts()
	return result, nil
}

// existingKeys indexes the vault's current connections by identity key, for
// duplicate detection.
func (s *SSHConfigImportService) existingKeys(ctx context.Context) (map[string]bool, error) {
	if s.connRepo == nil {
		return map[string]bool{}, nil
	}
	connections, err := s.connRepo.GetAllConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing connections: %w", err)
	}
	keys := make(map[string]bool, len(connections))
	for _, c := range connections {
		keys[connectionIdentityKey(c.Host, c.Port, c.EffectiveUsername())] = true
	}
	return keys, nil
}

// selectHosts filters parsed hosts down to the requested aliases, preserving
// config order, and records aliases that no longer exist.
func selectHosts(hosts []domain.SSHConfigHost, aliases []string) ([]domain.SSHConfigHost, SSHConfigImportResult) {
	wanted := make(map[string]bool, len(aliases))
	for _, alias := range aliases {
		wanted[strings.ToLower(alias)] = true
	}

	var (
		selected []domain.SSHConfigHost
		found    = make(map[string]bool, len(aliases))
	)
	for _, host := range hosts {
		key := strings.ToLower(host.Alias)
		if !wanted[key] {
			continue
		}
		found[key] = true
		selected = append(selected, host)
	}

	var result SSHConfigImportResult
	for _, alias := range aliases {
		if !found[strings.ToLower(alias)] {
			result.SkippedAliases = append(result.SkippedAliases, alias)
		}
	}
	return selected, result
}

// toHostPreview projects a parsed host into its dialog representation.
func toHostPreview(host domain.SSHConfigHost, existing map[string]bool) SSHConfigHostPreview {
	preview := SSHConfigHostPreview{
		Alias:    host.Alias,
		HostName: host.HostName,
		Port:     effectivePort(host.Port),
		User:     host.User,
		KeyCount: len(allIdentityFiles(host)),
	}
	for _, hop := range host.JumpHops {
		preview.JumpAliases = append(preview.JumpAliases, hop.Alias)
	}
	preview.Duplicate = existing[connectionIdentityKey(host.HostName, preview.Port, host.User)]
	return preview
}

// connectionIdentityKey is the value duplicate detection compares.
//
// Host and user are compared case-insensitively because DNS names and, in
// practice, account names are; the port is normalised so that an explicit
// "Port 22" and an omitted one are the same connection.
func connectionIdentityKey(host string, port int, user string) string {
	return strings.ToLower(strings.TrimSpace(host)) + "\x00" +
		strconv.Itoa(effectivePort(port)) + "\x00" +
		strings.ToLower(strings.TrimSpace(user))
}

// allIdentityFiles lists every key a host would use, its jump hops included.
func allIdentityFiles(host domain.SSHConfigHost) []string {
	files := append([]string(nil), host.IdentityFiles...)
	for _, hop := range host.JumpHops {
		files = append(files, hop.IdentityFiles...)
	}
	return files
}

func effectivePort(port int) int {
	if port <= 0 {
		return domain.DefaultSSHPort
	}
	return port
}
