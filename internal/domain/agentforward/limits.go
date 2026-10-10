package agentforward

import "time"

// Limits are fixed by ADR-022 and are not configurable. Each one bounds what a hostile server can
// make this client do; a knob to raise them would be a knob for the server's benefit.
const (
	// MaxFrameLen bounds one agent protocol message, as OpenSSH's own agent does. x/crypto would
	// accept 16 MiB, which lets every agent channel make the client allocate that much on demand.
	MaxFrameLen = 256 * 1024

	// MaxChannelsPerSession bounds concurrent agent channels on one SSH session. Each `ssh` or
	// `git` run on the server opens one; eight covers parallel jobs without letting a loop on the
	// server open channels until the client runs out of goroutines.
	MaxChannelsPerSession = 8

	// MaxBindingsPerChannel bounds session-bind entries on one agent channel. A legitimate client
	// binds once per hop it connects through.
	MaxBindingsPerChannel = 16

	// SignRequestsPerWindow and SignRequestWindow bound signature requests per session. A git or
	// rsync run signs once; ten in ten seconds is a script, and more is a flood of dialogs.
	SignRequestsPerWindow = 10
	SignRequestWindow     = 10 * time.Second

	// ConfirmTimeout is how long an unanswered confirmation waits before it counts as a denial.
	// The remote command is blocked meanwhile; an indefinite wait would hang it forever, and a
	// dialog left open on an unattended machine must not be approvable by whoever comes back.
	ConfirmTimeout = 60 * time.Second

	// DefaultWindowMinutes and MaxWindowMinutes bound the "allow for this session" window.
	DefaultWindowMinutes = 15
	MaxWindowMinutes     = 60

	// MaxForwardedKeys bounds the keys one connection may forward. OpenSSH servers drop a client
	// after a handful of failed publickey attempts, so more keys than this cannot help a login.
	MaxForwardedKeys = 16

	// maxGrants bounds approval-window entries held at once across every session.
	maxGrants = 256
)
