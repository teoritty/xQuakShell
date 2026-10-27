package transport

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/pkg/safego"
	"xquakshell/internal/syncserver/usecase"
)

const (
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
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
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
	s.writeEvents(ctx, conn, events, token)
	cancel()
	stop()
	_ = conn.Close() // #nosec G104 -- the peer is gone or misbehaving; nothing to report to it
}

// writeEvents forwards events until the client leaves, the hub drops the subscription, the request
// context ends with the server shutting down, or the token stops authenticating.
//
// The token is checked again on every ping, not only at the upgrade. A socket that outlived its
// credentials kept telling a revoked device, or one of a frozen account, which vaults moved and
// when; the device then reconnects with a fresh token, or cannot.
func (s *server) writeEvents(ctx context.Context, conn *websocket.Conn, events <-chan usecase.Event, token string) {
	ping := time.NewTicker(s.cfg.EventPing)
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
			if _, err := s.svc.Authenticate(ctx, token, usecase.TokenAccess); err != nil {
				closeEvents(conn)
				return
			}
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(eventWriteTimeout)) != nil {
				return
			}
		}
	}
}

// closeEvents tells the client the socket ends on purpose, so it reconnects - and authenticates
// afresh - rather than wait out its idle limit on a dead connection.
func closeEvents(conn *websocket.Conn) {
	msg := websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "credentials no longer valid")
	_ = conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(eventWriteTimeout)) // #nosec G104 -- the socket closes either way
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
