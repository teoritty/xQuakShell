package plugin

import (
	"fmt"
	"runtime"
)

// GitHubPluginMetadata contains plugin information extracted from xqsp.json.
type GitHubPluginMetadata struct {
	RepositoryURL  string         `json:"repositoryUrl"`
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	Description    string         `json:"description,omitempty"`
	Author         string         `json:"author,omitempty"`
	Homepage       string         `json:"homepage,omitempty"`
	License        string         `json:"license,omitempty"`
	MinCoreVersion string         `json:"minCoreVersion,omitempty"`
	Platforms      []PlatformInfo `json:"platforms"`
	Tags           []string       `json:"tags,omitempty"`
	README         string         `json:"readme,omitempty"`
	LatestRelease  string         `json:"latestRelease"`
	PublishedAt    string         `json:"publishedAt,omitempty"`
	DownloadCount  int            `json:"downloadCount"`
	Manifest       Manifest       `json:"-"`
}

// PlatformInfo describes platform support for a binary.
type PlatformInfo struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	AssetName string `json:"assetName"`
	Checksum  string `json:"checksum,omitempty"`
}

// Validate ensures metadata is complete and valid.
func (m *GitHubPluginMetadata) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("%w: plugin ID is required", ErrInvalidPluginMetadata)
	}
	if m.Name == "" {
		return fmt.Errorf("%w: plugin name is required", ErrInvalidPluginMetadata)
	}
	if m.Version == "" {
		return fmt.Errorf("%w: plugin version is required", ErrInvalidPluginMetadata)
	}
	if m.RepositoryURL == "" {
		return fmt.Errorf("%w: repository URL is required", ErrInvalidPluginMetadata)
	}
	if len(m.Platforms) == 0 {
		return fmt.Errorf("%w: at least one platform is required", ErrInvalidPluginMetadata)
	}

	for i, p := range m.Platforms {
		if !IsValidPlatformOS(p.OS) {
			return fmt.Errorf("%w: invalid OS at index %d: %s", ErrInvalidPluginMetadata, i, p.OS)
		}
		if !IsValidPlatformArch(p.Arch) {
			return fmt.Errorf("%w: invalid arch at index %d: %s", ErrInvalidPluginMetadata, i, p.Arch)
		}
	}

	return nil
}

// SupportsCurrentPlatform checks if plugin supports current OS/architecture.
func (m *GitHubPluginMetadata) SupportsCurrentPlatform() bool {
	return m.GetPlatformForCurrent() != nil
}

// GetPlatformForCurrent returns the platform info for current system.
func (m *GitHubPluginMetadata) GetPlatformForCurrent() *PlatformInfo {
	currentOS := CurrentPlatformOS()
	currentArch := CurrentPlatformArch()

	for i := range m.Platforms {
		if m.Platforms[i].OS == currentOS && m.Platforms[i].Arch == currentArch {
			return &m.Platforms[i]
		}
	}
	return nil
}

// RequiresSecretAccess reports whether the plugin declared vault.getSecret.
func (m *GitHubPluginMetadata) RequiresSecretAccess() bool {
	return m.Manifest.RequiresSecretAccess()
}

// IsValidPlatformOS reports whether os is a supported plugin platform.
func IsValidPlatformOS(os string) bool {
	switch os {
	case "windows", "linux", "darwin":
		return true
	default:
		return false
	}
}

// IsValidPlatformArch reports whether arch is a supported plugin platform.
func IsValidPlatformArch(arch string) bool {
	switch arch {
	case "amd64", "arm64", "386", "arm":
		return true
	default:
		return false
	}
}

// CurrentPlatformOS returns normalized runtime GOOS for plugin matching.
func CurrentPlatformOS() string {
	switch runtime.GOOS {
	case "windows", "linux", "darwin":
		return runtime.GOOS
	default:
		return runtime.GOOS
	}
}

// CurrentPlatformArch returns runtime GOARCH for plugin matching.
func CurrentPlatformArch() string {
	return runtime.GOARCH
}
