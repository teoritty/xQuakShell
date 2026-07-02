package plugin

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// GitHubRepository represents a registered GitHub repository containing plugins.
type GitHubRepository struct {
	URL           string     `json:"url"`
	Owner         string     `json:"owner"`
	Repo          string     `json:"repo"`
	DisplayName   string     `json:"displayName,omitempty"`
	AddedAt       time.Time  `json:"addedAt"`
	LastFetchedAt *time.Time `json:"lastFetchedAt,omitempty"`
	Trusted       bool       `json:"trusted"`
}

// Validate checks if the repository URL is valid.
func (r *GitHubRepository) Validate() error {
	if r.URL == "" {
		return ErrInvalidRepositoryURL
	}

	parsed, err := url.Parse(r.URL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRepositoryURL, err)
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("%w: must use HTTPS", ErrInvalidRepositoryURL)
	}

	if parsed.Host != "github.com" {
		return fmt.Errorf("%w: only github.com is supported", ErrInvalidRepositoryURL)
	}

	path := parsed.Path
	if path == "" || path == "/" {
		return fmt.Errorf("%w: missing owner/repo", ErrInvalidRepositoryURL)
	}

	return nil
}

// NormalizeURL ensures consistent URL format.
func NormalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimRight(rawURL, "/")

	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "https://github.com/" + strings.TrimPrefix(rawURL, "github.com/")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	parsed.Scheme = "https"
	parsed.Fragment = ""
	parsed.RawQuery = ""

	return strings.TrimRight(parsed.String(), "/"), nil
}

// ParseGitHubURL extracts owner and repo from URL.
func ParseGitHubURL(repoURL string) (owner, repo string, err error) {
	normalized, err := NormalizeURL(repoURL)
	if err != nil {
		return "", "", err
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", "", err
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("%w: invalid path format", ErrInvalidRepositoryURL)
	}

	return parts[0], parts[1], nil
}
