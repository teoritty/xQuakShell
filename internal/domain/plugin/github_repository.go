package plugin

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type GitHubRepository struct {
	URL           string     `json:"url"`
	Owner         string     `json:"owner"`
	Repo          string     `json:"repo"`
	DisplayName   string     `json:"displayName,omitempty"`
	AddedAt       time.Time  `json:"addedAt"`
	LastFetchedAt *time.Time `json:"lastFetchedAt,omitempty"`
	Trusted       bool       `json:"trusted"`
}

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

	if _, ok := ForgeForHost(parsed.Host); !ok {
		return fmt.Errorf("%w: only %s are supported", ErrInvalidRepositoryURL,
			strings.Join(SupportedForgeHosts(), " and "))
	}

	path := parsed.Path
	if path == "" || path == "/" {
		return fmt.Errorf("%w: missing owner/repo", ErrInvalidRepositoryURL)
	}

	return nil
}

func NormalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimRight(rawURL, "/")

	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "https://" + prefixHostForBareRef(rawURL)
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

// prefixHostForBareRef supplies the host for input that carried no scheme.
//
// "gitlab.com/group/proj" already names its host and only needs the scheme; "group/proj" names none
// and gets DefaultForge's, which is what every repository registered before forge support was added
// meant when it was stored.
func prefixHostForBareRef(rawURL string) string {
	for host := range forgeHosts {
		if rawURL == host || strings.HasPrefix(rawURL, host+"/") {
			return rawURL
		}
	}
	return defaultForgeHost() + "/" + rawURL
}

func defaultForgeHost() string {
	for host, forge := range forgeHosts {
		if forge == DefaultForge {
			return host
		}
	}
	return "github.com"
}

// Forge reports which platform hosts this repository. An unparseable URL reports DefaultForge;
// Validate is what rejects it, and reporting an error here would force every caller that only
// wants to label a row in the UI to handle one.
func (r *GitHubRepository) Forge() Forge {
	ref, err := ParseRepoRef(r.URL)
	if err != nil {
		return DefaultForge
	}
	return ref.Forge
}

// ParseGitHubURL splits a repository URL into its namespace and project.
//
// The name predates GitLab support and is kept because it is what the whole plugin stack calls;
// the parsing itself is forge-aware, so a GitLab subgroup path resolves to the full namespace
// rather than its first segment. Callers that need to know WHICH forge answered use ParseRepoRef.
func ParseGitHubURL(repoURL string) (owner, repo string, err error) {
	ref, err := ParseRepoRef(repoURL)
	if err != nil {
		return "", "", err
	}
	return ref.Owner, ref.Repo, nil
}
