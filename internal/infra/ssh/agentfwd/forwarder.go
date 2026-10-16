package agentfwd

import (
	"context"
	"errors"
	"io"
	"log/slog"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
	"xquakshell/internal/pkg/safego"
)

const (
	channelType      = "auth-agent@openssh.com"
	forwardRequest   = "auth-agent-req@openssh.com"
	refusedByServer  = "server-refused-forwarding"
	refusedChannels  = "channel-limit"
	refusedAfterStop = "channel-after-session-end"
)

// ErrAlreadyAttached means the client already serves an agent. x/crypto allows one handler per
// channel type per client; a second Attach would otherwise silently get no channels at all.
var ErrAlreadyAttached = errors.New("agent forwarding: client already serves an agent")

// Attach serves a forwarded agent on client and asks the server to forward it to session.
//
// The channel handler is registered only here, so a client this was never called for has no
// handler for auth-agent@openssh.com and x/crypto rejects any such channel a server opens: a server
// cannot reach an agent nobody offered it.
//
// A server that refuses the forwarding request (AllowAgentForwarding no) does not fail the shell —
// the user still gets the terminal they asked for — but the refusal is audited, so a missing agent
// has an explanation in the log.
func Attach(ctx context.Context, client *gossh.Client, session *gossh.Session, policy domain.AgentSignPolicy) error {
	channels := client.HandleChannelOpen(channelType)
	if channels == nil {
		return ErrAlreadyAttached
	}
	safego.GoNamed("agentfwd.accept", func() { acceptChannels(ctx, channels, policy) })

	ok, err := session.SendRequest(forwardRequest, true, nil)
	if err != nil {
		return err
	}
	if !ok {
		slog.Warn("server refused agent forwarding")
		policy.Refuse(ctx, refusedByServer)
	}
	return nil
}

// acceptChannels serves at most MaxChannelsPerSession channels at once and refuses the rest.
//
// A refusal is audited before the server is told. In the other order the server already had its
// answer while the audit entry was still being written, so anything reacting to the refusal - a
// test's assertion, an operator reading the log - could find it missing.
func acceptChannels(ctx context.Context, channels <-chan gossh.NewChannel, policy domain.AgentSignPolicy) {
	slots := make(chan struct{}, agentforward.MaxChannelsPerSession)
	for nc := range channels {
		if ctx.Err() != nil {
			policy.Refuse(context.WithoutCancel(ctx), refusedAfterStop)
			rejectChannel(nc, gossh.Prohibited, "session ended")
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			policy.Refuse(ctx, refusedChannels)
			rejectChannel(nc, gossh.ResourceShortage, "too many agent channels")
			continue
		}
		ch, reqs, err := nc.Accept()
		if err != nil {
			<-slots
			continue
		}
		safego.GoNamed("agentfwd.requests", func() { gossh.DiscardRequests(reqs) })
		safego.GoNamed("agentfwd.serve", func() {
			defer func() { <-slots }()
			serveChannel(ctx, ch, policy)
		})
	}
}

func serveChannel(ctx context.Context, ch gossh.Channel, policy domain.AgentSignPolicy) {
	// Closing the channel is what unblocks a Serve waiting on a read when the session ends.
	stop := context.AfterFunc(ctx, func() { closeChannel(ch) })
	defer stop()
	defer closeChannel(ch)
	var bindings agentforward.Bindings
	if err := Serve(ctx, ch, policy, &bindings); err != nil && !errors.Is(err, context.Canceled) {
		slog.Debug("agent channel ended", "err", err)
	}
}

func rejectChannel(nc gossh.NewChannel, reason gossh.RejectionReason, message string) {
	if err := nc.Reject(reason, message); err != nil {
		slog.Debug("agent channel reject failed", "err", err)
	}
}

func closeChannel(ch gossh.Channel) {
	// x/crypto reports a second close as io.EOF; both deferred paths reach here on a normal end.
	if err := ch.Close(); err != nil && !errors.Is(err, io.EOF) {
		slog.Debug("agent channel close failed", "err", err)
	}
}
