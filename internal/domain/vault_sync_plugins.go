package domain

// Plugin install sources a synced plugin reference may name (spec §6.4).
const (
	PluginSourceGitHub = "github"
	PluginSourceManual = "manual"
)

// SyncPlugin is a plugin installed on one of the user's devices, by reference: what to install and
// the digests the installed bundle must match. Receiving one never installs anything; a device that
// lacks the plugin offers it.
type SyncPlugin struct {
	ID      string           `json:"id"`
	Version string           `json:"version"`
	Source  SyncPluginSource `json:"source"`
	// ManifestDigest and SumsDigest are hex SHA-256 of plugin.json and of SHA256SUMS. A manual
	// install may have no SHA256SUMS.
	ManifestDigest string `json:"manifestDigest"`
	SumsDigest     string `json:"sumsDigest,omitempty"`
	// PublisherKey is the fingerprint of the key the manifest is signed with; empty when unsigned.
	PublisherKey string `json:"publisherKey,omitempty"`
}

// SyncPluginSource says where a plugin came from. Repo and Tag are set for GitHub only.
type SyncPluginSource struct {
	Kind string `json:"kind"`
	Repo string `json:"repo,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

// SyncPluginFile is one data file a plugin declared for sync (manifest sync.paths), as it was last
// synced. Path is slash-separated and relative to the plugin's data directory.
type SyncPluginFile struct {
	Plugin  string `json:"plugin"`
	Path    string `json:"path"`
	Mode    uint32 `json:"mode"`
	SHA256  string `json:"sha256"`
	Content []byte `json:"content"`
}

// Key is the file's natural key. Neither a plugin id nor a valid sync path can hold a newline.
func (f SyncPluginFile) Key() string { return f.Plugin + "\n" + f.Path }
