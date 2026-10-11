package agentfwd

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const waitTimeout = 10 * time.Second

// agentServer is an SSH server that plays the remote side of forwarding: it answers the client's
// auth-agent-req and opens auth-agent@openssh.com channels back to it, the way sshd does whenever
// a process on the server connects to SSH_AUTH_SOCK.
type agentServer struct {
	allowForwarding bool
	conns           chan *gossh.ServerConn
	forwardAsked    chan bool
}

func startAgentServer(t *testing.T, allowForwarding bool) (*agentServer, *gossh.Client) {
	t.Helper()
	host := newEd25519(t)
	srv := &agentServer{allowForwarding: allowForwarding, conns: make(chan *gossh.ServerConn, 1), forwardAsked: make(chan bool, 1)}
	cfg := &gossh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(host)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		sc, chans, reqs, err := gossh.NewServerConn(c, cfg)
		if err != nil {
			return
		}
		srv.conns <- sc
		go gossh.DiscardRequests(reqs)
		for nc := range chans {
			ch, chReqs, err := nc.Accept()
			if err != nil {
				continue
			}
			go srv.sessionRequests(ch, chReqs)
		}
	}()
	client, err := gossh.Dial("tcp", ln.Addr().String(), &gossh.ClientConfig{
		User: "ops", HostKeyCallback: gossh.FixedHostKey(host.PublicKey()), Timeout: waitTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return srv, client
}

func (s *agentServer) sessionRequests(ch gossh.Channel, reqs <-chan *gossh.Request) {
	defer ch.Close()
	for r := range reqs {
		ok := true
		if r.Type == forwardRequest {
			ok = s.allowForwarding
			s.forwardAsked <- true
		}
		if r.WantReply {
			_ = r.Reply(ok, nil)
		}
	}
}

func (s *agentServer) conn(t *testing.T) *gossh.ServerConn {
	t.Helper()
	select {
	case c := <-s.conns:
		s.conns <- c
		return c
	case <-time.After(waitTimeout):
		t.Fatal("server never completed the handshake")
		return nil
	}
}

// openAgent is what sshd does when a process on the server opens SSH_AUTH_SOCK.
func (s *agentServer) openAgent(t *testing.T) (gossh.Channel, error) {
	t.Helper()
	ch, reqs, err := s.conn(t).OpenChannel(channelType, nil)
	if err != nil {
		return nil, err
	}
	go gossh.DiscardRequests(reqs)
	return ch, nil
}

func attach(t *testing.T, ctx context.Context, client *gossh.Client, p *fakePolicy) *gossh.Session {
	t.Helper()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if err := Attach(ctx, client, session, p); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	return session
}

func TestForwardedAgentServesTheServer(t *testing.T) {
	srv, client := startAgentServer(t, true)
	p := &fakePolicy{key: newEd25519(t)}
	attach(t, context.Background(), client, p)
	ch, err := srv.openAgent(t)
	if err != nil {
		t.Fatalf("the server could not reach the agent it was offered: %v", err)
	}
	remote := agent.NewClient(ch)
	data := []byte("login")
	sig, err := remote.Sign(p.key.PublicKey(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.key.PublicKey().Verify(data, sig); err != nil {
		t.Fatalf("the signature that crossed the channel does not verify: %v", err)
	}
}

func TestAServerCannotReachAnAgentNobodyOffered(t *testing.T) {
	srv, _ := startAgentServer(t, true)
	_, err := srv.openAgent(t)
	var openErr *gossh.OpenChannelError
	if !errors.As(err, &openErr) || openErr.Reason != gossh.UnknownChannelType {
		t.Fatalf("err = %v; a client that never attached must reject agent channels outright", err)
	}
}

func TestAttachTwiceIsRefused(t *testing.T) {
	_, client := startAgentServer(t, true)
	p := &fakePolicy{key: newEd25519(t)}
	attach(t, context.Background(), client, p)
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := Attach(context.Background(), client, session, p); !errors.Is(err, ErrAlreadyAttached) {
		t.Fatalf("err = %v; a second agent on one client would silently receive nothing", err)
	}
}

func TestAServerRefusingForwardingIsAuditedNotFatal(t *testing.T) {
	srv, client := startAgentServer(t, false)
	p := &fakePolicy{key: newEd25519(t)}
	attach(t, context.Background(), client, p)
	<-srv.forwardAsked
	if r := p.refusals(); len(r) != 1 || r[0] != refusedByServer {
		t.Fatalf("refusals %v; a server that declines forwarding must leave a trace", r)
	}
}

func TestForwardedAgentCapsConcurrentChannels(t *testing.T) {
	srv, client := startAgentServer(t, true)
	p := &fakePolicy{key: newEd25519(t)}
	attach(t, context.Background(), client, p)
	var open []gossh.Channel
	for i := range 8 {
		ch, err := srv.openAgent(t)
		if err != nil {
			t.Fatalf("channel %d of 8 refused: %v", i+1, err)
		}
		open = append(open, ch)
	}
	_, err := srv.openAgent(t)
	var openErr *gossh.OpenChannelError
	if !errors.As(err, &openErr) || openErr.Reason != gossh.ResourceShortage {
		t.Fatalf("err = %v; a ninth concurrent agent channel must be refused", err)
	}
	if r := p.refusals(); len(r) != 1 || r[0] != refusedChannels {
		t.Fatalf("refusals %v; the refused channel must be audited", r)
	}
	for _, ch := range open {
		_ = ch.Close()
	}
}

func TestEndingTheSessionClosesItsAgentChannels(t *testing.T) {
	srv, client := startAgentServer(t, true)
	p := &fakePolicy{key: newEd25519(t)}
	ctx, cancel := context.WithCancel(context.Background())
	attach(t, ctx, client, p)
	ch, err := srv.openAgent(t)
	if err != nil {
		t.Fatal(err)
	}
	if reply := rawRequest(t, ch, []byte{agentRequestIdentities}); reply[0] != agentIdentitiesAnswer {
		t.Fatalf("the agent did not answer before the session ended: %v", reply)
	}
	cancel()
	ended := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		for {
			if _, err := ch.Read(buf); err != nil {
				ended <- err
				return
			}
		}
	}()
	select {
	case <-ended:
	case <-time.After(waitTimeout):
		t.Fatal("an agent channel outlived its session")
	}
	if _, err := srv.openAgent(t); err == nil {
		t.Fatal("a channel opened after the session ended was accepted")
	}
}
