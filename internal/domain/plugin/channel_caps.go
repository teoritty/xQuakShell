package plugin

// Purpose enumerates the allowed uses of a channel bus channel (ADR-011).
const (
	PurposeExec        = "exec"
	PurposeEmbedStream = "embed-stream"
	PurposeTCPRelay    = "tcp-relay"
	PurposeUDPRelay    = "udp-relay"
)

// ChannelCaps declares the channel bus capability (ADR-011).
type ChannelCaps struct {
	Purposes          []string              `json:"purposes,omitempty"`
	MaxConcurrent     int                   `json:"maxConcurrent,omitempty"`
	MaxThroughputKbps int                   `json:"maxThroughputKbps,omitempty"`
	ExecCommands      []ExecCommandTemplate `json:"execCommands,omitempty"`
}

// ExecCommandTemplate declares one allowlisted argv template for the exec purpose.
// argv is always invoked as an array, never a shell string.
type ExecCommandTemplate struct {
	Argv   []string          `json:"argv"`
	Params map[string]string `json:"params,omitempty"`
}
