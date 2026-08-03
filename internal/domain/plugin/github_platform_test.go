package plugin

import "testing"

func TestParseGitHubAssetName(t *testing.T) {
	osName, arch := ParseGitHubAssetName("demo-telnet-windows-amd64.exe")
	if osName != "windows" || arch != "amd64" {
		t.Fatalf("unexpected platform: %s/%s", osName, arch)
	}
}

func TestParseChecksumsFile(t *testing.T) {
	content := "abc123  demo-linux-amd64\n*def456  demo-windows-amd64.exe\n"
	got := ParseChecksumsFile(content)
	if got["demo-linux-amd64"] != "abc123" {
		t.Fatalf("unexpected linux checksum: %q", got["demo-linux-amd64"])
	}
	if got["demo-windows-amd64.exe"] != "def456" {
		t.Fatalf("unexpected windows checksum: %q", got["demo-windows-amd64.exe"])
	}
}

func TestExtractPlatformsFromAssets_SkipsChecksumFiles(t *testing.T) {
	assets := []GitHubReleaseAsset{
		{Name: "SHA256SUMS"},
		{Name: "demo-windows-amd64.exe"},
	}
	platforms := ExtractPlatformsFromAssets(assets, nil)
	if len(platforms) != 1 {
		t.Fatalf("expected 1 platform, got %d", len(platforms))
	}
	if platforms[0].OS != "windows" || platforms[0].Arch != "amd64" {
		t.Fatalf("unexpected platform: %+v", platforms[0])
	}
}

func TestBuildReleaseSummaries(t *testing.T) {
	summaries := BuildReleaseSummaries([]GitHubRelease{
		{
			TagName: "v1.0.0",
			Assets:  []GitHubReleaseAsset{{Name: "demo-windows-amd64.exe"}},
		},
	})
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Tag != "v1.0.0" {
		t.Fatalf("unexpected tag: %s", summaries[0].Tag)
	}
}

func TestHasReleaseWithPlatforms(t *testing.T) {
	if !HasReleaseWithPlatforms([]GitHubReleaseSummary{{Platforms: []PlatformInfo{{OS: "linux", Arch: "amd64"}}}}) {
		t.Fatal("expected platforms")
	}
	if HasReleaseWithPlatforms([]GitHubReleaseSummary{{Platforms: nil}}) {
		t.Fatal("expected no platforms")
	}
}
