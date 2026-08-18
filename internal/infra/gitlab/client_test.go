package gitlab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// requestRecorder serves canned JSON and keeps the paths it was asked for, so a test can assert on
// the URL the client built rather than only on what it decoded.
type requestRecorder struct {
	paths   []string
	queries []string
	status  int
	body    string
	headers map[string]string
}

func newTestClient(t *testing.T, rec *requestRecorder) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.paths = append(rec.paths, r.URL.EscapedPath())
		rec.queries = append(rec.queries, r.URL.RawQuery)
		for k, v := range rec.headers {
			w.Header().Set(k, v)
		}
		if rec.status != 0 {
			w.WriteHeader(rec.status)
		}
		_, _ = w.Write([]byte(rec.body))
	}))
	t.Cleanup(srv.Close)
	return NewClientWithBaseURL(srv.URL)
}

// A GitLab project is addressed by one URL-encoded id, not by path segments. Sending
// "group/subgroup/plugin" unencoded produces a URL GitLab reads as a different endpoint entirely,
// and the 404 that comes back names a path the user never typed.
func TestProjectIDEncodesTheWholeNamespace(t *testing.T) {
	if got := projectID("group/subgroup", "plugin"); got != "group%2Fsubgroup%2Fplugin" {
		t.Errorf("projectID = %q, want the slashes percent-encoded", got)
	}
}

func TestGetFileContentAddressesTheRawEndpoint(t *testing.T) {
	rec := &requestRecorder{body: "manifest bytes"}
	client := newTestClient(t, rec)

	got, err := client.GetFileContent(context.Background(), "group/sub", "plugin", "xqsp.json", "v1.2.0")
	if err != nil {
		t.Fatalf("GetFileContent err = %v, want nil", err)
	}

	if string(got) != "manifest bytes" {
		t.Errorf("content = %q, want the raw body", got)
	}
	wantPath := "/projects/group%2Fsub%2Fplugin/repository/files/xqsp.json/raw"
	if rec.paths[0] != wantPath {
		t.Errorf("path = %q, want %q", rec.paths[0], wantPath)
	}
	if rec.queries[0] != "ref=v1.2.0" {
		t.Errorf("query = %q, want ref=v1.2.0", rec.queries[0])
	}
}

// GitLab's raw endpoint does not treat a missing ref as "the default branch" the way GitHub's
// contents endpoint does, so an empty ref has to be spelled out or the request 404s.
func TestGetFileContentSubstitutesHeadForAnEmptyRef(t *testing.T) {
	rec := &requestRecorder{body: "x"}
	client := newTestClient(t, rec)

	if _, err := client.GetFileContent(context.Background(), "o", "r", "xqsp.json", ""); err != nil {
		t.Fatalf("GetFileContent err = %v, want nil", err)
	}

	if rec.queries[0] != "ref=HEAD" {
		t.Errorf("query = %q, want ref=HEAD for an empty ref", rec.queries[0])
	}
}

func TestGetFileContentReportsAMissingFileAsNotFound(t *testing.T) {
	rec := &requestRecorder{status: http.StatusNotFound, body: `{"message":"404 File Not Found"}`}
	client := newTestClient(t, rec)

	_, err := client.GetFileContent(context.Background(), "o", "r", "xqsp.json", "")

	// The usecase layer decides a manifest is absent by matching "not found" in the message, so the
	// wording here is load-bearing: change it and a repository without a manifest reports a
	// transport failure instead.
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetFileContent err = %v, want an error saying not found", err)
	}
}

func TestListPublishedReleasesDecodesAssetLinks(t *testing.T) {
	rec := &requestRecorder{body: `[
		{"tag_name":"v2.0.0","name":"Two","released_at":"2026-02-01T10:00:00.000Z",
		 "assets":{"links":[{"name":"xqs-demo-linux-amd64","url":"https://gitlab.com/u/p/-/x",
		                     "direct_asset_url":"https://gitlab.com/u/p/-/releases/v2.0.0/downloads/bin"}]}},
		{"tag_name":"v1.0.0","name":"One","released_at":"2026-01-01T10:00:00.000Z","assets":{"links":[]}}
	]`}
	client := newTestClient(t, rec)

	releases, err := client.ListPublishedReleases(context.Background(), "u", "p")
	if err != nil {
		t.Fatalf("ListPublishedReleases err = %v, want nil", err)
	}

	if len(releases) != 2 {
		t.Fatalf("got %d releases, want 2", len(releases))
	}
	if releases[0].TagName != "v2.0.0" {
		t.Errorf("first release = %q, want the newest (v2.0.0)", releases[0].TagName)
	}
	if got := releases[0].Assets.Links[0].DownloadURL(); got != "https://gitlab.com/u/p/-/releases/v2.0.0/downloads/bin" {
		t.Errorf("DownloadURL = %q, want the direct asset URL to win over url", got)
	}
}

