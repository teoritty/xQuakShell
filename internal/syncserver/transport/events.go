package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/pkg/safego"
	"xquakshell/internal/syncserver/usecase"
)

const (
	eventPingInterval = 30 * time.Second
	eventWriteTimeout = 10 * time.Second
	eventReadLimit    = 512
)

// upgrader refuses any request carrying an Origin header. The clients are native programs that send
// none; a browser always does, and a web page has no business opening a socket to a sync server.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" },
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	p, ok := s.authenticate(w, r, usecase.TokenAccess)
	if !ok {
		return
	}
	events, cancel, err := s.svc.Subscribe(p.Account)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		cancel()
		return
	}
	ctx, stop := context.WithCancel(r.Context())
	conn.SetReadLimit(eventReadLimit)
	// The reader exists only to notice the client going away: clients send nothing meaningful, and
	// any frame over the read limit closes the socket.
	safego.GoNamed("syncserver.events.read", func() {
		defer stop()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	s.writeEvents(ctx, conn, events)
	cancel()
	stop()
	_ = conn.Close() // #nosec G104 -- the peer is gone or misbehaving; nothing to report to it
}

// writeEvents forwards events until the client leaves, the hub drops the subscription, or the
// request context ends with the server shutting down.
func (s *server) writeEvents(ctx context.Context, conn *websocket.Conn, events <-chan usecase.Event) {
	ping := time.NewTicker(eventPingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, open := <-events:
			if !open {
				return
			}
			if !writeFrame(conn, eventFrame(ev)) {
				return
			}
		case <-ping.C:
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(eventWriteTimeout)) != nil {
				return
			}
		}
	}
}

func writeFrame(conn *websocket.Conn, v any) bool {
	if conn.SetWriteDeadline(time.Now().Add(eventWriteTimeout)) != nil {
		return false
	}
	return conn.WriteJSON(v) == nil
}

func eventFrame(ev usecase.Event) syncwire.Event {
	out := syncwire.Event{Seq: ev.Seq, MembershipSeq: ev.MembershipSeq, DeviceLogSeq: ev.DeviceLogSeq}
	if !ev.Vault.IsZero() {
		out.Vault = ev.Vault.String()
	}
	if !ev.Account.IsZero() {
		out.Account = ev.Account.String()
	}
	return out
}
