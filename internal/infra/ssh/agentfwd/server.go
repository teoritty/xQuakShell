// Package agentfwd speaks the SSH agent protocol on forwarded agent channels (ADR-022).
//
// It owns no rule about what may be signed: every request is handed to a domain.AgentSignPolicy.
// What it does own is the protocol boundary — which messages are read at all, how large they may
// be, and how a session-bind is verified — because those are properties of bytes on the wire.
package agentfwd

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
)

// Agent protocol message numbers (draft-miller-ssh-agent).
const (
	agentFailure           = 5
	agentSuccess           = 6
	agentRequestIdentities = 11
	agentIdentitiesAnswer  = 12
	agentSignRequest       = 13
	agentSignResponse      = 14
	agentExtension         = 27
	agentExtensionFailure  = 28
)

// refusedOps names the requests this agent declines without parsing, for the audit log. Anything
// not listed and not served is "unknown".
var refusedOps = map[byte]string{
	1:  "request-v1-identities",
	17: "add-identity",
	18: "remove-identity",
	19: "remove-all-identities",
	20: "add-smartcard-key",
	21: "remove-smartcard-key",
	22: "lock",
	23: "unlock",
	25: "add-identity-constrained",
	26: "add-smartcard-key-constrained",
}

// errFrame ends a channel whose framing cannot be trusted any more. After a bad length prefix
// there is no way to find the start of the next message, so the only safe move is to stop.
var errFrame = errors.New("agent forwarding: bad frame")

// Serve answers agent requests on one channel until it closes, ctx ends, or a frame is malformed.
//
// It is deliberately not agent.ServeAgent. That server accepts frames up to 16 MiB, parses the
// private key in an add-identity request before handing it to the agent to refuse, and writes its
// errors to the standard logger. This loop reads at most MaxFrameLen, dispatches exactly three
// message types, and answers every other one with failure before looking inside it.
func Serve(ctx context.Context, rw io.ReadWriter, policy domain.AgentSignPolicy, bindings *agentforward.Bindings) error {
	for ctx.Err() == nil {
		frame, err := readFrame(rw)
		if err != nil {
			if errors.Is(err, errFrame) {
				policy.Refuse(ctx, "bad-frame")
			}
			return err
		}
		if err := writeFrame(rw, handle(ctx, frame, policy, bindings)); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func readFrame(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n == 0 || n > agentforward.MaxFrameLen {
		return nil, fmt.Errorf("%w: length %d", errFrame, n)
	}
	frame := make([]byte, n)
	if _, err := io.ReadFull(r, frame); err != nil {
		return nil, err
	}
	return frame, nil
}

func writeFrame(w io.Writer, msg []byte) error {
	out := make([]byte, 4+len(msg))
	binary.BigEndian.PutUint32(out, uint32(len(msg)))
	copy(out[4:], msg)
	_, err := w.Write(out)
	return err
}

func handle(ctx context.Context, frame []byte, policy domain.AgentSignPolicy, bindings *agentforward.Bindings) []byte {
	switch frame[0] {
	case agentRequestIdentities:
		return listIdentities(ctx, frame, policy)
	case agentSignRequest:
		return sign(ctx, frame, policy, bindings)
	case agentExtension:
		return extension(ctx, frame, policy, bindings)
	default:
		op, known := refusedOps[frame[0]]
		if !known {
			op = "unknown"
		}
		policy.Refuse(ctx, op)
		return []byte{agentFailure}
	}
}

func listIdentities(ctx context.Context, frame []byte, policy domain.AgentSignPolicy) []byte {
	if len(frame) != 1 {
		return []byte{agentFailure}
	}
	keys, err := policy.PublicKeys(ctx)
	if err != nil {
		return []byte{agentFailure}
	}
	out := []byte{agentIdentitiesAnswer}
	out = binary.BigEndian.AppendUint32(out, uint32(len(keys)))
	for _, k := range keys {
		// The comment is always empty; see domain.AgentPublicKey.
		out = appendString(appendString(out, k.Blob), nil)
	}
	return out
}

type signRequestMsg struct {
	KeyBlob []byte `sshtype:"13"`
	Data    []byte
	Flags   uint32
}

func sign(ctx context.Context, frame []byte, policy domain.AgentSignPolicy, bindings *agentforward.Bindings) []byte {
	var req signRequestMsg
	if err := gossh.Unmarshal(frame, &req); err != nil {
		policy.Refuse(ctx, "malformed-sign-request")
		return []byte{agentFailure}
	}
	sig, err := policy.Sign(ctx, domain.AgentSignRequest{
		KeyBlob: req.KeyBlob, Data: req.Data, Flags: agentforward.Flags(req.Flags), Bindings: bindings,
	})
	if err != nil {
		return []byte{agentFailure}
	}
	return appendString([]byte{agentSignResponse}, gossh.Marshal(sig))
}

type extensionMsg struct {
	ExtensionType string `sshtype:"27"`
	Contents      []byte `ssh:"rest"`
}

func extension(ctx context.Context, frame []byte, policy domain.AgentSignPolicy, bindings *agentforward.Bindings) []byte {
	var ext extensionMsg
	if err := gossh.Unmarshal(frame, &ext); err != nil {
		policy.Refuse(ctx, "malformed-extension")
		return []byte{agentFailure}
	}
	if ext.ExtensionType != sessionBindExtension {
		// PROTOCOL.agent: an unsupported extension is a plain failure, which is also what
		// OpenSSH's "query" probe expects to receive from an agent that supports nothing else.
		return []byte{agentFailure}
	}
	binding, err := verifySessionBind(ext.Contents)
	if err == nil {
		err = bindings.Add(binding)
	}
	if err != nil {
		policy.Refuse(ctx, "session-bind")
		return []byte{agentExtensionFailure}
	}
	return []byte{agentSuccess}
}

func appendString(out, s []byte) []byte {
	out = binary.BigEndian.AppendUint32(out, uint32(len(s)))
	return append(out, s...)
}
