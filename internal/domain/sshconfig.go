package domain

import "errors"

// SSHConfigHost is one importable host derived from an OpenSSH client
// configuration file. It is the fully resolved view of a `Host` alias: every
// matching block (including wildcard defaults such as `Host *`) has already
// been folded in by the parser, so the importer never re-implements OpenSSH
// precedence rules.
type SSHConfigHost struct {
	// Alias is the literal name written after `Host`. It becomes the
	// connection name and is the identifier the UI selects rows by.
	Alias string
	// HostName is the network target, with %h already substituted. It falls
	// back to Alias when the config declares no HostName.
	HostName string
	// Port is 0 when the config declares none; callers apply DefaultSSHPort.
	Port int
	// User is empty when the config declares none.
	User string
	// IdentityFiles are absolute paths to existing, readable regular files,
	// in the order OpenSSH would try them. Entries that do not exist are
	// dropped and reported as a notice instead.
	IdentityFiles []string
	// JumpHops is the ProxyJump chain in traversal order (client → hop[0] →
	// … → this host). Empty when no ProxyJump applies.
	JumpHops []SSHConfigHop
}

// SSHConfigHop is one bastion in a ProxyJump chain, resolved through the same
// configuration as its target host so that a bare `ProxyJump bastion` picks up
// the `Host bastion` block's HostName, User, Port and IdentityFile.
type SSHConfigHop struct {
	Alias         string
	HostName      string
	Port          int
	User          string
	IdentityFiles []string
}

// SSHConfigNoticeKind classifies a non-fatal finding from parsing.
type SSHConfigNoticeKind string

const (
	// SSHConfigNoticeMatchBlockSkipped reports a `Match` block. Match
	// conditions (exec, canonical, final) cannot be evaluated without
	// performing the connection, so the block contributes no values.
	SSHConfigNoticeMatchBlockSkipped SSHConfigNoticeKind = "matchBlockSkipped"
	// SSHConfigNoticeProxyCommandUnsupported reports a host whose proxying is
	// expressed as ProxyCommand, which has no equivalent in a jump chain.
	SSHConfigNoticeProxyCommandUnsupported SSHConfigNoticeKind = "proxyCommandUnsupported"
	// SSHConfigNoticeIncludeUnreadable reports an Include target that could
	// not be read, matched nothing, or was refused by a parser limit.
	SSHConfigNoticeIncludeUnreadable SSHConfigNoticeKind = "includeUnreadable"
	// SSHConfigNoticeIdentityFileMissing reports an IdentityFile path that
	// does not resolve to a readable regular file.
	SSHConfigNoticeIdentityFileMissing SSHConfigNoticeKind = "identityFileMissing"
	// SSHConfigNoticeJumpHostUnresolved reports a ProxyJump hop that could not
	// be turned into a usable hop (unparsable, or a self-referencing chain).
	SSHConfigNoticeJumpHostUnresolved SSHConfigNoticeKind = "jumpHostUnresolved"
	// SSHConfigNoticeLimitReached reports that a parser safety limit stopped
	// further processing, so the result may be incomplete.
	SSHConfigNoticeLimitReached SSHConfigNoticeKind = "limitReached"
)

// SSHConfigNotice is a single non-fatal parsing finding.
//
// Security: Target carries only a host alias or a file's base name — never
// file contents, never an absolute path from outside the user's selection.
// The presentation layer renders Kind as a human sentence; no error text from
// the filesystem ever reaches the UI through this type.
type SSHConfigNotice struct {
	Kind   SSHConfigNoticeKind
	Target string
}

// SSHConfigParseResult is everything one parse produced: the importable hosts
// and the findings worth telling the user about.
type SSHConfigParseResult struct {
	Hosts   []SSHConfigHost
	Notices []SSHConfigNotice
}

// SSHConfigImporter reads and interprets OpenSSH client configuration files.
//
// Security: this port is the only filesystem surface of the ssh_config import
// feature. Parse is given a path the user chose explicitly; ReadKeyFile must
// only ever be called with a path that the same Parse call returned in
// SSHConfigHost.IdentityFiles or SSHConfigHop.IdentityFiles, so that no caller
// upstream — including a compromised frontend — can use the importer as a
// general-purpose file reader.
type SSHConfigImporter interface {
	// DefaultPath returns the conventional user config path when it exists,
	// or an empty string when there is none. It never fails merely because
	// the file is absent.
	DefaultPath() (string, error)
	// Parse reads path (following Include directives) and resolves it into
	// importable hosts. Malformed directives are skipped as notices; only an
	// unreadable or oversized root file is an error.
	Parse(path string) (SSHConfigParseResult, error)
	// ReadKeyFile returns the bytes of a private key file previously reported
	// by Parse.
	ReadKeyFile(path string) ([]byte, error)
}

var (
	// ErrSSHConfigNotFound indicates the requested config file does not exist.
	ErrSSHConfigNotFound = errors.New("ssh config not found")
	// ErrSSHConfigTooLarge indicates a config or key file exceeded the size
	// limit the parser enforces to bound memory use.
	ErrSSHConfigTooLarge = errors.New("ssh config file too large")
	// ErrSSHConfigUnreadable indicates the file exists but could not be read.
	ErrSSHConfigUnreadable = errors.New("ssh config file unreadable")
)
