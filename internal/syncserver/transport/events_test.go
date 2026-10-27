package transport_test

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/syncserver/servertest"
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

// expectClosed reads until the server ends the socket, failing if it is still open after a while.
func expectClosed(t *testing.T, conn *websocket.Conn, why string) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for {
		_, _, err := conn.ReadMessage()
		if err == nil {
			continue
		}
		if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
			t.Fatalf("socket %s ended with %v; want the server to close it", why, err)
		}
		return
	}
}

// The token is checked again while the socket is open: a socket checked only at the upgrade kept
// telling a frozen account, a revoked device or an expired session which vaults moved.
func TestEventsEndWithTheirCredentials(t *testing.T) {
	r := newRig(t)
	alice := r.h.Register("alice")
	conn, _, err := r.dialEvents(alice.Access, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := r.h.Svc.SetFrozen(servertest.Ctx, "alice", true); err != nil {
		t.Fatal(err)
	}
	expectClosed(t, conn, "of a frozen account")

	bob := r.h.Register("bob")
	conn2, _, err := r.dialEvents(bob.Access, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn2.Close() }()
	r.h.Clock.Advance(15*60_000 + 1)
	expectClosed(t, conn2, "with an expired token")
}

// A socket whose credentials stay valid is kept open across many re-checks.
func TestEventsStayOpenWhileTheTokenIsValid(t *testing.T) {
	r := newRig(t)
	alice := r.h.Register("alice")
	conn, _, err := r.dialEvents(alice.Access, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); !isTimeout(err) {
		t.Fatalf("a socket with a valid token ended: %v", err)
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
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
