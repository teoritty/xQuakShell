# ADR-018: Plugin Process Isolation

## Status

Accepted

## Context

A plugin is a third-party binary the user installed, running on the user's account with the user's
privileges. Before this decision the only boundary around it was the process boundary itself
([ADR-003](../security-model.md#process-isolation-adr-003)): a separate process, a JSON-RPC control
plane, capability proxies for the things a plugin is supposed to reach. None of that stops the
binary from opening `~/.ssh/id_ed25519` directly, or `vault.age`, or another plugin's data — the
capability proxies are the door, and nothing was stopping anyone walking around the building.

The operating systems this application ships on offer unequal answers:

| Platform | Mechanism | What it covers |
|---|---|---|
| Windows | AppContainer + job object | Files by ACL, network entirely, one process per plugin |
| Linux | Landlock (ABI 1+) | Files completely; TCP from ABI 4 (kernel 6.7); never UDP or raw sockets |
| macOS | — | Nothing available to a non-sandboxed, non-App-Store binary without an entitlement |

Two further facts shaped everything below. Landlock is **self-applied and irreversible**: a process
confines itself, the domain survives `execve`, and no later ruleset can widen it. And an
AppContainer child **cannot be an `*exec.Cmd`** — `os/exec` exposes no attribute list, so
`PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES` has no way through it.

## Decision

**1. Confine the plugin process on every platform that can, using that platform's own mechanism.**
On Linux the host re-executes itself as a shim
([`shim_args.go`](../../internal/infra/plugin/sandbox/shim_args.go)), the shim applies the ruleset to
the process it is already in, and only then execs the plugin into that confinement. There is no
other shape available: the host cannot restrict a child from outside, Go offers no hook between fork
and exec, and an untrusted binary cannot be asked to confine itself.

**2. Exactly two grants, and never a third.** The plugin's installed files, read and execute; its own
instance directory, read and write. The writable grant carries **no** execute right on either
platform — otherwise a plugin could write a binary into its data directory and run it, which is a
loader for arbitrary code wearing a plugin's clothes.

**3. The instance directory, not the plugin directory.** A plugin's data directory is a *child* of
its install tree, so granting the install tree as one path would hand every session read access to
every other session's files — through the very mechanism added to prevent that.

**4. macOS gets no isolation, permanently.** Not a gap awaiting work: the mechanisms available to a
binary distributed outside the App Store do not confine a child process this way. The product says
so rather than implying a boundary it does not have.

**5. A platform that CANNOT confine starts the plugin unconfined with no opt-in. A confinement that
CAN be applied and FAILS refuses the start.** These deliberately do not share a code path
([`spawn_fallback.go`](../../internal/infra/plugin/spawn_fallback.go)). A sandbox that quietly fell
through to an unrestricted exec whenever it broke would keep reporting success while protecting
nobody, for whichever fraction of users hit the breakage. Refusing is loud and diagnosable. A user
who needs the plugin anyway can enable an explicit fallback setting, and every fallback start is
logged at warning level — accepting a risk is not the same as asking to stop being told about it.

**6. Partial confinement is reported as partial, never rounded up.**
[`SandboxSupport.Mode`](../../internal/domain/plugin/sandbox.go) has four values, and Linux reports
`enforced-partial` on **every** kernel — even ABI 5 — because Landlock covers no UDP and no raw
socket. "Sandboxed" and "sandboxed except for the network" are different sentences and the user gets
the true one.

## Alternatives rejected

- **A container or VM per plugin.** Correct and unusable: it would require Docker or Hyper-V on a
  desktop application that ships as a single portable binary.
- **`seccomp` instead of Landlock.** Filters syscalls, not paths. Denying `openat` outright breaks
  every plugin; allowing it protects nothing, because the interesting question is *which file*.
- **Dropping to a dedicated user account.** Needs administrator rights at install time on both
  platforms, and turns a portable application into an installed one.
- **Falling back to an unconfined start whenever confinement fails.** Rejected as decision 5 above;
  this is the failure mode the whole status pipeline exists to make impossible.
- **Reporting "sandboxed" on Linux when the kernel has network rules.** Rejected as decision 6: the
  claim would still be false for UDP.

## Consequences

- Every capability a plugin legitimately needs must arrive through the host's proxies. A plugin
  that reads a path directly now fails on two of three platforms — which is the point, and is a
  breaking change for any plugin that was doing so.
- The Linux shim means the host binary is also its own sandbox launcher. `IsShimMode` is dispatched
  in `main` before anything else is built, and its argv is a security boundary: it is parsed, every
  writable grant is checked against the data root, and a malformed instruction exits without
  exec'ing anything.
- Confinement is asserted by trying to escape it, not by inspecting a mask
  ([`sandbox_escape_test.go`](../../test/unit/plugin/sandbox_escape_test.go)): a real plugin behind
  the real sandbox attacks the vault path, another plugin's data, its own install tree, `/proc`,
  symlinks and sockets, with a control arm that requires all of it to work unconfined.
- Because half the implementation exists only on one platform, the test workflow runs on both
  ([`test.yml`](../../.github/workflows/test.yml)). A single-platform matrix left the AppContainer
  ACLs and the Landlock ruleset each tested only by whoever happened to be on that OS.
- Neither the plugin manifest nor `pluginApi` gained a field for this, so nothing here is a frozen
  contract under [ADR-017](017-release-and-compatibility-policy.md). The sandbox mode is a runtime
  fact reported to the UI, not something a plugin declares or negotiates.

## What this does not cover

Named because a gap that lives only in a commit message stops being a gap in anyone's mind:

- **UDP and raw sockets on Linux, at every ABI.** Landlock has no rule for them. A confined plugin
  can still open a UDP socket and send whatever it can read.
- **macOS, entirely.**
- **A kernel below Landlock ABI 1, or one booted without `lsm=landlock`.** Reported as unavailable
  with a reason that names the fix.
- **The plugin's own memory and its IPC channel.** Isolation bounds what the process can *reach*,
  not what the user hands it: a plugin granted a secret still has that secret.