// url is the fallback when a release link has no permalink; dropping it would leave those assets
// with an empty download URL and the install would fail with no explanation.
func TestAssetDownloadURLFallsBackToURL(t *testing.T) {
	asset := Asset{Name: "x", URL: "https://gitlab.com/u/p/-/jobs/1/artifacts/x"}

	if got := asset.DownloadURL(); got != asset.URL {
		t.Errorf("DownloadURL = %q, want %q when direct_asset_url is absent", got, asset.URL)
	}
}

// GitLab publishes no /releases/latest, and an upcoming release is dated in the future. Offering
// one as the latest tells the user to install something that is not out yet.
func TestGetLatestReleaseSkipsUpcomingReleases(t *testing.T) {
	rec := &requestRecorder{body: `[
		{"tag_name":"v3.0.0","upcoming_release":true,"assets":{"links":[]}},
		{"tag_name":"v2.0.0","upcoming_release":false,"assets":{"links":[]}}
	]`}
	client := newTestClient(t, rec)

	release, err := client.GetLatestRelease(context.Background(), "u", "p")
	if err != nil {
		t.Fatalf("GetLatestRelease err = %v, want nil", err)
	}

	if release.TagName != "v2.0.0" {
		t.Errorf("latest = %q, want v2.0.0; an upcoming release is not published yet", release.TagName)
	}
}

func TestGetLatestReleaseReportsAProjectWithNoReleases(t *testing.T) {
	rec := &requestRecorder{body: `[]`}
	client := newTestClient(t, rec)

	_, err := client.GetLatestRelease(context.Background(), "u", "p")

	if !errors.Is(err, domainplugin.ErrNoReleases) {
		t.Fatalf("GetLatestRelease err = %v, want ErrNoReleases", err)
	}
}

func TestGetReleaseByTagEncodesTheTag(t *testing.T) {
	rec := &requestRecorder{body: `{"tag_name":"release/1.0","assets":{"links":[]}}`}
	client := newTestClient(t, rec)

	if _, err := client.GetReleaseByTag(context.Background(), "u", "p", "release/1.0"); err != nil {
		t.Fatalf("GetReleaseByTag err = %v, want nil", err)
	}

	wantPath := "/projects/u%2Fp/releases/release%2F1.0"
	if rec.paths[0] != wantPath {
		t.Errorf("path = %q, want %q; an unescaped tag becomes an extra path segment", rec.paths[0], wantPath)
	}
}

func TestGetReleaseByTagReportsAMissingRelease(t *testing.T) {
	rec := &requestRecorder{status: http.StatusNotFound, body: `{"message":"404 Not Found"}`}
	client := newTestClient(t, rec)

	_, err := client.GetReleaseByTag(context.Background(), "u", "p", "v9.9.9")

	if !errors.Is(err, domainplugin.ErrReleaseAssetNotFound) {
		t.Fatalf("GetReleaseByTag err = %v, want ErrReleaseAssetNotFound", err)
	}
}

// GitLab spells its throttling headers without the X- prefix GitHub uses and answers 429 rather
// than 403, so GitHub's detection matches nothing here. Without this mapping a rate limit surfaces
// as an opaque API error and the user goes looking for a fault in their repository.
func TestRateLimitIsReportedAsARateLimit(t *testing.T) {
	rec := &requestRecorder{
		status:  http.StatusTooManyRequests,
		body:    "Retry later",
		headers: map[string]string{"RateLimit-Remaining": "0", "RateLimit-Reset": "1800000000"},
	}
	client := newTestClient(t, rec)

	_, err := client.ListPublishedReleases(context.Background(), "u", "p")

	if !errors.Is(err, domainplugin.ErrGitHubRateLimitExceeded) {
		t.Fatalf("ListPublishedReleases err = %v, want the rate limit error", err)
	}
}

// A 429 that carries no usable headers is still a rate limit.
func TestRateLimitWithoutHeadersIsStillARateLimit(t *testing.T) {
	rec := &requestRecorder{status: http.StatusTooManyRequests, body: "slow down"}
	client := newTestClient(t, rec)

	_, err := client.ListPublishedReleases(context.Background(), "u", "p")

	if !errors.Is(err, domainplugin.ErrGitHubRateLimitExceeded) {
		t.Fatalf("ListPublishedReleases err = %v, want the rate limit error", err)
	}
}

// The downgrade policy has to be attached to the client, not merely defined somewhere.
func TestClientInstallsTheRedirectPolicy(t *testing.T) {
	client := NewClient()

	if client.httpClient.CheckRedirect == nil {
		t.Fatal("the GitLab client follows redirects with no policy at all")
	}
}
