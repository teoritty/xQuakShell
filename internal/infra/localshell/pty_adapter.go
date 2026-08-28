package localshell

import (
	"fmt"
	"os"
	"strings"

	"github.com/aymanbagabas/go-pty"

	"xquakshell/internal/domain"
)

// appEnvPrefix marks the variables this application sets for its own child processes. They are
// stripped from a shell's environment because they are internal wiring, not something the user's
// commands should inherit or be able to read.
const appEnvPrefix = "XQS_"

var _ domain.LocalTerminalFactory = (*Factory)(nil)

// Factory starts shells attached to a pseudo-terminal.
type Factory struct {
	catalog *Catalog
}

// NewFactory returns a Factory that starts shells the given catalog knows about.
func NewFactory(catalog *Catalog) *Factory {
	return &Factory{catalog: catalog}
}

// Start launches the shell named by shellID, falling back to this platform's best available shell
// when that id resolves to nothing.
//
// The resolved executable path stays inside this function. Everything it hands back names the
// shell by id and display name, which is what keeps a path out of every layer above infra.
func (f *Factory) Start(
	shellID string,
	opts domain.LocalTerminalOptions,
) (domain.LocalTerminalPTY, domain.ShellOption, error) {
	path, opt, ok := f.catalog.resolve(shellID)
	if !ok {
		path, opt, ok = f.catalog.resolveDefault()
	}
	if !ok {
		return nil, domain.ShellOption{}, fmt.Errorf("no usable shell: %w", domain.ErrLocalTerminalUnsupported)
	}

	// Windows before build 17763 has no CreatePseudoConsole, and nothing reports that before the
	// first attempt. A failure here is the feature being unavailable on this machine rather than
	// this particular shell failing, so it is reported as the permanent condition it is.
	handle, err := pty.New()
	if err != nil {
		return nil, domain.ShellOption{}, fmt.Errorf("open pty: %w", domain.ErrLocalTerminalUnsupported)
	}

	term, err := startShell(handle, path, opt, opts)
	if err != nil {
		_ = handle.Close()
		return nil, domain.ShellOption{}, err
	}
	return term, opt, nil
}

// startShell runs the executable on an already-open pseudo-terminal and begins pumping its output.
func startShell(
	handle pty.Pty,
	path string,
	opt domain.ShellOption,
	opts domain.LocalTerminalOptions,
) (*terminal, error) {
	cmd := handle.Command(path, startupArgs(opt.ID)...)
	cmd.Dir = startDir(opts.StartDir)
	cmd.Env = shellEnv()
	configureCmd(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shell %s: %w", opt.Name, err)
	}

	term := newTerminal(handle, cmd)
	// Assigning after Start leaves a sliver in which a child spawned immediately escapes the
	// group. A shell has not read its first keystroke yet at this point, so in practice there is
	// nothing to escape; closing it properly needs a handle the API only gives out after Start.
	term.adopt()

	// A geometry the frontend has not measured yet is not worth failing an otherwise working
	// shell over - the first resize from the mounted terminal supersedes it either way.
	if opts.Cols > 0 && opts.Rows > 0 {
		_ = handle.Resize(int(opts.Cols), int(opts.Rows))
	}

	term.pump()
	return term, nil
}

// startDir falls back to the user's home directory, and then to whatever the process is already
// in. An unreadable home is not a reason to refuse a shell.
func startDir(requested string) string {
	if requested != "" {
		return requested
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

// shellEnv returns the user's environment with this application's own variables removed.
//
// Deliberately the opposite posture from the plugin spawner's allowlist: a plugin gets the few
// variables it was promised, while a shell has to behave like the user's terminal, and a terminal
// that quietly dropped PATH, LANG or SSH_AUTH_SOCK would be broken in ways nobody would connect
// back to this function.
func shellEnv() []string {
	source := os.Environ()
	out := make([]string, 0, len(source))
	for _, entry := range source {
		if strings.HasPrefix(entry, appEnvPrefix) {
			continue
		}
		out = append(out, entry)
	}
	return out
}
