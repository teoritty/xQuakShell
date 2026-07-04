package plugin

// InstallMetaSource identifies how a plugin was installed.
type InstallMetaSource string

const (
	InstallMetaSourceGitHub InstallMetaSource = "github"
)

// PluginInstallMeta records provenance for user-installed plugins.
type PluginInstallMeta struct {
	Source        InstallMetaSource `json:"source"`
	RepositoryURL string            `json:"repositoryUrl,omitempty"`
	ReleaseTag    string            `json:"releaseTag,omitempty"`
}
