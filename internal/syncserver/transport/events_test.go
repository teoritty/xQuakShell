package transport_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"xquakshell/internal/domain/syncwire"
)

func (r *rig) dialEvents(token, origin string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		header.Set("Origin", origin)
	}
	return websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(r.srv.URL, "http")+"/v1/events", header)
}

func TestEventsReachTheSubscribedDevice(t *testing.T) {
	r := newRig(t)
	alice := r.h.Register("alice")
	conn, _, err := r.dialEvents(alice.Access, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	cv := r.h.PersonalVault(alice)
	// The subscription is registered before the upgrade completes, so a commit after Dial returns
	// cannot be missed.
	cv.Accept(alice, cv.Commit(alice, cv.Record(alice, 1)))
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var ev syncwire.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("no event frame: %v", err)
	}
	if ev.Vault != alice.Vault.String() || ev.Seq != 1 {
		t.Fatalf("event = %+v", ev)
	}
}

func TestEventsRequireAnAccessTokenAndNoOrigin(t *testing.T) {
	r := newRig(t)
	alice := r.h.Register("alice")
	if _, resp, err := r.dialEvents("", ""); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous socket: %v, %v", err, resp)
	}
	if _, resp, err := r.dialEvents(alice.Access, "https://evil.example"); err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a browser origin opened a socket: %v, %v", err, resp)
	}
}

func TestEventsCapPerAccount(t *testing.T) {
	r := newRig(t)
	alice := r.h.Register("alice")
	var conns []*websocket.Conn
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	for i := 0; i < 16; i++ {
		c, _, err := r.dialEvents(alice.Access, "")
		if err != nil {
			t.Fatalf("socket %d: %v", i+1, err)
		}
		conns = append(conns, c)
	}
	if _, resp, err := r.dialEvents(alice.Access, ""); err == nil || resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("17th socket: %v, %v", err, resp)
	}
}
