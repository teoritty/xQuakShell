package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

const (
	APIBaseURL     = "https://api.github.com"
	DefaultTimeout = 30 * time.Second
	userAgent      = "xQuakShell"
)

// Client implements unauthenticated GitHub REST API access.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new GitHub API client.
func NewClient() *Client {
	return NewClientWithBaseURL(APIBaseURL)
}

// NewClientWithBaseURL creates a client for the given GitHub API base URL.
func NewClientWithBaseURL(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: DefaultTimeout},
		baseURL:    baseURL,
	}
}

// Release represents a GitHub release.
type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	Assets      []Asset `json:"assets"`
}

// Asset represents a release asset.
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	DownloadCount      int    `json:"download_count"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GetFileContent fetches a file from the repository (raw content).
// ref is an optional branch or tag name; empty uses the default branch.
func (c *Client) GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	fileURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.baseURL, owner, repo, path)
	if ref != "" {
		fileURL += "?ref=" + url.QueryEscape(ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3.raw")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkRateLimit(resp); err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, fmt.Errorf("file not found: %s", path)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("repository is private or requires authentication")
	case http.StatusOK:
		return io.ReadAll(resp.Body)
	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(body))
	}
}

// GetLatestRelease fetches the latest release for a repository.
// GitHub's /releases/latest excludes pre-releases, so this falls back to the
// releases list when only pre-releases are published.
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) (*Release, error) {
	release, err := c.fetchReleaseURL(ctx, fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, owner, repo))
	if err == nil {
		return release, nil
	}
	if !errors.Is(err, errReleaseEndpointNotFound) {
		return nil, err
	}

	releases, err := c.ListPublishedReleases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("%w in %s/%s", domainplugin.ErrNoReleases, owner, repo)
	}
	return &releases[0], nil
}

var errReleaseEndpointNotFound = errors.New("release endpoint not found")

func (c *Client) fetchReleaseURL(ctx context.Context, url string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkRateLimit(resp); err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, errReleaseEndpointNotFound
	case http.StatusOK:
		var release Release
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return nil, err
		}
		return &release, nil
	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(body))
	}
}

// ListPublishedReleases returns published GitHub releases, newest first.
func (c *Client) ListPublishedReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	return c.listPublishedReleases(ctx, owner, repo)
}

func (c *Client) listPublishedReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30", c.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkRateLimit(resp); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(body))
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	published := make([]Release, 0, len(releases))
	for _, release := range releases {
		if !release.Draft {
			published = append(published, release)
		}
	}
	return published, nil
}

// GetReleaseByTag fetches a release by tag name.
func (c *Client) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", c.baseURL, owner, repo, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkRateLimit(resp); err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: release %s", domainplugin.ErrReleaseAssetNotFound, tag)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, string(body))
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

// DownloadAsset downloads a release asset from the given URL.
func (c *Client) DownloadAsset(ctx context.Context, downloadURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	return resp.Body, nil
}

func checkRateLimit(resp *http.Response) error {
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	if remaining == "0" {
		resetTime := resp.Header.Get("X-RateLimit-Reset")
		return fmt.Errorf("%w: reset at %s", domainplugin.ErrGitHubRateLimitExceeded, resetTime)
	}
	if resp.StatusCode == http.StatusForbidden {
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if n, err := strconv.ParseInt(reset, 10, 64); err == nil {
				return fmt.Errorf("%w: reset at %s", domainplugin.ErrGitHubRateLimitExceeded, time.Unix(n, 0).Format(time.RFC3339))
			}
		}
	}
	return nil
}

// ParseReleasePublishedAt parses GitHub's published_at timestamp.
func ParseReleasePublishedAt(raw string) string {
	return domainplugin.ParseReleasePublishedAt(raw)
}

// TotalDownloadCount sums asset download counts.
func TotalDownloadCount(assets []Asset) int {
	domainAssets := make([]domainplugin.GitHubReleaseAsset, len(assets))
	for i := range assets {
		domainAssets[i] = domainplugin.GitHubReleaseAsset{
			Name:          assets[i].Name,
			DownloadCount: assets[i].DownloadCount,
		}
	}
	return domainplugin.TotalReleaseDownloadCount(domainAssets)
}

// FindAsset returns the asset with the given name.
func FindAsset(assets []Asset, name string) (*Asset, error) {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", domainplugin.ErrReleaseAssetNotFound, name)
}

// ParseChecksumsFile parses SHA256SUMS content into asset name -> checksum map.
func ParseChecksumsFile(content string) map[string]string {
	return domainplugin.ParseChecksumsFile(content)
}
