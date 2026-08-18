// Package gitlab implements unauthenticated read access to the GitLab REST API v4, in the shape
// the plugin stack already expects from a forge.
package gitlab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"xquakshell/internal/pkg/httpsafe"
)

const (
	APIBaseURL     = "https://gitlab.com/api/v4"
	DefaultTimeout = 30 * time.Second
	userAgent      = "xQuakShell"

	// defaultRef is what an empty ref means to GitLab.
	//
	// GitHub's contents endpoint treats a missing ref as "the default branch". GitLab's does not:
	// the raw file endpoint wants a ref and answers 404 without one on older instances, and there
	// is no cheap way to learn the default branch name without a second request per file. HEAD is
	// GitLab's own spelling for it and costs nothing.
	defaultRef = "HEAD"
)

// Client implements unauthenticated GitLab REST API access.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a client for gitlab.com.
func NewClient() *Client {
	return NewClientWithBaseURL(APIBaseURL)
}

// NewClientWithBaseURL creates a client for the given GitLab API v4 base URL.
func NewClientWithBaseURL(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout:       DefaultTimeout,
			CheckRedirect: httpsafe.RefuseTransportDowngrade,
		},
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// projectID renders a namespace and project as the URL-encoded id every GitLab project endpoint
// takes. The whole path is encoded, slashes included, because "group/sub/proj" is one id and not
// three path segments — the single largest difference from GitHub's owner/repo addressing.
func projectID(owner, repo string) string {
	return url.PathEscape(owner + "/" + repo)
}

// GetFileContent fetches a file from the repository (raw content).
// ref is an optional branch or tag name; empty means the default branch.
func (c *Client) GetFileContent(ctx context.Context, owner, repo, path, ref string) ([]byte, error) {
	if ref == "" {
		ref = defaultRef
	}
	fileURL := fmt.Sprintf("%s/projects/%s/repository/files/%s/raw?ref=%s",
		c.baseURL, projectID(owner, repo), url.PathEscape(path), url.QueryEscape(ref))

	resp, err := c.get(ctx, fileURL, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, fmt.Errorf("file not found: %s", path)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("repository is private or requires authentication")
	case http.StatusOK:
		return io.ReadAll(resp.Body)
	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API error (%d): %s", resp.StatusCode, string(body))
	}
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

// get issues one GET and hands back the still-open response for the caller to classify.
func (c *Client) get(ctx context.Context, requestURL, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if err := checkRateLimit(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}
	return resp, nil
}
