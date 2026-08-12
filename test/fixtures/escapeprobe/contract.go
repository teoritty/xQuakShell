// Package escapeprobe is the wire contract between the escape-probe fixture and the tests that
// drive it.
//
// It exists because the fixture is a separate binary: the two sides can only agree through JSON,
// and a vector renamed on one side and not the other would report "not attempted" — which, read
// carelessly, looks exactly like a boundary that held. Sharing the names makes that drift a compile
// error instead of a silently weaker test. The control arm covers the rest: it requires every
// vector it sent to have been attempted.
package escapeprobe

// The vectors an escape probe can be asked to try. Each name is a JSON key on the wire, so renaming
// one is a contract change that both binaries pick up together.
const (
	// VectorReadForeign reads a file outside every directory the plugin was granted.
	VectorReadForeign = "read_foreign"
	// VectorWriteForeign creates a file in a directory outside every grant.
	VectorWriteForeign = "write_foreign"
	// VectorDialTCP opens a TCP connection to a listener the test is running.
	VectorDialTCP = "dial_tcp"
	// VectorDialUDP sends a datagram and waits for the echo, so a socket that was created but
	// never carried a byte does not count as reaching the network.
	VectorDialUDP = "dial_udp"
	// VectorSymlinkEscape reads a symlink that lives inside the plugin's own writable directory
	// and points outside it — the boundary must be judged by where the link lands.
	VectorSymlinkEscape = "symlink_escape"
	// VectorDotDotTraversal reads an unnormalised path that climbs out of the data root with "..".
	VectorDotDotTraversal = "dotdot_traversal"
	// VectorProcEnviron reads /proc/<pid>/environ of the host process, which holds its environment.
	VectorProcEnviron = "proc_environ"
	// VectorProcSelfRoot reaches a foreign path through /proc/self/root, the magic symlink to /.
	VectorProcSelfRoot = "proc_self_root"
	// VectorPtraceHost attaches to a process outside the sandbox, which would hand the plugin that
	// process's memory regardless of any filesystem rule.
	VectorPtraceHost = "ptrace_host"
	// VectorWriteThenExec writes a binary into the plugin's own writable directory and runs it.
	// A writable grant that also carried execute would turn every plugin into an arbitrary loader.
	VectorWriteThenExec = "write_then_exec"
	// VectorWriteInstallTree writes into the plugin's own install directory, which is granted for
	// reading and executing only.
	VectorWriteInstallTree = "write_install_tree"
	// VectorReadOtherPlugin reads the data directory of a different installed plugin.
	VectorReadOtherPlugin = "read_other_plugin"
	// VectorListPluginsRoot lists the directory every plugin is installed under, which names what
	// else the user has installed.
	VectorListPluginsRoot = "list_plugins_root"
	// VectorReadVault reads the vault file, at the path production computes for it.
	VectorReadVault = "read_vault"
)

// Target is what one vector is pointed at. A vector reads the one field it needs; the others are
// absent from the wire.
type Target struct {
	Path string `json:"path,omitempty"`
	Addr string `json:"addr,omitempty"`
	Pid  int    `json:"pid,omitempty"`
}

// Request is the whole instruction: try these vectors against these targets, and only these.
type Request struct {
	Vectors map[string]Target `json:"vectors"`
}

// Outcome reports one attempt.
//
// Succeeded is what the tests assert on, rather than "was this a permission error", because a
// sandbox does not owe anyone a tidy errno. A Windows AppContainer drops a loopback connection and
// the caller sees a timeout; Landlock returns EACCES; a denied read returns a permission error on
// both. Insisting on the tidy shape made a genuinely blocked socket read as "not denied".
//
// Err carries the detail anyway, because a failure message that only says "it did not succeed"
// cannot distinguish a boundary that held from a path with a typo in it. What guards against the
// typo is the unconfined control run, which requires every vector it asked for to succeed.
type Outcome struct {
	Attempted bool   `json:"attempted"`
	Succeeded bool   `json:"succeeded"`
	Err       string `json:"err,omitempty"`
}

// Report answers a Request with one Outcome per vector asked for.
type Report struct {
	Vectors map[string]Outcome `json:"vectors"`
}
