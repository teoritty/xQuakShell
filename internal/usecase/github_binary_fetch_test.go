package usecase

import (
	"context"
	"errors"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// checksumDownloader answers DownloadAssetContent with whatever the test set up, so a fetch
// failure can be told apart from a release that never published the file.
type checksumDownloader struct {
	content []byte
	err     error
	calls   int
}

func (d *checksumDownloader) DownloadAsset(context.Context, domainplugin.AssetDownloadRequest) (domainplugin.DownloadedAsset, func(), error) {
	return domainplugin.DownloadedAsset{}, func() {}, errors.New("not used in this test")
}

func (d *checksumDownloader) DownloadAssetContent(context.Context, domainplugin.RepoRef, string, string) ([]byte, error) {
	d.calls++
	return d.content, d.err
}

// testRepoRef is any valid ref; these tests are about the checksum listing, not about routing.
var testRepoRef = domainplugin.RepoRef{Forge: domainplugin.ForgeGitHub, Owner: "o", Repo: "r"}

func releaseWithAssets(names ...string) *domainplugin.GitHubRelease {
	release := &domainplugin.GitHubRelease{TagName: "v1.0.0"}
	for _, name := range names {
		release.Assets = append(release.Assets, domainplugin.GitHubReleaseAsset{Name: name})
	}
	return release
}

// The hole this closes: a failed fetch used to `continue` and end up returning nil, which is the
// same value as "this release published no checksums at all". So a rate limit, a 5xx, or a dropped
// connection silently turned the download's integrity check off, and nothing said so. Failing one
// HTTPS request - not forging it, just failing it - was enough to get an unverified install.
func TestLoadReleaseChecksums_FetchFailureIsAnErrorNotAnAbsence(t *testing.T) {
	downloader := &checksumDownloader{err: errors.New("403 rate limit exceeded")}
	svc := &GitHubPluginService{downloader: downloader}

	got, err := svc.loadReleaseChecksums(context.Background(), testRepoRef, releaseWithAssets("SHA256SUMS", "xqs-demo-linux-amd64"))

	if err == nil {
		t.Fatal("a failed SHA256SUMS fetch returned no error; it is indistinguishable from a release with no checksums")
	}
	if got != nil {
		t.Errorf("got %v, want nil on failure", got)
	}
}

// A release that genuinely lists no checksums asset is a different fact, and it must not be
// reported as a transport failure - the install refuses either way, but for a different reason and
// with a different message.
func TestLoadReleaseChecksums_NoChecksumsAssetIsNotAnError(t *testing.T) {
	downloader := &checksumDownloader{}
	svc := &GitHubPluginService{downloader: downloader}

	got, err := svc.loadReleaseChecksums(context.Background(), testRepoRef, releaseWithAssets("xqs-demo-linux-amd64"))

	if err != nil {
		t.Fatalf("loadReleaseChecksums err = %v, want nil when the release lists no checksums asset", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if downloader.calls != 0 {
		t.Errorf("downloaded %d assets, want 0: there was nothing to fetch", downloader.calls)
	}
}

func TestLoadReleaseChecksums_ParsesAPublishedListing(t *testing.T) {
	downloader := &checksumDownloader{
		content: []byte("abc123  xqs-demo-linux-amd64\ndef456  xqs-demo-windows-amd64.exe\n"),
	}
	svc := &GitHubPluginService{downloader: downloader}

	got, err := svc.loadReleaseChecksums(context.Background(), testRepoRef, releaseWithAssets("SHA256SUMS"))

	if err != nil {
		t.Fatalf("loadReleaseChecksums err = %v, want nil", err)
	}
	if got["xqs-demo-linux-amd64"] != "abc123" {
		t.Errorf("checksum for the linux asset = %q, want abc123", got["xqs-demo-linux-amd64"])
	}
}

// checksums.txt is accepted under the same rules as SHA256SUMS, failure included.
func TestLoadReleaseChecksums_ChecksumsTxtFailureIsAlsoAnError(t *testing.T) {
	downloader := &checksumDownloader{err: errors.New("connection reset")}
	svc := &GitHubPluginService{downloader: downloader}

	if _, err := svc.loadReleaseChecksums(context.Background(), testRepoRef, releaseWithAssets("checksums.txt")); err == nil {
		t.Fatal("a failed checksums.txt fetch returned no error")
	}
}
