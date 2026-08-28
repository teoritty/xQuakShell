# ADR-020: The Local Terminal

## Status

Accepted

## Context

xQuakShell is a terminal application whose terminals all point somewhere else. Opening a shell on
the machine the application is already running on meant leaving it for another window.

That is a small feature with one large property: a shell is a loader for arbitrary code. The
application has spent considerable effort making sure that one particular actor — an installed
plugin — cannot execute what it likes. [ADR-018](018-plugin-process-isolation.md) confines every
plugin process with the strongest mechanism its platform offers, refuses to start when a
confinement it asked for fails, and proves the boundary by attacking it. A plugin able to ask the
host to open a shell would step around all of it with one call.

So the question this decision answers is not "is a shell dangerous". The person at the keyboard
already has one, and this application is not a sandbox for its own user. The question is **who,
other than that person, can reach it** — and the answer has to be nobody, by construction rather
than by review.

Two further facts shaped the implementation. The application ships as a single portable binary and
cross-compiles, which rules out any pseudo-terminal library needing cgo. And the pseudo-console API
on Windows (`CreatePseudoConsole`) only exists from build 17763, with nothing that reports its
absence before the first attempt fails.

## Decision

**1. The RPC that opens a terminal takes no arguments, and never will.**
`AppAPI.OpenLocalTerminal()` is niladic. Nothing the frontend sends can influence which program
runs. This is what makes the command-injection class (CLAUDE.md §2.1.7) absent rather than guarded
against: there is no parameter to inject into. `TestOpenLocalTerminalTakesNoArguments` holds the
signature to it by reflection, because the parameter that breaks this would arrive in a diff that
looks like a small convenience.

**2. The stored setting names a shell, it does not describe one.**
A user picks from a list the host built by probing this machine, and what the vault holds is an id
from a closed, hand-written set — `pwsh`, `bash`, `zsh` and four others. Resolving an id to an
executable happens in `internal/infra/localshell`, at the moment a terminal opens. `ShellOption`,
the type that crosses every layer above infra, **has no path field**, so no layer above infra can
hold a path, leak one, or be handed one.

The obvious alternative — a free-text "shell executable" box in settings — is rejected. A path in
the vault is an instruction to a process launcher, and the vault is a file on disk.

**3. `$SHELL` is honoured by basename, matched against the same closed set.**
An environment variable is settable by anything in the process tree. Taking its value as an
executable would hand the choice of what this application runs to whoever set it.

**4. Input travels one way, from a keyboard.**
There is no API to write bytes into a local terminal programmatically. This is what stops a
compromised remote host — whose session output is bytes it controls — from ever escalating into
local code execution.

**5. The local terminal is not exposed to plugins, and this is enforced structurally.**
Nothing was added to `api_registry.go` or `hostRegistry`: no capability, no host method, no channel
purpose. Two tests in `test/unit/architecture/local_terminal_isolation_test.go` keep it that way —
plugin-facing packages may not import the local shell or name its use case, and the local shell may
not import the plugin process machinery. Both run on the AST with comments dropped, so the prose
explaining the rule does not trip the test enforcing it.

**6. The two process spawners share no code, deliberately.**
`internal/infra/plugin` and `internal/infra/localshell` both start child processes and both kill
process trees, and the duplication stays. One exists to confine an untrusted binary; the other to
run the user's own shell unconfined. A shared implementation puts a single switch between those two
behaviours, and the day it is flipped the wrong way is the day the plugin sandbox silently stops
being one. A duplicated process-group kill is much the cheaper mistake. The difference is visible
in the job objects: the plugin's caps memory, forbids descendants entirely and strips the UI
surface; the shell's sets `KILL_ON_JOB_CLOSE` and nothing else, because a shell exists to start
other programs.

**7. A local terminal survives a vault lock.**
Plugin surfaces are cleared on lock, because they are views onto sessions that no longer exist. A
local shell belongs to the user's machine and holds nothing from the vault, so locking hides it.
The mechanism is an absence: the store exports no `clearLocalTerminals`, the service exposes no
lock hook, and two tests assert those absences. A lock that killed a running build would be the
worst surprise this feature could produce.

**8. Only the fact of a shell is audited, never its contents.**
An SSH session reconstructs and records every command line, because that trail is about what was
done to somebody else's machine. A local terminal is the user's own computer, where they already
have a shell history; recording their keystrokes would collect passwords typed into command
arguments in exchange for telling nobody anything they could not already read. What is recorded is
that the application started a shell, and which one.

**9. A system that cannot host a pseudo-console says so.**
`ErrLocalTerminalUnsupported` is distinct from an ordinary start failure, because only one of them
is worth retrying: Windows before build 17763 will never work, and no build-time check can detect
it.

## Alternatives rejected

- **Modelling a local terminal as a session.** `OpenSession` requires a persisted `Connection`, and
  `ValidateForConnect` would in fact accept `{Protocol: "local", Host: "localhost"}` today. It was
  rejected on two counts: the id would have to be a magic constant crossing the RPC boundary,
  weakening decision 1; and the change would land in `session_lifecycle_service.go`, whose
  `CloseSession` holds both of the bugs recorded against that file in the last six months.
- **Reusing the plugin surface machinery (ADR-015).** A surface is a view onto work a plugin is
  already authorized to do, keyed by `PluginID` and torn down with its parent session. A
  core-owned tab with no plugin and no session would have had to fake both.
- **Shipping it as a bundled plugin.** It would reuse the most code and require exactly the
  capability decision 5 forbids. A sandbox with a shell inside it is not a sandbox.
- **Reusing `SurfaceOutputBroker` for output.** Its chunk carries a stdout/stderr stream and its
  overflow error wraps a plugin rate-limit error, neither of which means anything here. The broker
  exists because its producer is a separate process that must be told when the consumer is behind;
  this producer is a read loop behind a bounded channel and simply blocks, so the backpressure is
  already correct and only batching was needed.

## Consequences

- A new external dependency, `github.com/aymanbagabas/go-pty`: pure Go, no cgo, verified to
  cross-compile for windows/amd64, linux/amd64, linux/arm64, darwin/amd64 and darwin/arm64.
- Windows consoles start on the system OEM code page while the renderer decodes UTF-8, so `cmd` and
  Windows PowerShell are started with an argument that switches the console to UTF-8 from within.
  The host cannot do this from outside: the code page belongs to the console the child attaches to.
- A third kind of tab. `resolveTabIn`, `tabTitle`, `tabState`, `allTabIds`, `closeTab`,
  `tileLayout.sync` and the two components that render a tab all route on three answers now.
- Nothing here touches a frozen contract under [ADR-017](017-release-and-compatibility-policy.md).
  `pluginApi`, the capability set, the manifest schema, `bundleFormat` and the on-disk schemas are
  unchanged — which is decision 5 restated as a release fact.

## What this does not cover

Named because a gap that lives only in a commit message stops being a gap in anyone's mind:

- **Windows builds between 17763 and 1903.** The pseudo-console exists there and the UTF-8 console
  has known defects, so some output may still be garbled. Not a regression this feature introduces,
  since there was no local terminal to compare against.
- **A shell that detaches its own children.** A process that starts a new process group on Unix, or
  breaks away from the job object on Windows, outlives the tab. Both are deliberate acts by the
  program doing them; the guarantee here covers the ordinary tree.
- **macOS confinement.** As in ADR-018, and here it is not even a goal: the local terminal is
  unconfined on every platform by design.
- **Restoring terminals after a restart.** A local terminal keeps no state and is not reopened.
