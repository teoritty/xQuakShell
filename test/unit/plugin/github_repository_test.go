package plugin_test

import (
	"strings"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestGitHubRepository_Validate_ValidURL(t *testing.T) {
	repo := &domainplugin.GitHubRepository{
		URL: "https://github.com/user/repo",
	}
	if err := repo.Validate(); err != nil {
		t.Fatalf("expected valid URL, got %v", err)
	}
}

func TestGitHubRepository_Validate_InvalidURL(t *testing.T) {
	repo := &domainplugin.GitHubRepository{URL: "not-a-url"}
	if err := repo.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeURL(t *testing.T) {
	got, err := domainplugin.NormalizeURL("github.com/user/repo")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/user/repo" {
		t.Fatalf("got %q", got)
	}
}

func TestParseGitHubURL(t *testing.T) {
	owner, repo, err := domainplugin.ParseGitHubURL("https://github.com/acme/widgets/")
	if err != nil {
		t.Fatal(err)
	}
	if owner != "acme" || repo != "widgets" {
		t.Fatalf("got %s/%s", owner, repo)
	}
}

func TestGitHubPluginMetadata_SupportsCurrentPlatform(t *testing.T) {
	meta := &domainplugin.GitHubPluginMetadata{
		Platforms: []domainplugin.PlatformInfo{
			{OS: domainplugin.CurrentPlatformOS(), Arch: domainplugin.CurrentPlatformArch()},
		},
	}
	if !meta.SupportsCurrentPlatform() {
		t.Fatal("expected current platform support")
	}
}

func TestParseGitHubAssetNameConvention(t *testing.T) {
	osName, arch := parseGitHubAssetName("demo-telnet-windows-amd64.exe")
	if osName != "windows" || arch != "amd64" {
		t.Fatalf("got %s/%s", osName, arch)
	}
}

func parseGitHubAssetName(filename string) (string, string) {
	name := strings.ToLower(filename)
	name = strings.TrimSuffix(name, ".exe")
	name = strings.TrimSuffix(name, ".zip")
	name = strings.TrimSuffix(name, ".tar.gz")
	name = strings.TrimSuffix(name, ".tgz")
	parts := strings.Split(name, "-")
	if len(parts) < 3 {
		return "", ""
	}
	arch := parts[len(parts)-1]
	osName := parts[len(parts)-2]
	if !domainplugin.IsValidPlatformOS(osName) || !domainplugin.IsValidPlatformArch(arch) {
		return "", ""
	}
	return osName, arch
}
