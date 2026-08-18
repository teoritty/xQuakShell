package plugin

import (
	"fmt"
	"net/url"
	"strings"
)

// Forge names the hosting platform a plugin repository lives on.
//
// It exists because the two platforms disagree about everything the installer needs: the REST
// shape, how a project is addressed (owner/repo path segments vs one URL-encoded id), and what a
// release asset even is (an uploaded file vs a link record). Everything above infra is written
// against this value and never against a host name, so adding a third forge is a new adapter and a
// new constant rather than a sweep through the usecase layer.
type Forge string

const (
	ForgeGitHub Forge = "github"
	ForgeGitLab Forge = "gitlab"
)

// DefaultForge is what a bare "owner/repo" is taken to mean.
//
// It stays GitHub for a reason that outlives the project's own move to GitLab: every repository
// registered before forges existed was stored as a bare pair and normalized against github.com, so
// changing this silently repoints all of them at a host where they do not exist.
const DefaultForge = ForgeGitHub

// forgeHosts maps the public host of each supported platform to its forge. Self-hosted instances
// are deliberately absent: a GitLab install on any other domain is indistinguishable from an
// arbitrary URL, and guessing wrong sends an unauthenticated request somewhere the user never named.
var forgeHosts = map[string]Forge{
	"github.com": ForgeGitHub,
	"gitlab.com": ForgeGitLab,
}

// ForgeForHost reports which forge serves a host, and whether it is one this build supports.
func ForgeForHost(host string) (Forge, bool) {
	forge, ok := forgeHosts[strings.ToLower(strings.TrimSpace(host))]
	return forge, ok
}

// SupportedForgeHosts lists the accepted hosts, for error messages that name what would work.
func SupportedForgeHosts() []string {
	return []string{"github.com", "gitlab.com"}
}

// RepoRef identifies one repository on one forge.
//
// Owner is a single account or organisation on GitHub, but on GitLab it is the whole namespace and
// may contain slashes: group/subgroup/project is an ordinary GitLab layout, and truncating it to
// the first two segments addresses a project that does not exist.
type RepoRef struct {
	Forge Forge
	Owner string
	Repo  string
}

// ProjectPath is the namespace and project joined the way both forges spell it in a URL.
func (r RepoRef) ProjectPath() string {
	return r.Owner + "/" + r.Repo
}

// IsZero reports whether the ref names nothing.
func (r RepoRef) IsZero() bool {
	return r.Owner == "" && r.Repo == ""
}

// ParseRepoRef resolves a repository URL to the forge that serves it and the project on it.
func ParseRepoRef(repoURL string) (RepoRef, error) {
	normalized, err := NormalizeURL(repoURL)
	if err != nil {
		return RepoRef{}, err
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return RepoRef{}, err
	}

	forge, ok := ForgeForHost(parsed.Host)
	if !ok {
		return RepoRef{}, fmt.Errorf("%w: %s is not a supported host (%s)",
			ErrInvalidRepositoryURL, parsed.Host, strings.Join(SupportedForgeHosts(), ", "))
	}

	owner, repo, err := splitProjectPath(forge, parsed.Path)
	if err != nil {
		return RepoRef{}, err
	}
	return RepoRef{Forge: forge, Owner: owner, Repo: repo}, nil
}

// splitProjectPath cuts a URL path into namespace and project.
//
// The two forges need different rules. GitHub's namespace is exactly one segment, so anything past
// owner/repo is a page within the repository (/tree, /releases) and is dropped. GitLab's namespace
// is any number of segments and is terminated instead by the "/-/" separator GitLab inserts before
// every in-project route, which is the only thing distinguishing a subgroup from a page.
func splitProjectPath(forge Forge, rawPath string) (owner, repo string, err error) {
	path := strings.Trim(rawPath, "/")
	if idx := strings.Index(path, "/-/"); idx >= 0 {
		path = path[:idx]
	}

	parts := make([]string, 0, 4)
	for _, part := range strings.Split(path, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) < 2 {
		return "", "", fmt.Errorf("%w: invalid path format", ErrInvalidRepositoryURL)
	}

	if forge == ForgeGitHub {
		parts = parts[:2]
	}
	repo = strings.TrimSuffix(parts[len(parts)-1], ".git")
	if repo == "" {
		return "", "", fmt.Errorf("%w: invalid path format", ErrInvalidRepositoryURL)
	}
	return strings.Join(parts[:len(parts)-1], "/"), repo, nil
}
