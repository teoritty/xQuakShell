package ipc

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

// The tests here exercise the seam between the bus (Conn/mux/channel/credit) and its consumers
// over a real Conn, with a fake plugin on the other end of real pipes. Every layer below is
// covered by unit tests already; those pass a fake ChannelDataPath to the backends and a fake
// writeFrame to the channels — substituting, on both sides, exactly the adapter that did not
// exist. Each half was proven solid and the halves did not meet: channel.open returned an id
// nothing was wired to, and the mux was registered with nothing, so the first frame killed the
// process. A unit test cannot catch that by construction. These can.

// fakePlugin is the far end of the pipe: it speaks frames, tracks its own send window the way a
// real plugin must, and records what the host sent it.
type fakePlugin struct {
	fw   *FrameWriter
	r    io.Reader
	t    *testing.T
	done chan struct{}

	mu       sync.Mutex
	received [][]byte
	credit   int // how many frames the host has granted us
	granted  chan struct{}
}

func newFakePlugin(t *testing.T, r io.Reader, w io.Writer, initialCredit int) *fakePlugin {
	p := &fakePlugin{
		fw:      NewFrameWriter(w),
		r:       r,
		t:       t,
		done:    make(chan struct{}),
		credit:  initialCredit,
		granted: make(chan struct{}, 1024),
	}
	go p.readLoop()
	return p
}

// readLoop consumes what the host sends: data frames are recorded, credit frames widen our
// window exactly as ADR-011 requires a plugin to do.
func (p *fakePlugin) readLoop() {
	defer close(p.done)
	for {
		hdr, payload, err := ReadFrame(p.r)
		if err != nil {
			return
		}
		switch hdr.Kind {
		case domainplugin.FrameKindBinary:
			p.mu.Lock()
			p.received = append(p.received, append([]byte(nil), payload...))
			p.mu.Unlock()
		case domainplugin.FrameKindCredit:
			p.mu.Lock()
			p.credit++
			p.mu.Unlock()
			select {
			case p.granted <- struct{}{}:
			default:
			}
		}
	}
}

func (p *fakePlugin) receivedFrames() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]byte, len(p.received))
	copy(out, p.received)
	return out
}

// sendRespectingCredit sends one data frame, waiting for a grant if our window is shut. A plugin
// that ignored this would be killed by the host, which is the intended contract.
func (p *fakePlugin) sendRespectingCredit(channelID uint32, payload []byte) error {
	for {
		p.mu.Lock()
		if p.credit > 0 {
			p.credit--
			p.mu.Unlock()
			return p.fw.Write(domainplugin.FrameKindBinary, channelID, payload)
		}
		p.mu.Unlock()
		select {
		case <-p.granted:
		case <-time.After(2 * time.Second):
			return fmt.Errorf("plugin never received credit to send; the host is not granting")
		}
	}
}

// grantCredit tells the host it may send us one more frame.
func (p *fakePlugin) grantCredit(channelID uint32, n uint32) error {
	payload := make([]byte, 8)
	payload[0] = byte(channelID >> 24)
	payload[1] = byte(channelID >> 16)
	payload[2] = byte(channelID >> 8)
	payload[3] = byte(channelID)
	payload[4] = byte(n >> 24)
	payload[5] = byte(n >> 16)
	payload[6] = byte(n >> 8)
	payload[7] = byte(n)
	return p.fw.Write(domainplugin.FrameKindCredit, channelID, payload)
}

func newSeamConn(t *testing.T) (*Conn, *fakePlugin) {
	t.Helper()
	pluginOutR, pluginOutW := io.Pipe() // plugin -> host
	hostOutR, hostOutW := io.Pipe()     // host -> plugin

	conn := NewConn(pluginOutR, hostOutW, nil, nil, 0)
	t.Cleanup(conn.Close)

	plugin := newFakePlugin(t, hostOutR, pluginOutW, domainplugin.InitialCredit(domainplugin.PurposeExec))
	t.Cleanup(func() { _ = pluginOutW.Close() })
	return conn, plugin
}

