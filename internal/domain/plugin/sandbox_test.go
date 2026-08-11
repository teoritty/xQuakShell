package plugin_test

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func TestWeakestSandboxModePicksTheProcessWithTheLeastConfinement(t *testing.T) {
	cases := []struct {
		name  string
		modes []domainplugin.SandboxMode
		want  domainplugin.SandboxMode
		why   string
	}{
		{
			name: "nothing running",
			want: "",
			why:  "a plugin with no processes has no mode; reporting one would be a guess about a start nobody asked for",
		},
		{
			name:  "one confined process",
			modes: []domainplugin.SandboxMode{domainplugin.SandboxEnforced},
			want:  domainplugin.SandboxEnforced,
		},
		{
			name:  "one confined, one not",
			modes: []domainplugin.SandboxMode{domainplugin.SandboxEnforced, domainplugin.SandboxUnavailable},
			want:  domainplugin.SandboxUnavailable,
			why:   "reporting the confined one would tell the user their plugin is contained while a process of it is not",
		},
		{
			name:  "order does not matter",
			modes: []domainplugin.SandboxMode{domainplugin.SandboxUnavailable, domainplugin.SandboxEnforced},
			want:  domainplugin.SandboxUnavailable,
		},
		{
			name:  "confined and switched off",
			modes: []domainplugin.SandboxMode{domainplugin.SandboxEnforced, domainplugin.SandboxDisabled},
			want:  domainplugin.SandboxDisabled,
		},
		{
			name:  "switched off beats unavailable",
			modes: []domainplugin.SandboxMode{domainplugin.SandboxDisabled, domainplugin.SandboxUnavailable},
			want:  domainplugin.SandboxUnavailable,
			why:   "neither confines anything, and the platform's inability is the more fundamental answer",
		},
		{
			name:  "all confined",
			modes: []domainplugin.SandboxMode{domainplugin.SandboxEnforced, domainplugin.SandboxEnforced},
			want:  domainplugin.SandboxEnforced,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domainplugin.WeakestSandboxMode(tc.modes)
			if got != tc.want {
				msg := "WeakestSandboxMode(%v) = %q, want %q"
				if tc.why != "" {
					msg += "; " + tc.why
				}
				t.Errorf(msg, tc.modes, got, tc.want)
			}
		})
	}
}

func TestSandboxSupportModeReportsEnforcedOnlyWhenAvailable(t *testing.T) {
	if got := (domainplugin.SandboxSupport{Available: true}).Mode(); got != domainplugin.SandboxEnforced {
		t.Errorf("Mode() = %q for available support, want %q", got, domainplugin.SandboxEnforced)
	}
	unavailable := domainplugin.SandboxSupport{Reason: "not implemented"}
	if got := unavailable.Mode(); got != domainplugin.SandboxUnavailable {
		t.Errorf("Mode() = %q for unavailable support, want %q; a platform that cannot confine a "+
			"process must never report one as confined", got, domainplugin.SandboxUnavailable)
	}
}
