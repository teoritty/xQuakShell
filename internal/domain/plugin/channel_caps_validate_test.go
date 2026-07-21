package plugin_test

import (
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func baseChannelManifest() domainplugin.Manifest {
	return domainplugin.Manifest{
		ID:      "com.example.channel",
		Name:    "Channel",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "channel.exe"},
		Capabilities: domainplugin.CapabilitySet{
			Channel: &domainplugin.ChannelCaps{
				Purposes: []string{
					domainplugin.PurposeExec,
					domainplugin.PurposeEmbedStream,
					domainplugin.PurposeTCPRelay,
				},
				ExecCommands: []domainplugin.ExecCommandTemplate{
					{Argv: []string{"docker", "system", "dial-stdio"}},
					{
						Argv:   []string{"docker", "exec", "-it", "{containerId}", "sh"},
						Params: map[string]string{"containerId": "^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$"},
					},
				},
			},
		},
	}
}

func TestValidateChannelCapsAcceptsValidManifest(t *testing.T) {
	m := baseChannelManifest()
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected valid channel manifest, got %v", err)
	}
}

func TestValidateChannelCapsRejectsUnknownPurpose(t *testing.T) {
	m := baseChannelManifest()
	m.Capabilities.Channel.Purposes = []string{"exec", "bogus-purpose"}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "unknown channel purpose") {
		t.Fatalf("expected unknown purpose error, got %v", err)
	}
}

func TestValidateChannelCapsAcceptsUDPRelayPurpose(t *testing.T) {
	m := baseChannelManifest()
	m.Capabilities.Channel.Purposes = append(m.Capabilities.Channel.Purposes, domainplugin.PurposeUDPRelay)
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected udp-relay purpose to be valid, got %v", err)
	}
}

func TestValidateChannelCapsExecCommandsRequireExecPurpose(t *testing.T) {
	m := baseChannelManifest()
	m.Capabilities.Channel.Purposes = []string{domainplugin.PurposeEmbedStream}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "execCommands requires") {
		t.Fatalf("expected execCommands requires exec purpose error, got %v", err)
	}
}

func TestValidateChannelCapsRejectsMissingPlaceholderParam(t *testing.T) {
	m := baseChannelManifest()
	m.Capabilities.Channel.ExecCommands = []domainplugin.ExecCommandTemplate{
		{Argv: []string{"docker", "exec", "-it", "{containerId}", "sh"}},
	}
	if err := m.ValidateCapabilities(); err == nil || !strings.Contains(err.Error(), "containerId") {
		t.Fatalf("expected missing placeholder param error, got %v", err)
	}
}

func TestValidateChannelCapsRejectsUnsafeRegex(t *testing.T) {
	m := baseChannelManifest()
	m.Capabilities.Channel.ExecCommands = []domainplugin.ExecCommandTemplate{
		{
			Argv:   []string{"docker", "exec", "-it", "{containerId}", "sh"},
			Params: map[string]string{"containerId": "(a+)+"},
		},
	}
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected unsafe regex to be rejected")
	}
}

func TestValidateChannelCapsRejectsUncompilableRegex(t *testing.T) {
	m := baseChannelManifest()
	m.Capabilities.Channel.ExecCommands = []domainplugin.ExecCommandTemplate{
		{
			Argv:   []string{"docker", "exec", "-it", "{containerId}", "sh"},
			Params: map[string]string{"containerId": "["},
		},
	}
	if err := m.ValidateCapabilities(); err == nil {
		t.Fatal("expected uncompilable regex to be rejected")
	}
}

func TestValidateChannelCapsAllowsNoChannelCapability(t *testing.T) {
	m := domainplugin.Manifest{
		ID:      "com.example.nochannel",
		Name:    "NoChannel",
		Version: "1",
		Engine:  domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "x.exe"},
	}
	if err := m.ValidateCapabilities(); err != nil {
		t.Fatalf("expected nil channel caps to be valid, got %v", err)
	}
}

func TestPermissionSummaryIncludesExecConsentLine(t *testing.T) {
	m := baseChannelManifest()
	lines := m.PermissionSummary()
	found := false
	for _, l := range lines {
		if strings.Contains(strings.ToLower(l), "run commands") || strings.Contains(strings.ToLower(l), "exec") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected exec consent line in permission summary, got %v", lines)
	}
}

func TestPermissionSummaryOmitsExecLineWithoutExecPurpose(t *testing.T) {
	m := baseChannelManifest()
	m.Capabilities.Channel.Purposes = []string{domainplugin.PurposeEmbedStream}
	m.Capabilities.Channel.ExecCommands = nil
	lines := m.PermissionSummary()
	for _, l := range lines {
		if strings.Contains(strings.ToLower(l), "run commands") {
			t.Fatalf("did not expect exec consent line, got %v", lines)
		}
	}
}
