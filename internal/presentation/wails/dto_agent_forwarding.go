package wails

import (
	"xquakshell/internal/domain/agentforward"
	"xquakshell/internal/usecase"
)

// AgentForwardingDTO is a connection's agent forwarding settings as the editor sees them.
type AgentForwardingDTO struct {
	Enabled       bool     `json:"enabled"`
	IdentityIDs   []string `json:"identityIds"`
	Confirm       string   `json:"confirm"`
	WindowMinutes int      `json:"windowMinutes"`
}

// AgentConfirmDTO is one signature request shown to the user. It carries names and fingerprints
// only; the data to be signed never leaves the backend.
type AgentConfirmDTO struct {
	RequestID      string   `json:"requestId"`
	SessionID      string   `json:"sessionId"`
	ConnectionName string   `json:"connectionName"`
	Host           string   `json:"host"`
	KeyLabel       string   `json:"keyLabel"`
	KeyFingerprint string   `json:"keyFingerprint"`
	RemoteUser     string   `json:"remoteUser"`
	Verified       bool     `json:"verified"`
	Destination    string   `json:"destination"`
	KnownAs        []string `json:"knownAs"`
	ForwardsOn     bool     `json:"forwardsFurther"`
	WindowMinutes  int      `json:"windowMinutes"`
}

func agentForwardingToDTO(in *agentforward.Config) *AgentForwardingDTO {
	if in == nil {
		return nil
	}
	confirm := in.Confirm
	if confirm == "" {
		confirm = agentforward.ConfirmEach
	}
	ids := cloneStringSlice(in.IdentityIDs)
	if ids == nil {
		ids = []string{}
	}
	return &AgentForwardingDTO{Enabled: in.Enabled, IdentityIDs: ids, Confirm: string(confirm), WindowMinutes: in.WindowMinutes}
}

// agentForwardingFromDTO keeps the confirm policy exactly as sent. An unknown value is not mapped
// to a default here: the domain's Validate refuses it, so a client that sends "always" gets an
// error instead of a setting it did not choose.
func agentForwardingFromDTO(in *AgentForwardingDTO) *agentforward.Config {
	if in == nil {
		return nil
	}
	return &agentforward.Config{
		Enabled:       in.Enabled,
		IdentityIDs:   cloneStringSlice(in.IdentityIDs),
		Confirm:       agentforward.ConfirmPolicy(in.Confirm),
		WindowMinutes: in.WindowMinutes,
	}
}

func agentConfirmToDTO(p usecase.AgentConfirmPrompt) AgentConfirmDTO {
	known := append([]string{}, p.Destination.KnownAs...)
	return AgentConfirmDTO{
		RequestID:      p.RequestID,
		SessionID:      p.SessionID,
		ConnectionName: p.ConnectionName,
		Host:           p.Host,
		KeyLabel:       p.KeyLabel,
		KeyFingerprint: p.KeyFingerprint,
		RemoteUser:     p.RemoteUser,
		Verified:       p.Destination.Verified,
		Destination:    p.Destination.Fingerprint,
		KnownAs:        known,
		ForwardsOn:     p.Destination.ForwardsFurther,
		WindowMinutes:  p.WindowMinutes,
	}
}
