package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

const acceptJSON = "application/json"

// Release is the subset of the GitLab releases API response this client reads.
type Release struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// ReleasedAt is GitLab's only publication timestamp; there is no separate created/published
	// pair as on GitHub.
	ReleasedAt string `json:"released_at"`
	// UpcomingRelease is GitLab's nearest equivalent to a pre-release: it marks a release whose
	// released_at is still in the future. GitLab has no draft flag at all, so a release is either
	// upcoming or published and nothing is hidden from this listing.
	UpcomingRelease bool `json:"upcoming_release"`
	Links           struct {
		Self string `json:"self"`
	} `json:"_links"`
	Assets struct {
		Links []Asset `json:"links"`
	} `json:"assets"`
}

// Asset is one file published with a GitLab release.
//
// GitLab release assets are link records, not uploads: the publisher supplies a name and a URL that
// may point anywhere, including at a job artifact or an external host. DirectAssetURL is GitLab's
// stable permalink for the link and is preferred when present, because URL can be a redirect the
// publisher rewrites later.
type Asset struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	DirectAssetURL string `json:"direct_asset_url"`
}

// DownloadURL is the URL an installer should fetch this asset from.
func (a Asset) DownloadURL() string {
	if a.DirectAssetURL != "" {
		return a.DirectAssetURL
	}
	return a.URL
}

// ListPublishedReleases returns published GitLab releases, newest first.
func (c *Client) ListPublishedReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	listURL := fmt.Sprintf("%s/projects/%s/releases?per_page=30", c.baseURL, projectID(owner, repo))

	resp, err := c.get(ctx, listURL, acceptJSON)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API error (%d): %s", resp.StatusCode, string(body))
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// GetReleaseByTag fetches a release by tag name.
func (c *Client) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*Release, error) {
	releaseURL := fmt.Sprintf("%s/projects/%s/releases/%s",
		c.baseURL, projectID(owner, repo), url.PathEscape(tag))

	resp, err := c.get(ctx, releaseURL, acceptJSON)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: release %s", domainplugin.ErrReleaseAssetNotFound, tag)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API error (%d): %s", resp.StatusCode, string(body))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

// GetLatestRelease returns the newest release that is actually out.
//
// GitLab publishes no /releases/latest, so this is the list endpoint plus the filter GitHub applies
// server-side: an upcoming release is dated in the future and offering it as the latest would tell
// a user to install something that is not published yet.
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) (*Release, error) {
	releases, err := c.ListPublishedReleases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	for i := range releases {
		if !releases[i].UpcomingRelease {
			return &releases[i], nil
		}
	}
	return nil, fmt.Errorf("%w in %s/%s", domainplugin.ErrNoReleases, owner, repo)
}

// checkRateLimit maps GitLab's throttling response onto the same domain error GitHub's does, so the
// UI keeps one message for "the forge is rate limiting you".
//
// GitLab spells the headers without the X- prefix GitHub uses and answers 429 rather than 403, so
// none of GitHub's detection fires here; both spellings are read because self-managed instances
// behind a proxy have been seen emitting the X- form.
func checkRateLimit(resp *http.Response) error {
	remaining := resp.Header.Get("RateLimit-Remaining")
	if remaining == "" {
		remaining = resp.Header.Get("X-RateLimit-Remaining")
	}
	if remaining != "0" && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}

	reset := resp.Header.Get("RateLimit-Reset")
	if reset == "" {
		reset = resp.Header.Get("X-RateLimit-Reset")
	}
	if n, err := strconv.ParseInt(reset, 10, 64); err == nil {
		return fmt.Errorf("%w: reset at %s", domainplugin.ErrGitHubRateLimitExceeded, time.Unix(n, 0).Format(time.RFC3339))
	}
	// A 429 with no usable reset header is still a rate limit, and reporting it as a generic API
	// error would send the user looking for a fault in their repository instead of waiting.
	return fmt.Errorf("%w: reset at %s", domainplugin.ErrGitHubRateLimitExceeded, reset)
}
