package gitlab

import (
	"context"
	"fmt"

	"xquakshell/internal/domain"
)

// ReleaseFeed adapts the GitLab client to domain.ReleaseFeed for the application's own repository,
// so the app can tell a user that the release they are running has been superseded (ADR-017).
//
// It is deliberately read-only and anonymous: one unauthenticated GET of a public endpoint, no
// token, and nothing about the installation travels with it beyond what any HTTP request carries.
type ReleaseFeed struct {
	client *Client
	owner  string
	repo   string
}

// NewReleaseFeed builds the feed for one project. The owner and repo are passed in rather than
// baked in so a fork does not silently check the upstream project's releases.
func NewReleaseFeed(client *Client, owner, repo string) *ReleaseFeed {
	return &ReleaseFeed{client: client, owner: owner, repo: repo}
}

// LatestStableRelease returns the newest published, non-upcoming release.
//
// GitLab has no pre-release flag, so there is no rc to filter out the way the GitHub feed must:
// a project that ships release candidates as ordinary GitLab releases will have them offered as
// upgrades. Tagging them upcoming is the only signal GitLab gives, and it is honoured here.
func (f *ReleaseFeed) LatestStableRelease(ctx context.Context) (domain.ReleaseInfo, error) {
	release, err := f.client.GetLatestRelease(ctx, f.owner, f.repo)
	if err != nil {
		return domain.ReleaseInfo{}, fmt.Errorf("latest release for %s/%s: %w", f.owner, f.repo, err)
	}
	if release == nil || release.UpcomingRelease {
		return domain.ReleaseInfo{}, domain.ErrNoStableRelease
	}
	return domain.ReleaseInfo{Version: release.TagName, URL: release.Links.Self}, nil
}
