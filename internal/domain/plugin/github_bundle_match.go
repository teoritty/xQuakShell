package plugin

import (
	"fmt"
	"strings"
)

// VerifyStagedIdentity checks that a release bundle carries the plugin the repository's xqsp.json
// advertised. Only the ID is binding, and it is binding for a concrete reason: the ID picks the
// install directory, so a bundle that disagrees would overwrite — or masquerade as — a different
// plugin than the one the user chose to install.
//
// The declared capabilities need no comparison here. Consent is enforced against the staged
// manifest (PreviewInstall and Install both read the tree that is about to be copied), so a
// bundle that asks for more than xqsp.json advertised is refused by the consent gate rather than
// silently granted.
func VerifyStagedIdentity(repo, staged Manifest) error {
	repoID := strings.TrimSpace(repo.ID)
	stagedID := strings.TrimSpace(staged.ID)
	if repoID == stagedID {
		return nil
	}
	return fmt.Errorf("%w: repository declares %q, release bundle contains %q",
		ErrBundleIdentityMismatch, repoID, stagedID)
}

// StagedVersionMatchesRepo reports whether the bundle's version equals the one xqsp.json declared.
//
// It returns a bool rather than an error because a mismatch must not fail the install: the
// repository manifest is fetched from the release tag when it exists there and from the default
// branch otherwise, so installing an older tag legitimately compares an old bundle against the
// current manifest. Refusing that would break a correct install to report a naming discrepancy.
// The caller logs it; the version the user ends up with is the bundle's, which is what the
// install preview already shows.
func StagedVersionMatchesRepo(repo, staged Manifest) bool {
	return strings.TrimSpace(repo.Version) == strings.TrimSpace(staged.Version)
}
