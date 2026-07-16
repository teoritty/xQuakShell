package usecase

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
	"ssh-client/internal/pkg/safego"
)

// execSession is the minimal surface ChannelExecBackend needs from an SSH exec session. Its
// method set matches *golang.org/x/crypto/ssh.Session exactly, so the real *ssh.Session returned
// by domain.SSHClient.NewSession satisfies it structurally without this file importing the ssh
// package (usecase production code must not import golang.org packages — see
// test/unit/architecture/rules.go).
type execSession interface {
	StdinPipe() (io.WriteCloser, error)
	StdoutPipe() (io.Reader, error)
	StderrPipe() (io.Reader, error)
	Start(cmd string) error
	Wait() error
	Close() error
}

// ChannelCloseNotifier reports a channel's terminal reason/message once its remote process ends,
// so the caller can surface it as a channel.close {reason, message} JSON-RPC notification (ADR-011:
// there is no binary error frame for application-level errors).
type ChannelCloseNotifier func(reason, message string)

const stderrCaptureLimit = 4 * 1024

// ChannelExecBackend implements the exec purpose (ADR-011 Stage 7): it matches a channel.open
// hint against the plugin's manifest argv-template allowlist (domainplugin.MatchExecCommand),
// then runs the matched argv as an SSH exec channel over the parent session's already
// authenticated *ssh.Client, streaming stdio onto the channel's binary data path.
type ChannelExecBackend struct {
	pluginID       string
	execCommands   []domainplugin.ExecCommandTemplate
	consentGranted bool
	registry       *SessionRegistry
	audit          domainplugin.ChannelAuditRecorder
	closeNotifier  ChannelCloseNotifier

	// sessionOpener obtains the exec session for a parentSessionId. Defaults to
	// openSessionViaRegistry (production path, through SessionRegistry's lock discipline);
	// overridden by tests to avoid needing a live SSH server.
	sessionOpener func(parentSessionID string) (execSession, error)

	mu              sync.Mutex
	argv            []string
	parentSessionID string
	session         execSession
	closed          bool
}

// NewChannelExecBackend creates an exec backend for one channel.open request. consentGranted
// reflects the plugin's install-time exec consent (the same one-time gate
// PluginManager.Install already enforces for allowArbitraryOutbound/allowMultiSession — a
// successfully installed plugin with an ungranted high-impact capability should never reach this
// backend with consentGranted=true, but Authorize checks it explicitly rather than trusting the
// manifest declaration alone).
func NewChannelExecBackend(pluginID string, execCommands []domainplugin.ExecCommandTemplate, consentGranted bool, registry *SessionRegistry, audit domainplugin.ChannelAuditRecorder, closeNotifier ChannelCloseNotifier) *ChannelExecBackend {
	b := &ChannelExecBackend{
		pluginID:       pluginID,
		execCommands:   execCommands,
		consentGranted: consentGranted,
		registry:       registry,
		audit:          audit,
		closeNotifier:  closeNotifier,
	}
	b.sessionOpener = b.openSessionViaRegistry
	return b
}

// Authorize validates purpose, install-time consent, and matches hint against the manifest's
// declared argv templates, storing the concrete argv for Wire. No SSH session is opened here.
func (b *ChannelExecBackend) Authorize(purpose, parentSessionID, hint string) error {
	if purpose != domainplugin.PurposeExec {
		return domainplugin.ErrCapabilityDenied
	}
	if !b.consentGranted {
		return domainplugin.ErrCapabilityDenied
	}
	argv, err := domainplugin.MatchExecCommand(b.execCommands, hint)
	if err != nil {
		return err
	}

	b.mu.Lock()
	b.argv = argv
	b.parentSessionID = parentSessionID
	b.mu.Unlock()
	return nil
}

