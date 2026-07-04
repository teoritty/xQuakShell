package plugin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
	infragithub "ssh-client/internal/infra/github"
)

func TestParseChecksumsFile(t *testing.T) {
	content := "a1b2c3 demo-windows-amd64.exe\n# comment\ne5f6g7 demo-linux-amd64\n"
	got := infragithub.ParseChecksumsFile(content)
	if got["demo-windows-amd64.exe"] != "a1b2c3" {
		t.Fatalf("unexpected checksum map: %#v", got)
	}
	if got["demo-linux-amd64"] != "e5f6g7" {
		t.Fatalf("unexpected checksum map: %#v", got)
	}
}

func TestGetLatestRelease_NoReleasesReturnsDedicatedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/owner/repo/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := infragithub.NewClientWithBaseURL(server.URL)

	_, err := client.GetLatestRelease(context.Background(), "owner", "repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, domainplugin.ErrRepositoryNotFound) {
		t.Fatalf("unexpected ErrRepositoryNotFound: %v", err)
	}
	if !errors.Is(err, domainplugin.ErrNoReleases) {
		t.Fatalf("expected ErrNoReleases, got %v", err)
	}
}

func TestGetLatestRelease_FallsBackToPrerelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/owner/repo/releases":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0","draft":false,"prerelease":true,"assets":[{"name":"demo-windows-amd64.exe"}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := infragithub.NewClientWithBaseURL(server.URL)

	release, err := client.GetLatestRelease(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if release.TagName != "v1.0.0" {
		t.Fatalf("expected v1.0.0, got %q", release.TagName)
	}
	if !release.Prerelease {
		t.Fatal("expected prerelease flag")
	}
}

func TestTotalDownloadCount(t *testing.T) {
	total := infragithub.TotalDownloadCount([]infragithub.Asset{
		{DownloadCount: 3},
		{DownloadCount: 7},
	})
	if total != 10 {
		t.Fatalf("got %d", total)
	}
}
