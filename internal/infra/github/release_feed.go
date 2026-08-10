package github

import (
	"context"
	"fmt"

	"xquakshell/internal/domain"
)

// ReleaseFeed adapts the GitHub client to domain.ReleaseFeed for the application's own repository,
// so the app can tell a user that the release they are running has been superseded (ADR-017).
//
// It is deliberately read-only and anonymous: one unauthenticated GET of a public endpoint, no
// token, and nothing about the installation travels with it beyond what any HTTP request carries.
type ReleaseFeed struct {
	client *Client
	owner  string
	repo   string
}

// NewReleaseFeed builds the feed for one repository. The owner and repo are passed in rather than
// baked in so a fork does not silently check the upstream project's releases.
func NewReleaseFeed(client *Client, owner, repo string) *ReleaseFeed {
	return &ReleaseFeed{client: client, owner: owner, repo: repo}
}

// LatestStableRelease returns the newest published, non-draft, non-pre-release release.
//
// GetLatestRelease falls back to the releases list when /releases/latest 404s, which happens while
// a repository has only ever published pre-releases — and that fallback can hand back an rc. The
// filter below is what keeps an rc from ever being presented as an upgrade; without it a user on a
// stable build would be told to move to a release candidate.
func (f *ReleaseFeed) LatestStableRelease(ctx context.Context) (domain.ReleaseInfo, error) {
	release, err := f.client.GetLatestRelease(ctx, f.owner, f.repo)
	if err != nil {
		return domain.ReleaseInfo{}, fmt.Errorf("latest release for %s/%s: %w", f.owner, f.repo, err)
	}
	if release == nil || release.Draft || release.Prerelease {
		return domain.ReleaseInfo{}, domain.ErrNoStableRelease
	}
	return domain.ReleaseInfo{Version: release.TagName, URL: release.HTMLURL}, nil
}
