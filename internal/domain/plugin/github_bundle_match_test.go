package plugin_test

import (
	"errors"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func TestVerifyStagedIdentityAcceptsMatchingID(t *testing.T) {
	repo := domainplugin.Manifest{ID: "com.example.demo", Version: "1.0.0"}
	staged := domainplugin.Manifest{ID: " com.example.demo ", Version: "1.0.1"}
	if err := domainplugin.VerifyStagedIdentity(repo, staged); err != nil {
		t.Fatalf("expected matching ids to pass, got %v", err)
	}
}

func TestVerifyStagedIdentityRejectsDifferentID(t *testing.T) {
	repo := domainplugin.Manifest{ID: "com.example.demo"}
	staged := domainplugin.Manifest{ID: "com.attacker.other"}

	err := domainplugin.VerifyStagedIdentity(repo, staged)
	if !errors.Is(err, domainplugin.ErrBundleIdentityMismatch) {
		t.Fatalf("expected ErrBundleIdentityMismatch, got %v", err)
	}
	for _, want := range []string{"com.example.demo", "com.attacker.other"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to name %q, got %q", want, err.Error())
		}
	}
}

func TestStagedVersionMatchesRepo(t *testing.T) {
	cases := []struct {
		name       string
		repo       string
		staged     string
		wantEquals bool
	}{
		{name: "equal", repo: "1.0.4", staged: "1.0.4", wantEquals: true},
		{name: "surrounding space ignored", repo: "1.0.4", staged: " 1.0.4", wantEquals: true},
		{name: "different", repo: "1.0.4", staged: "1.0.5", wantEquals: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domainplugin.StagedVersionMatchesRepo(
				domainplugin.Manifest{Version: tc.repo},
				domainplugin.Manifest{Version: tc.staged},
			)
			if got != tc.wantEquals {
				t.Fatalf("StagedVersionMatchesRepo() = %v, want %v", got, tc.wantEquals)
			}
		})
	}
}
