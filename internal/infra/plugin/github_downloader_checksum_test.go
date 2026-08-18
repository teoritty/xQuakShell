package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
	infraforge "xquakshell/internal/infra/forge"
	infragithub "xquakshell/internal/infra/github"
)

// releaseServer serves one release whose single asset is assetBody, over the two endpoints the
// downloader uses: the release-by-tag lookup and the asset download itself.
func releaseServer(t *testing.T, assetName string, assetBody []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("/asset", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(assetBody)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		release := map[string]any{
			"tag_name": "v1.0.0",
			"assets": []map[string]any{{
				"name":                 assetName,
				"browser_download_url": srv.URL + "/asset",
				"size":                 len(assetBody),
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(release)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newTestDownloader points the GitHub arm of a real router at the test server, so these tests
// exercise the same routing a production download goes through.
func newTestDownloader(t *testing.T, srv *httptest.Server) *BinaryDownloader {
	t.Helper()
	router := infraforge.NewRouterWithClients(map[domainplugin.Forge]infraforge.Client{
		domainplugin.ForgeGitHub: infragithub.NewUseCaseClient(infragithub.NewClientWithBaseURL(srv.URL)),
	})
	return NewBinaryDownloader(router, t.TempDir())
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// The hole this closes: an empty ExpectedChecksum used to skip verification entirely, so a release
// with no SHA256SUMS - or one whose SHA256SUMS fetch merely failed - installed with TLS to
// the forge as the only thing standing between a tampered asset and the user's machine. The .xqsp
// path has always required checksums; the bare-binary path skipping them was an asymmetry.
func TestFetchAssetRefusesAnAssetWithNoExpectedChecksum(t *testing.T) {
	body := []byte("plugin binary")
	srv := releaseServer(t, "xqs-demo-linux-amd64", body)
	d := newTestDownloader(t, srv)

	_, cleanup, err := d.DownloadAsset(context.Background(), domainplugin.AssetDownloadRequest{
		Forge: domainplugin.ForgeGitHub, Owner: "o", Repo: "r", Tag: "v1.0.0",
		AssetName: "xqs-demo-linux-amd64",
		EntryName: "xqs-demo",
	})
	defer cleanup()

	if !errors.Is(err, domainplugin.ErrChecksumUnavailable) {
		t.Fatalf("DownloadAsset err = %v, want ErrChecksumUnavailable", err)
	}
}

func TestFetchAssetAcceptsAMatchingChecksum(t *testing.T) {
	body := []byte("plugin binary")
	srv := releaseServer(t, "xqs-demo-linux-amd64", body)
	d := newTestDownloader(t, srv)

	asset, cleanup, err := d.DownloadAsset(context.Background(), domainplugin.AssetDownloadRequest{
		Forge: domainplugin.ForgeGitHub, Owner: "o", Repo: "r", Tag: "v1.0.0",
		AssetName:        "xqs-demo-linux-amd64",
		ExpectedChecksum: sha256Hex(body),
		EntryName:        "xqs-demo",
	})
	defer cleanup()

	if err != nil {
		t.Fatalf("DownloadAsset err = %v, want nil", err)
	}
	if asset.Path == "" {
		t.Error("a verified asset returned no path")
	}
}

func TestFetchAssetRejectsAMismatchedChecksum(t *testing.T) {
	srv := releaseServer(t, "xqs-demo-linux-amd64", []byte("tampered binary"))
	d := newTestDownloader(t, srv)

	_, cleanup, err := d.DownloadAsset(context.Background(), domainplugin.AssetDownloadRequest{
		Forge: domainplugin.ForgeGitHub, Owner: "o", Repo: "r", Tag: "v1.0.0",
		AssetName:        "xqs-demo-linux-amd64",
		ExpectedChecksum: sha256Hex([]byte("the binary the author published")),
		EntryName:        "xqs-demo",
	})
	defer cleanup()

	if err == nil {
		t.Fatal("a body that does not match its SHA256SUMS entry was accepted")
	}
	if errors.Is(err, domainplugin.ErrChecksumUnavailable) {
		t.Fatalf("a mismatch was reported as an absent checksum: %v", err)
	}
}

// SHA256SUMS cannot appear in its own listing, so it is the one asset allowed through unverified.
// If this ever fails, the checksums file stopped being fetchable and every install fails with it.
func TestDownloadAssetContentIsTheOnlyUnverifiedPath(t *testing.T) {
	sums := []byte(fmt.Sprintf("%s  xqs-demo-linux-amd64\n", sha256Hex([]byte("plugin binary"))))
	srv := releaseServer(t, "SHA256SUMS", sums)
	d := newTestDownloader(t, srv)

	got, err := d.DownloadAssetContent(context.Background(), domainplugin.RepoRef{Forge: domainplugin.ForgeGitHub, Owner: "o", Repo: "r"}, "v1.0.0", "SHA256SUMS")
	if err != nil {
		t.Fatalf("DownloadAssetContent err = %v, want nil", err)
	}
	if string(got) != string(sums) {
		t.Errorf("DownloadAssetContent returned %q, want %q", got, sums)
	}
}
