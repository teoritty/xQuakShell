package ssh

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
)

// stubHopClient stands in for a bastion. It records the context each hop dial
// ran under, which is what the chain used to throw away.
type stubHopClient struct {
	mu       sync.Mutex
	dialCtx  context.Context
	dialAddr string
	closed   bool
	conn     net.Conn
	err      error
}

func (c *stubHopClient) OpenDirectTCP(ctx context.Context, addr string) (net.Conn, error) {
	c.mu.Lock()
	c.dialCtx, c.dialAddr = ctx, addr
	c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.conn, c.err
}

func (c *stubHopClient) ListenTCP(context.Context, string) (net.Listener, error) {
	return nil, errors.New("not used")
}
func (c *stubHopClient) NewSession() (*gossh.Session, error) { return nil, errors.New("not used") }
func (c *stubHopClient) Client() *gossh.Client               { return nil }
func (c *stubHopClient) KeepAlive() error                    { return nil }

func (c *stubHopClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *stubHopClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *stubHopClient) seenCtx() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dialCtx
}

type stubHopFactory struct{ client *stubHopClient }

func (f stubHopFactory) Create(context.Context, domain.SSHClientConfig) (domain.SSHClient, error) {
	return f.client, nil
}

func noHopAuth(domain.JumpHop) ([]gossh.Signer, string, []domain.AuthMethod, func(), error) {
	return nil, "", nil, nil, nil
}

// The hop dial must run under the connect deadline the caller set. A raw
// client.Dial ignores it, so one stalled bastion outlives every configured
// timeout and hangs the whole chain.
func TestBuildTransportChainDialsTheHopUnderTheCallerContext(t *testing.T) {
	client := &stubHopClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transport, cleanup, err := BuildTransportChain(
		ctx,
		[]domain.JumpHop{{Host: "bastion.example", Port: 22, Username: "jump"}},
		"target.example", 22, 5,
		stubHopFactory{client: client},
		gossh.InsecureIgnoreHostKey(),
		noHopAuth,
	)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled; the hop dial ignored the caller's context", err)
	}
	if transport != nil {
		t.Errorf("transport = %v, want nil when the chain failed", transport)
	}
	if cleanup != nil {
		t.Error("cleanup must be nil on failure; the chain runs it itself before returning")
	}
	if got := client.seenCtx(); got == nil {
		t.Fatal("the hop dial received no context at all")
	}
	if !client.isClosed() {
		t.Error("the hop client was left open after a failed chain; cleanup did not run")
	}
}

// The address a hop forwards to is the next hop, or the target on the last one.
// Pinned here because the context change rewrote this call.
func TestBuildTransportChainForwardsToTheTarget(t *testing.T) {
	want := &trackedConn{}
	client := &stubHopClient{conn: want}

	transport, cleanup, err := BuildTransportChain(
		context.Background(),
		[]domain.JumpHop{{Host: "bastion.example", Port: 22, Username: "jump"}},
		"target.example", 2222, 5,
		stubHopFactory{client: client},
		gossh.InsecureIgnoreHostKey(),
		noHopAuth,
	)
	if err != nil {
		t.Fatalf("BuildTransportChain: %v", err)
	}
	defer cleanup()

	if transport != net.Conn(want) {
		t.Errorf("transport = %v, want the conn the hop dial produced", transport)
	}
	client.mu.Lock()
	addr := client.dialAddr
	client.mu.Unlock()
	if addr != "target.example:2222" {
		t.Errorf("hop dialed %q, want \"target.example:2222\"; the last hop forwards to the target", addr)
	}
}
