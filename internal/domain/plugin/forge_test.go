package plugin

import (
	"errors"
	"testing"
)

func TestParseRepoRefResolvesTheForgeFromTheHost(t *testing.T) {
	cases := []struct {
		name  string
		url   string
		forge Forge
		owner string
		repo  string
	}{
		{"github", "https://github.com/teoritty/xQuakShell", ForgeGitHub, "teoritty", "xQuakShell"},
		{"gitlab", "https://gitlab.com/teoritty/xQuakShell", ForgeGitLab, "teoritty", "xQuakShell"},
		{"a bare pair still means the default forge", "teoritty/xQuakShell", DefaultForge, "teoritty", "xQuakShell"},
		{"a bare gitlab path keeps its host", "gitlab.com/teoritty/xQuakShell", ForgeGitLab, "teoritty", "xQuakShell"},
		{"a trailing .git is not part of the name", "https://gitlab.com/teoritty/xQuakShell.git", ForgeGitLab, "teoritty", "xQuakShell"},
		{"host case is not significant", "https://GitLab.com/teoritty/xQuakShell", ForgeGitLab, "teoritty", "xQuakShell"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := ParseRepoRef(tc.url)
			if err != nil {
				t.Fatalf("ParseRepoRef(%q) err = %v, want nil", tc.url, err)
			}
			if ref.Forge != tc.forge {
				t.Errorf("forge = %q, want %q; the host is what selects the API to speak", ref.Forge, tc.forge)
			}
			if ref.Owner != tc.owner || ref.Repo != tc.repo {
				t.Errorf("owner/repo = %q/%q, want %q/%q", ref.Owner, ref.Repo, tc.owner, tc.repo)
			}
		})
	}
}

// A GitLab namespace is any number of segments. Truncating it to the first two - which is the
// correct rule for GitHub and the one this code started with - addresses a project that does not
// exist, and the install fails with a 404 that names the wrong path.
func TestParseRepoRefKeepsAGitLabSubgroupNamespaceWhole(t *testing.T) {
	ref, err := ParseRepoRef("https://gitlab.com/group/subgroup/plugin")
	if err != nil {
		t.Fatalf("ParseRepoRef err = %v, want nil", err)
	}

	if ref.Owner != "group/subgroup" {
		t.Errorf("owner = %q, want %q; a GitLab subgroup is part of the namespace", ref.Owner, "group/subgroup")
	}
	if ref.Repo != "plugin" {
		t.Errorf("repo = %q, want %q", ref.Repo, "plugin")
	}
	if got := ref.ProjectPath(); got != "group/subgroup/plugin" {
		t.Errorf("ProjectPath() = %q, want %q", got, "group/subgroup/plugin")
	}
}

// GitLab inserts "/-/" before every in-project route, and it is the only thing separating a
// subgroup from a page: without the cut, /-/releases becomes two more namespace segments.
func TestParseRepoRefStopsAtTheGitLabRouteSeparator(t *testing.T) {
	ref, err := ParseRepoRef("https://gitlab.com/group/plugin/-/releases/v1.0.0")
	if err != nil {
		t.Fatalf("ParseRepoRef err = %v, want nil", err)
	}

	if ref.Owner != "group" || ref.Repo != "plugin" {
		t.Errorf("owner/repo = %q/%q, want group/plugin", ref.Owner, ref.Repo)
	}
}

// GitHub's namespace is exactly one segment, so anything past owner/repo is a page in the
// repository and must not be mistaken for a deeper namespace the way a GitLab path would be.
func TestParseRepoRefDropsGitHubPagesAfterTheRepo(t *testing.T) {
	ref, err := ParseRepoRef("https://github.com/teoritty/xQuakShell/tree/main/docs")
	if err != nil {
		t.Fatalf("ParseRepoRef err = %v, want nil", err)
	}

	if ref.Owner != "teoritty" || ref.Repo != "xQuakShell" {
		t.Errorf("owner/repo = %q/%q, want teoritty/xQuakShell", ref.Owner, ref.Repo)
	}
}

