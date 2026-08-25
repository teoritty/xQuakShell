package domain

// ShellOption names one shell the user may choose between.
//
// It carries no filesystem path, and that absence is the security design rather than an
// omission. The application must never take a path from the UI or the vault and hand it to a
// process launcher: that is the whole command-injection class, and a type with nowhere to put a
// path cannot participate in it. The ID is a token from a closed set minted by the catalog
// (`pwsh`, `bash`, ...), so what the vault stores is a choice, not an instruction.
type ShellOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ShellCatalog reports which shells are actually present on this machine, for the settings
// picker to offer. It exists only to populate that list; starting one goes through
// LocalTerminalFactory, which does its own resolution.
type ShellCatalog interface {
	List() []ShellOption
}

// LocalTerminalOptions carries the initial geometry and working directory for a new shell.
type LocalTerminalOptions struct {
	Cols     uint16
	Rows     uint16
	StartDir string
}

// LocalTerminalFactory starts a shell attached to a pseudo-terminal on this machine.
//
// Start takes the stored ID and returns the ShellOption it actually launched, because resolving
// an ID to an executable and falling back when it no longer resolves both happen behind this
// port. Keeping them here is what lets the rest of the application handle only IDs and display
// names: no caller above infra ever holds an executable path, so no caller above infra can leak
// one or be tricked into supplying one.
//
// A shell the user picked and later uninstalled is not an error. Start falls back to the
// platform default and says so through the returned ShellOption, because a terminal that opens
// with the wrong shell is useful and one that refuses to open is not.
type LocalTerminalFactory interface {
	Start(shellID string, opts LocalTerminalOptions) (LocalTerminalPTY, ShellOption, error)
}

// LocalTerminalPTY is a running shell: bytes out, bytes in, a window size, and an end.
//
// Output is a channel rather than an io.Reader to match how the SSH terminal already reaches the
// UI, so both producers feed the same batching path instead of each inventing a pump. It closes
// when the shell exits or Close is called, which is the only signal that the process is gone.
type LocalTerminalPTY interface {
	Output() <-chan []byte
	Write(data []byte) error
	Resize(cols, rows uint16) error
	Close() error
}
