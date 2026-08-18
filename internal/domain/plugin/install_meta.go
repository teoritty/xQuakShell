package plugin

// InstallMetaSource identifies how a plugin was installed.
type InstallMetaSource string

const (
	InstallMetaSourceGitHub InstallMetaSource = "github"
	InstallMetaSourceGitLab InstallMetaSource = "gitlab"
)

// InstallMetaSourceForForge names the provenance value recorded for a forge install.
func InstallMetaSourceForForge(forge Forge) InstallMetaSource {
	if forge == ForgeGitLab {
		return InstallMetaSourceGitLab
	}
	return InstallMetaSourceGitHub
}

// PluginInstallMeta records provenance for user-installed plugins.
type PluginInstallMeta struct {
	Source        InstallMetaSource `json:"source"`
	RepositoryURL string            `json:"repositoryUrl,omitempty"`
	ReleaseTag    string            `json:"releaseTag,omitempty"`
}
