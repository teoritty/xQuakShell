package domain_test

import (
	"testing"

	"xquakshell/internal/domain"
)

func TestIsNewerRelease(t *testing.T) {
	cases := []struct {
		candidate string
		current   string
		want      bool
		why       string
	}{
		{"1.0.1", "1.0.0", true, "a patch release is an upgrade"},
		{"1.1.0", "1.0.9", true, "minor outranks patch"},
		{"2.0.0", "1.9.9", true, "major outranks minor"},
		{"v1.0.1", "1.0.0", true, "a tag's leading v is not part of the version"},
		{"1.0.1", "v1.0.0", true, "the running version may carry it too"},
		{"1.0.0", "1.0.0", false, "the same release is not an upgrade"},
		{"1.0.0", "1.0.1", false, "an older release is never offered"},
		{"1.0.0", "1.0.0-rc.3", true, "a final release supersedes the candidates that led to it"},
		{"1.0.0-rc.4", "1.0.0-rc.3", false, "a pre-release is never something to upgrade to"},
		{"1.0.0-rc.1", "1.0.0", false, "and never supersedes the final release"},
		{"1.0.1+build.7", "1.0.0", true, "build metadata does not affect precedence"},
		{"", "1.0.0", false, "an empty version is not an upgrade"},
		{"not-a-version", "1.0.0", false, "an unparseable version must not nag the user"},
		{"1.0", "1.0.0", false, "a two-part version is not the grammar releases use"},
		{"1.0.1", "dev", false, "an unrecognised running version means we cannot know"},
	}

	for _, tc := range cases {
		if got := domain.IsNewerRelease(tc.candidate, tc.current); got != tc.want {
			t.Errorf("IsNewerRelease(%q, %q) = %v, want %v; %s", tc.candidate, tc.current, got, tc.want, tc.why)
		}
	}
}

// The setting was added after vaults existed. Every vault written before it decodes the field as
// absent, and absent has to mean on — otherwise the check silently stays off for exactly the people
// who never chose to turn it off, and the support policy has no way of reaching them.
func TestStartupCheckDefaultsToOnWhenNeverConfigured(t *testing.T) {
	var never domain.UpdateSettings
	if !never.StartupCheckEnabled() {
		t.Error("an unconfigured update setting must resolve to on")
	}

	off := false
	if (domain.UpdateSettings{CheckOnStartup: &off}).StartupCheckEnabled() {
		t.Error("an explicit false must turn the check off")
	}

	on := true
	if !(domain.UpdateSettings{CheckOnStartup: &on}).StartupCheckEnabled() {
		t.Error("an explicit true must leave the check on")
	}
}
