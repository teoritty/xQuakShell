package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
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
	return &Client{
		httpClient: &http.Client{Timeout: DefaultTimeout},
		baseURL:    APIBaseURL,
	}
}

// Release represents a GitHub release.
type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
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
func (c *Client) GetFileContent(ctx context.Context, owner, repo, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.baseURL, owner, repo, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, owner, repo)
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
		return nil, fmt.Errorf("%w: %s/%s", domainplugin.ErrRepositoryNotFound, owner, repo)
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
	if raw == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return t.Format(time.RFC3339)
}

// TotalDownloadCount sums asset download counts.
func TotalDownloadCount(assets []Asset) int {
	total := 0
	for _, a := range assets {
		total += a.DownloadCount
	}
	return total
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
	out := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := strings.TrimPrefix(parts[0], "*")
		out[parts[len(parts)-1]] = hash
	}
	return out
}