// TestSeamPluginToHostFlowsPastInitialCredit is the F-3 regression at the seam. A run of only
// InitialCredit frames passes even with credit replenishment completely broken — the window
// simply never has to reopen. The boundary is the frame after it, which is why this sends
// several windows' worth.
func TestSeamPluginToHostFlowsPastInitialCredit(t *testing.T) {
	conn, plugin := newSeamConn(t)

	data, err := conn.OpenDataPath(1, domainplugin.PurposeExec)
	if err != nil {
		t.Fatalf("open data path: %v", err)
	}

	total := domainplugin.InitialCredit(domainplugin.PurposeExec) * 3
	received := make(chan []byte, total)
	go func() {
		for {
			payload, ok := data.Recv()
			if !ok {
				return
			}
			received <- payload
			// Ack is what returns credit; without it the plugin stalls at the first window.
			if err := data.Ack(context.Background()); err != nil {
				return
			}
		}
	}()

	for i := 0; i < total; i++ {
		if err := plugin.sendRespectingCredit(1, []byte(fmt.Sprintf("frame-%d", i))); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}

	for i := 0; i < total; i++ {
		select {
		case got := <-received:
			want := fmt.Sprintf("frame-%d", i)
			if string(got) != want {
				t.Fatalf("frame %d = %q, want %q", i, got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("only %d of %d frames arrived; the credit window never reopened", i, total)
		}
	}
}

// TestSeamHostToPluginFlowsPastInitialCredit is the same boundary in the other direction: the
// host's Send must block at the window edge and resume on the plugin's kind=0x03 grant.
func TestSeamHostToPluginFlowsPastInitialCredit(t *testing.T) {
	conn, plugin := newSeamConn(t)

	data, err := conn.OpenDataPath(1, domainplugin.PurposeExec)
	if err != nil {
		t.Fatalf("open data path: %v", err)
	}
	// OpenDataPath gates outbound sends until the channel.open reply is on the wire; in
	// production Conn.handleIncomingRequest releases it after that reply. This test drives the
	// data path directly, so it stands in for that step.
	conn.mux.MarkOpened(1)

	total := domainplugin.InitialCredit(domainplugin.PurposeExec) * 3
	sendErr := make(chan error, 1)
	go func() {
		for i := 0; i < total; i++ {
			if err := data.Send(context.Background(), []byte(fmt.Sprintf("out-%d", i))); err != nil {
				sendErr <- err
				return
			}
		}
		sendErr <- nil
	}()

	// Feed credit as the plugin consumes, exactly as a real one would.
	deadline := time.After(3 * time.Second)
	for len(plugin.receivedFrames()) < total {
		select {
		case <-deadline:
			t.Fatalf("host delivered only %d of %d frames; Send never resumed past the initial window",
				len(plugin.receivedFrames()), total)
		default:
		}
		if err := plugin.grantCredit(1, 1); err != nil {
			t.Fatalf("grant credit: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	if err := <-sendErr; err != nil {
		t.Fatalf("host send: %v", err)
	}

	for i, got := range plugin.receivedFrames()[:total] {
		want := fmt.Sprintf("out-%d", i)
		if string(got) != want {
			t.Fatalf("plugin frame %d = %q, want %q", i, got, want)
		}
	}
}

// TestSeamOpenDataPathRejectsUnknownPurpose: InitialCredit returns 0 for a purpose the host has
// no policy for, which would build a channel whose first Send blocks forever and whose first
// inbound frame kills the plugin. It must be refused at the door instead.
// TestSeamOutboundBlocksUntilOpened is the VNC-crash regression: a channel opened via
// OpenDataPath must not emit host->plugin data until MarkOpened (in production, once the
// channel.open reply is on the wire). A VNC server speaks first, so the relay pump reads the
// RFB banner and calls Send immediately; before this gate that frame reached the plugin ahead
// of the reply, for a channelId the plugin had not yet registered — a fatal protocol violation.
func TestSeamOutboundBlocksUntilOpened(t *testing.T) {
	conn, plugin := newSeamConn(t)

	data, err := conn.OpenDataPath(1, domainplugin.PurposeExec)
	if err != nil {
		t.Fatalf("open data path: %v", err)
	}

	sent := make(chan error, 1)
	go func() { sent <- data.Send(context.Background(), []byte("banner")) }()

	select {
	case <-sent:
		t.Fatal("Send emitted before MarkOpened; a data frame can beat the channel.open reply")
	case <-time.After(100 * time.Millisecond):
		// Correctly blocked.
	}

	conn.mux.MarkOpened(1)

	select {
	case err := <-sent:
		if err != nil {
			t.Fatalf("Send after MarkOpened: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send stayed blocked after MarkOpened")
	}
	_ = plugin
}

func TestSeamOpenDataPathRejectsUnknownPurpose(t *testing.T) {
	conn, _ := newSeamConn(t)

	if _, err := conn.OpenDataPath(1, "not-a-purpose"); err == nil {
		t.Fatal("OpenDataPath accepted a purpose with no credit policy; the channel would be born dead")
	}
}

// TestSeamPluginExceedingCreditIsProtocolViolation proves the defence is actually armed: the
// unbounded host-side queue it replaces was reachable by any plugin, compromised or merely fast.
func TestSeamPluginExceedingCreditIsProtocolViolation(t *testing.T) {
	conn, plugin := newSeamConn(t)

	if _, err := conn.OpenDataPath(1, domainplugin.PurposeExec); err != nil {
		t.Fatalf("open data path: %v", err)
	}

	// Nothing Acks, so the window never reopens: send straight past it, ignoring credit.
	//
	// The flood runs in its own goroutine because the pipe is synchronous: once the host fails
	// the connection it stops reading, and the writes that follow block forever. Blocking there
	// is the correct end state — it just cannot happen on the goroutine doing the asserting.
	over := domainplugin.InitialCredit(domainplugin.PurposeExec) + 2
	go func() {
		for i := 0; i < over; i++ {
			if err := plugin.fw.Write(domainplugin.FrameKindBinary, 1, []byte("flood")); err != nil {
				return
			}
		}
	}()

	deadline := time.After(3 * time.Second)
	for {
		if err := conn.ReadError(); err != nil {
			return // the connection was failed, as it must be
		}
		select {
		case <-deadline:
			t.Fatal("plugin sent past its granted credit and the host accepted it; the inbound queue is unbounded again")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