func TestParseRepoRefRejectsAnUnsupportedHost(t *testing.T) {
	_, err := ParseRepoRef("https://bitbucket.org/team/plugin")

	if !errors.Is(err, ErrInvalidRepositoryURL) {
		t.Fatalf("ParseRepoRef err = %v, want ErrInvalidRepositoryURL", err)
	}
}

func TestParseRepoRefRejectsAPathWithNoProject(t *testing.T) {
	if _, err := ParseRepoRef("https://gitlab.com/teoritty"); !errors.Is(err, ErrInvalidRepositoryURL) {
		t.Fatalf("ParseRepoRef of a namespace with no project = %v, want ErrInvalidRepositoryURL", err)
	}
}

// Validate is the gate the repository list runs on user input; it has to accept both forges and
// keep rejecting everything else, including plain http on a host that IS supported.
func TestValidateAcceptsBothForgesAndNothingElse(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"https://github.com/teoritty/xQuakShell", false},
		{"https://gitlab.com/teoritty/xQuakShell", false},
		{"https://gitlab.com/group/subgroup/plugin", false},
		{"https://bitbucket.org/team/plugin", true},
		{"https://gitlab.com.evil.example/team/plugin", true},
		{"http://gitlab.com/teoritty/xQuakShell", true},
		{"", true},
	}

	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			repo := &GitHubRepository{URL: tc.url}
			err := repo.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate(%q) = nil, want an error", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate(%q) = %v, want nil", tc.url, err)
			}
		})
	}
}

func TestRepositoryReportsItsForge(t *testing.T) {
	gitlabRepo := &GitHubRepository{URL: "https://gitlab.com/teoritty/plugin"}
	if got := gitlabRepo.Forge(); got != ForgeGitLab {
		t.Errorf("Forge() = %q, want %q", got, ForgeGitLab)
	}

	githubRepo := &GitHubRepository{URL: "https://github.com/teoritty/plugin"}
	if got := githubRepo.Forge(); got != ForgeGitHub {
		t.Errorf("Forge() = %q, want %q", got, ForgeGitHub)
	}
}

// Provenance has to record which forge a plugin came from, or an installed plugin cannot be traced
// back to the release it was built from once both platforms are in play.
func TestInstallMetaSourceFollowsTheForge(t *testing.T) {
	if got := InstallMetaSourceForForge(ForgeGitLab); got != InstallMetaSourceGitLab {
		t.Errorf("InstallMetaSourceForForge(gitlab) = %q, want %q", got, InstallMetaSourceGitLab)
	}
	if got := InstallMetaSourceForForge(ForgeGitHub); got != InstallMetaSourceGitHub {
		t.Errorf("InstallMetaSourceForForge(github) = %q, want %q", got, InstallMetaSourceGitHub)
	}
}

// NormalizeURL is what produces the key repositories are stored under. A bare pair has to keep
// normalizing to the default forge, because every repository registered before forges existed was
// stored that way and re-pointing them at another host loses them.
func TestNormalizeURLKeepsStoredKeysStable(t *testing.T) {
	got, err := NormalizeURL("teoritty/xQuakShell")
	if err != nil {
		t.Fatalf("NormalizeURL err = %v, want nil", err)
	}
	if got != "https://github.com/teoritty/xQuakShell" {
		t.Errorf("NormalizeURL(bare pair) = %q, want the default forge's URL", got)
	}

	got, err = NormalizeURL("https://gitlab.com/teoritty/xQuakShell/")
	if err != nil {
		t.Fatalf("NormalizeURL err = %v, want nil", err)
	}
	if got != "https://gitlab.com/teoritty/xQuakShell" {
		t.Errorf("NormalizeURL(gitlab) = %q, want the trailing slash gone and the host kept", got)
	}
}
