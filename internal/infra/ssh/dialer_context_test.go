package ssh

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

// trackedConn records whether the dial result was closed, which is the only
// observable difference between draining an abandoned dial and leaking it.
type trackedConn struct {
	net.Conn
	mu     sync.Mutex
	closed bool
}

func (c *trackedConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *trackedConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func TestDialUnderContextReturnsTheDialResult(t *testing.T) {
	want := &trackedConn{}
	got, err := dialUnderContext(context.Background(), "test.dial", func() (net.Conn, error) {
		return want, nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != net.Conn(want) {
		t.Errorf("conn = %v, want the conn the dial produced", got)
	}
	if want.isClosed() {
		t.Error("a successful dial must not close the conn it hands back")
	}
}

func TestDialUnderContextPropagatesTheDialError(t *testing.T) {
	wantErr := errors.New("channel open failed")
	conn, err := dialUnderContext(context.Background(), "test.dial", func() (net.Conn, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want the dial's own error unwrapped", err)
	}
	if conn != nil {
		t.Errorf("conn = %v, want nil on a failed dial", conn)
	}
}

// A hung bastion is the case this exists for: without the context select the
// caller waits on Dial forever, and its own deadline never fires.
func TestDialUnderContextReturnsWhenTheContextIsCancelled(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	conn, err := dialUnderContext(ctx, "test.dial", func() (net.Conn, error) {
		<-release
		return &trackedConn{}, nil
	})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled; a hung dial must not outlast its context", err)
	}
	if conn != nil {
		t.Errorf("conn = %v, want nil when the context won the race", conn)
	}
	if elapsed > time.Second {
		t.Errorf("took %v to give up; the dial was waited on rather than abandoned", elapsed)
	}
}

// The dial keeps running after cancellation. Whatever it produces is a channel
// the peer believes is open, so it must be closed rather than dropped.
func TestDialUnderContextClosesTheConnThatArrivesAfterCancellation(t *testing.T) {
	release := make(chan struct{})
	late := &trackedConn{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := dialUnderContext(ctx, "test.dial", func() (net.Conn, error) {
		<-release
		return late, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	close(release)

	deadline := time.Now().Add(2 * time.Second)
	for !late.isClosed() {
		if time.Now().After(deadline) {
			t.Fatal("the conn produced after cancellation was never closed; a cancelled dial leaks one direct-tcpip channel per attempt")
		}
		time.Sleep(time.Millisecond)
	}
}
