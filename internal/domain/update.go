package domain

import (
	"context"
	"strconv"
	"strings"
)

// UpdateSettings controls the startup check against the project's published releases.
type UpdateSettings struct {
	// CheckOnStartup is a pointer because nil has to mean something different from false.
	// The setting was added after vaults existed, so a plain bool would decode as false in every
	// vault written before it and silently switch the check off for exactly the users who never
	// chose to. nil means "never configured" and resolves to on; false is a deliberate opt-out.
	CheckOnStartup *bool `json:"checkOnStartup,omitempty"`
}

// StartupCheckEnabled resolves the tri-state to the effective behaviour.
func (u UpdateSettings) StartupCheckEnabled() bool {
	return u.CheckOnStartup == nil || *u.CheckOnStartup
}

// ReleaseInfo is one published release, reduced to what deciding "should the user upgrade" needs.
type ReleaseInfo struct {
	Version string
	URL     string
}

// ReleaseFeed reports the project's newest stable release. Only the latest release is supported
// (ADR-017), so a user on anything older has to be able to find out that they are.
type ReleaseFeed interface {
	// LatestStableRelease excludes drafts and pre-releases: an rc is never something a user is
	// told to upgrade to.
	LatestStableRelease(ctx context.Context) (ReleaseInfo, error)
}

// UpdateStatus is the result of one check, and what the UI renders.
type UpdateStatus struct {
	CurrentVersion  string
	LatestVersion   string
	ReleaseURL      string
	UpdateAvailable bool
	// Checked is false when no check has run yet, or when the user turned the check off. It keeps
	// "we know there is no update" distinct from "we never looked", which the UI must not conflate.
	Checked bool
}

// IsNewerRelease reports whether candidate is a release the user on current should upgrade to.
//
// This is a second version comparison in the codebase, and deliberately not plugin.Satisfies: that
// one answers "does this host satisfy a requirement", is caret-shaped (equal major, minor at
// least), and documents that it does not model pre-release ordering. Here the question is strict
// ordering including pre-releases, because the case that actually matters at 1.0 is a user running
// 1.0.0-rc.3 who should be told 1.0.0 exists — and by the caret rule those two are equivalent.
//
// Anything unparseable returns false. A version string this build does not understand is not
// grounds for telling someone their install is out of date.
func IsNewerRelease(candidate, current string) bool {
	newCore, newPre, ok := parseReleaseVersion(candidate)
	if !ok {
		return false
	}
	oldCore, oldPre, ok := parseReleaseVersion(current)
	if !ok {
		return false
	}

	for i := range newCore {
		if newCore[i] != oldCore[i] {
			return newCore[i] > oldCore[i]
		}
	}
	// Same MAJOR.MINOR.PATCH: a final release supersedes the pre-releases that led to it, and
	// nothing supersedes a final release.
	return oldPre != "" && newPre == ""
}

// parseReleaseVersion splits a release version into its numeric core and pre-release suffix,
// tolerating the leading "v" that git tags carry and release payloads often keep.
func parseReleaseVersion(raw string) ([3]int, string, bool) {
	var core [3]int

	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return core, "", false
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i] // build metadata never affects precedence
	}

	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s, pre = s[:i], s[i+1:]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return core, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return core, "", false
		}
		core[i] = n
	}
	return core, pre, true
}
