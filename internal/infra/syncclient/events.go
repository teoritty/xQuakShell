package syncclient

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/pkg/safego"
)

const (
	eventFrameLimit = 4 << 10
	// eventIdleLimit is three of the server's 30-second pings: a connection silent for longer is
	// dead, and the scheduler reconnects rather than wait on it forever.
	eventIdleLimit = 90 * time.Second
)

var _ synctrust.EventSource = (*Client)(nil)

// Events opens the notification socket. It uses the same pinned TLS check as every request, and
// one malformed frame ends the stream: events are only hints to pull, so dropping a garbled stream
// loses nothing a reconnect does not recover.
func (c *Client) Events(ctx context.Context) (<-chan synctrust.RemoteEvent, error) {
	c.mu.Lock()
	token, trusted := c.access, c.trusted
	c.mu.Unlock()
	if trusted == nil {
		return nil, synctrust.ErrServerIdentity
	}
	u := *c.base
	u.Scheme = "wss"
	u = *u.JoinPath("/v1/events")
	d := websocket.Dialer{TLSClientConfig: c.tlsConfig(c.verifiedPeer), HandshakeTimeout: 10 * time.Second}
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	conn, resp, err := d.DialContext(ctx, u.String(), header)
	if resp != nil && err != nil {
		defer func() { _ = resp.Body.Close() }()
		return nil, remoteError(resp)
	}
	if err != nil {
		return nil, fmt.Errorf("syncclient: events: %w", err)
	}
	conn.SetReadLimit(eventFrameLimit)
	out := make(chan synctrust.RemoteEvent, 8)
	safego.GoNamed("syncclient.events", func() { readEvents(ctx, conn, out) })
	return out, nil
}

func readEvents(ctx context.Context, conn *websocket.Conn, out chan<- synctrust.RemoteEvent) {
	defer close(out)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer func() { _ = conn.Close() }()
	extend := func() error { return conn.SetReadDeadline(time.Now().Add(eventIdleLimit)) }
	conn.SetPingHandler(func(data string) error {
		if err := extend(); err != nil {
			return err
		}
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(10*time.Second))
	})
	for extend() == nil {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		ev, err := decodeEvent(raw)
		if err != nil {
			return
		}
		select {
		case out <- ev:
		case <-ctx.Done():
			return
		}
	}
}

func decodeEvent(raw []byte) (synctrust.RemoteEvent, error) {
	var w syncwire.Event
	if err := syncwire.Decode(bytes.NewReader(raw), eventFrameLimit, &w); err != nil {
		return synctrust.RemoteEvent{}, err
	}
	ev := synctrust.RemoteEvent{Seq: w.Seq, MembershipSeq: w.MembershipSeq, DeviceLogSeq: w.DeviceLogSeq}
	if w.Vault != "" {
		id, err := syncwire.ParseID(w.Vault)
		if err != nil {
			return synctrust.RemoteEvent{}, err
		}
		ev.Vault = id
	}
	return ev, nil
}
