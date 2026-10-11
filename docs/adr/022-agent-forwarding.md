# ADR-022: Agent Forwarding Signs With Vault Keys, Only Logins, Only When Asked

## Status

Accepted. Adds two optional vault fields; the vault schema does not move
([ADR-017](017-release-and-compatibility-policy.md)).

## Context

Users need to work *from* a server: `git pull` from a private repository, `ssh` to the next host,
`scp` or `rsync` between two servers. The usual answer is OpenSSH agent forwarding, which hands the
remote side a socket that signs with the user's keys.

That socket is the most dangerous thing an SSH client can give a server. Anyone with root on it —
or with access to the socket — can sign with every key the agent holds, for as long as the
connection lasts, for any purpose, and the user sees nothing. OpenSSH's mitigations (`ssh-add -c`,
destination constraints) exist but are opt-in and rarely used.

xQuakShell's keys live in the vault, so whatever answers on that socket is this application. That
puts every decision OpenSSH leaves to configuration under our control.

## Decision

**1. Off unless both the key and the connection say yes.** A connection lists the keys it forwards;
a key carries `AllowForwarding`, off by default. A key is offered only when both hold, and this is
re-checked at every request, so revoking either takes effect mid-session. The login key is not
implied: what authenticates the user to the first server and what is forwarded are separate lists.

**2. Only logins are signed.** The data to sign must parse as a `publickey`
`SSH_MSG_USERAUTH_REQUEST` for the very key being asked for, with nothing after it. That covers
`ssh`, `git` over SSH, `scp` and `rsync`. SSHSIG (git commit signing), arbitrary blobs and any
other protocol are refused: an agent that signs what it is shown is a signing oracle for whoever is
showing it. RSA signs only with SHA-2, and the algorithm in the request must match the flags.

**3. Every signature is confirmed, and the dialog says where it goes.** The remote OpenSSH client
(8.9+) sends `session-bind@openssh.com` with the next host's key and its signature over the
session identifier. We verify that signature; a sign request whose session id matches a verified
binding is usable only in a session with that host, so the dialog can name it truthfully —
"git.corp:22, known host" or "unknown key SHA256:…". Without a binding the dialog says the
destination is unverified.

**4. A window is narrow.** "Allow for N minutes" grants one key, one *verified* destination, one
session. An unverified destination is never approved without asking. Windows die with the session
and do not survive a reconnect.

**5. The agent is read-only and bounded.** We do not use `agent.ServeAgent`. Our own loop accepts
three opcodes — list, sign, extension — and answers everything else with failure before parsing
it, so a hostile server never makes us decode a private key it sent. Frames are capped at 256 KiB
(x/crypto allows 16 MiB), channels at 8 per session, sign requests at 10 per 10 seconds, bindings
at 16 per channel, and an unanswered dialog becomes a deny after 60 seconds.

**6. Only the final target, only the shell.** `auth-agent-req` is sent on the PTY session to the
final host. Jump hops, exec sessions, SFTP and plugin connectors never receive the agent, and the
`auth-agent@openssh.com` channel handler is registered only on a client that asked for forwarding.

**7. Every decision is audited**, by connection id, key fingerprint, remote user and destination
fingerprint. Never the signed data, never the signature.

## Rejected

- **Delegating to the system ssh-agent.** Keys would have to leave the vault.
- **`agent.Keyring` plus filters in infra.** Business rules would sit in infrastructure where they
  cannot be tested in isolation, and the keyring accepts add/remove/lock from the remote side.
- **A setting that signs without asking.** It would make the residual risk below silent, which is
  the exact failure this feature exists to avoid.
- **Allowing SSHSIG.** Commit signing on a shared server is a request for a signing oracle; it can
  be added later as its own decision with namespace checks.

## Residual risk

While the user approves a signature, root on the intermediate server can use it for the
destination named in the dialog — this is inherent to agent forwarding. A window extends that to
the window. The connection editor states this next to the switch.

## Tests

Hostile input is tested at each layer: SSHSIG and arbitrary blobs, wrong message type, `none` and
`hostbased` methods, trailing bytes, length overflow, a key in the data that differs from the key
asked for, SHA-1 RSA, keys out of scope or revoked mid-session, a window reused for another key,
host or session, an unverified destination with a window, a dialog timeout, a double answer, a
foreign request id, requests after close, rate limit, add/remove/lock/unlock from the remote,
oversized frames, a ninth channel, an agent channel nobody asked for, forged and conflicting
session-binds, and an audit log checked for secrets. `domain/agentforward` is a mutation target.
