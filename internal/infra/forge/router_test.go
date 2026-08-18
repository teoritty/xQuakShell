package forge

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// namedClient records that it was the one asked, so a test can assert which forge answered rather
// than only that something did.
type namedClient struct {
	name   string
	called []string
}

func (c *namedClient) GetFileContent(_ context.Context, _ domainplugin.RepoRef, _, _ string) ([]byte, error) {
	c.called = append(c.called, "GetFileContent")
	return []byte(c.name), nil
}

func (c *namedClient) GetLatestRelease(_ context.Context, _ domainplugin.RepoRef) (*domainplugin.GitHubRelease, error) {
	c.called = append(c.called, "GetLatestRelease")
	return &domainplugin.GitHubRelease{TagName: c.name}, nil
}

func (c *namedClient) ListPublishedReleases(_ context.Context, _ domainplugin.RepoRef) ([]domainplugin.GitHubRelease, error) {
	c.called = append(c.called, "ListPublishedReleases")
	return []domainplugin.GitHubRelease{{TagName: c.name}}, nil
}

func (c *namedClient) GetReleaseByTag(_ context.Context, _ domainplugin.RepoRef, _ string) (*domainplugin.GitHubRelease, error) {
	c.called = append(c.called, "GetReleaseByTag")
	return &domainplugin.GitHubRelease{TagName: c.name}, nil
}

func (c *namedClient) DownloadAsset(_ context.Context, _ string) (io.ReadCloser, error) {
	c.called = append(c.called, "DownloadAsset")
	return io.NopCloser(strings.NewReader(c.name)), nil
}

func newTestRouter() (*Router, *namedClient, *namedClient) {
	gh := &namedClient{name: "github"}
	gl := &namedClient{name: "gitlab"}
	return NewRouterWithClients(map[domainplugin.Forge]Client{
		domainplugin.ForgeGitHub: gh,
		domainplugin.ForgeGitLab: gl,
	}), gh, gl
}

// The whole point of the router: the ref decides, and a GitLab repository must never have its
// metadata read from GitHub. Getting this wrong does not fail loudly - api.github.com answers 404
// for a project that only exists on GitLab - so the failure reads as "your repository is missing".
func TestRouterDispatchesOnTheRefsForge(t *testing.T) {
	cases := []struct {
		forge domainplugin.Forge
		want  string
	}{
		{domainplugin.ForgeGitHub, "github"},
		{domainplugin.ForgeGitLab, "gitlab"},
	}

	for _, tc := range cases {
		t.Run(string(tc.forge), func(t *testing.T) {
			router, gh, gl := newTestRouter()
			ref := domainplugin.RepoRef{Forge: tc.forge, Owner: "o", Repo: "r"}

			got, err := router.GetFileContent(context.Background(), ref, "xqsp.json", "")
			if err != nil {
				t.Fatalf("GetFileContent err = %v, want nil", err)
			}

			if string(got) != tc.want {
				t.Errorf("answered by %q, want %q", got, tc.want)
			}
			other := gl
			if tc.forge == domainplugin.ForgeGitLab {
				other = gh
			}
			if len(other.called) != 0 {
				t.Errorf("the other forge was called %v; a ref must reach exactly one platform", other.called)
			}
		})
	}
}

func TestRouterRoutesEveryReleaseCall(t *testing.T) {
	router, _, gl := newTestRouter()
	ref := domainplugin.RepoRef{Forge: domainplugin.ForgeGitLab, Owner: "o", Repo: "r"}

	if _, err := router.GetLatestRelease(context.Background(), ref); err != nil {
		t.Fatalf("GetLatestRelease err = %v", err)
	}
	if _, err := router.ListPublishedReleases(context.Background(), ref); err != nil {
		t.Fatalf("ListPublishedReleases err = %v", err)
	}
	if _, err := router.GetReleaseByTag(context.Background(), ref, "v1"); err != nil {
		t.Fatalf("GetReleaseByTag err = %v", err)
	}
	if _, err := router.DownloadAsset(context.Background(), domainplugin.ForgeGitLab, "https://x/y"); err != nil {
		t.Fatalf("DownloadAsset err = %v", err)
	}

	want := []string{"GetLatestRelease", "ListPublishedReleases", "GetReleaseByTag", "DownloadAsset"}
	if strings.Join(gl.called, ",") != strings.Join(want, ",") {
		t.Errorf("gitlab client saw %v, want %v", gl.called, want)
	}
}

// An unknown forge is refused rather than defaulted. A default would send an unauthenticated
// request for someone's repository path to a host they never named.
func TestRouterRefusesAnUnknownForge(t *testing.T) {
	router, gh, gl := newTestRouter()
	ref := domainplugin.RepoRef{Forge: domainplugin.Forge("bitbucket"), Owner: "o", Repo: "r"}

	_, err := router.GetFileContent(context.Background(), ref, "xqsp.json", "")

	if !errors.Is(err, ErrUnsupportedForge) {
		t.Fatalf("GetFileContent err = %v, want ErrUnsupportedForge", err)
	}
	if len(gh.called)+len(gl.called) != 0 {
		t.Error("an unknown forge still reached a client")
	}
}

// The zero Forge is what a caller that forgot to set it produces. It must not silently mean GitHub.
func TestRouterRefusesTheZeroForge(t *testing.T) {
	router, _, _ := newTestRouter()

	_, err := router.GetLatestRelease(context.Background(), domainplugin.RepoRef{Owner: "o", Repo: "r"})

	if !errors.Is(err, ErrUnsupportedForge) {
		t.Fatalf("GetLatestRelease with no forge = %v, want ErrUnsupportedForge", err)
	}
}

// The production router has to actually carry both adapters; a router wired with one is the bug
// this whole change exists to prevent.
func TestNewRouterCarriesBothForges(t *testing.T) {
	router := NewRouter()

	for _, forge := range []domainplugin.Forge{domainplugin.ForgeGitHub, domainplugin.ForgeGitLab} {
		if _, err := router.clientFor(forge); err != nil {
			t.Errorf("clientFor(%q) = %v, want a client", forge, err)
		}
	}
}