// openSessionViaRegistry obtains the parent session's SSH client through SessionRegistry.Get —
// the same accessor SessionIOService.GetSSHClient uses — and never reads sessionEntry fields or
// the registry's mutex any other way (ADR-011 §Implementation note / ADR-009 lock discipline).
func (b *ChannelExecBackend) openSessionViaRegistry(parentSessionID string) (execSession, error) {
	entry, ok := b.registry.Get(parentSessionID)
	if !ok || entry.sshClient == nil {
		return nil, domain.ErrSessionNotFound
	}
	sess, err := entry.sshClient.NewSession()
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// Wire opens the SSH exec session for the argv Authorize matched, starts it via a safely
// shell-quoted command string (see shellQuoteArgv), and streams stdio onto the channel.
func (b *ChannelExecBackend) Wire(ctx context.Context, ch *domainplugin.ChannelHandle) error {
	b.mu.Lock()
	argv := b.argv
	parentSessionID := b.parentSessionID
	b.mu.Unlock()
	if len(argv) == 0 {
		return domainplugin.ErrCapabilityDenied
	}

	sess, err := b.sessionOpener(parentSessionID)
	if err != nil {
		return err
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		return err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		_ = sess.Close()
		return err
	}

	if err := sess.Start(shellQuoteArgv(argv)); err != nil {
		_ = sess.Close()
		return err
	}

	b.mu.Lock()
	b.session = sess
	b.mu.Unlock()

	if b.audit != nil {
		b.audit(domainplugin.ChannelAuditEntry{
			Timestamp:       time.Now(),
			PluginID:        b.pluginID,
			Action:          "channel.open",
			ChannelID:       ch.ChannelID(),
			Purpose:         ch.Purpose(),
			ParentSessionID: ch.ParentSessionID(),
			Target:          strings.Join(argv, " "),
			Success:         true,
		})
	}

	data := ch.Data()

	safego.GoNamed("plugin.channelExecInbound", func() {
		for {
			payload, ok := data.Recv()
			if !ok {
				_ = stdin.Close()
				return
			}
			if _, err := stdin.Write(payload); err != nil {
				_ = stdin.Close()
				return
			}
			// The frame is in the remote process's stdin: the plugin may send one more.
			if err := data.Ack(ctx); err != nil {
				_ = stdin.Close()
				return
			}
		}
	})

	safego.GoNamed("plugin.channelExecOutbound", func() {
		buf := make([]byte, 32*1024)
		for {
			if err := data.WaitForCapacity(ctx); err != nil {
				return
			}
			n, err := stdout.Read(buf)
			if n > 0 {
				if sendErr := data.Send(ctx, append([]byte(nil), buf[:n]...)); sendErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	})

	// The wait/close-notify goroutine runs alongside the stdio pumps so process completion
	// surfaces via channel.close {reason, message} even while they are still draining.
	safego.GoNamed("plugin.channelExecWait", func() {
		stderrBuf := make([]byte, 0, stderrCaptureLimit)
		limited := io.LimitReader(stderr, stderrCaptureLimit)
		captured, _ := io.ReadAll(limited)
		stderrBuf = append(stderrBuf, captured...)

		waitErr := sess.Wait()
		reason := "exit"
		message := strings.TrimSpace(string(stderrBuf))
		if waitErr != nil {
			reason = "error"
			if message == "" {
				message = waitErr.Error()
			}
		}
		if b.closeNotifier != nil {
			b.closeNotifier(reason, message)
		}
		_ = b.CloseRemote()
	})

	return nil
}

// CloseRemote closes the SSH exec session, terminating the remote process. Idempotent: safe to
// call after an already-closed/never-opened backend.
func (b *ChannelExecBackend) CloseRemote() error {
	b.mu.Lock()
	sess := b.session
	b.session = nil
	alreadyClosed := b.closed
	b.closed = true
	b.mu.Unlock()

	if alreadyClosed || sess == nil {
		return nil
	}
	return sess.Close()
}

// shellQuoteArgv builds the single command string golang.org/x/crypto/ssh's Session.Start
// requires for an SSH "exec" request, from an already-authorized argv array.
//
// WHY this is still safe despite building a string: the "argv array, never a shell string"
// guarantee in ADR-011 is about the HOST never constructing a command by concatenating
// plugin-supplied fragments into a string it trusts as-is. The SSH wire protocol's exec request
// is unavoidably a single string — sshd hands it to the remote user's shell as `shell -c
// <string>` — so *some* serialization to a string is required to invoke it at all. The security
// property is preserved by POSIX single-quoting every argv element individually (wrapping each
// in '...', escaping embedded quotes as '\''): inside single quotes a POSIX shell disables all
// metacharacter interpretation (;, |, $, `, &&, etc.), so a malicious value like "x; rm -rf /"
// becomes the literal, inert argument 'x; rm -rf /' — not a second command. The argv array
// remains the sole source of truth; this function is purely a safe, mechanical serialization of
// it, never a place where plugin-supplied text is trusted to be syntax.
func shellQuoteArgv(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = shellQuoteOne(a)
	}
	return strings.Join(quoted, " ")
}

func shellQuoteOne(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
