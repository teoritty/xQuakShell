package transport_test

import (
	"io"
	"net/http"
	"testing"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/syncserver/transport"
)

// A request whose body is still arriving holds its slot. With every slot taken, the next request is
// told to retry rather than queued without bound, an event socket is let through, and a freed slot
// serves again.
func TestConcurrentRequestsAreBounded(t *testing.T) {
	r := newRigWith(t, func(c *transport.Config) { c.MaxConcurrent = 1 })
	alice := r.h.Register("alice")
	body, feed := io.Pipe()
	// Registered after the rig's own cleanup, so it runs first: a test failing with the slow request
	// still open must not leave the server's Close waiting on it.
	t.Cleanup(func() { _ = feed.Close() })
	slow, err := http.NewRequest(http.MethodPost, r.srv.URL+"/v1/hello", body)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan *http.Response, 1)
	go func() {
		resp, _ := http.DefaultClient.Do(slow)
		done <- resp
	}()
	if _, err := feed.Write([]byte(`{"clientProtocol":`)); err != nil {
		t.Fatal(err)
	}
	// The slow request reaches its handler, and takes the slot, a moment after its first bytes left
	// the client; ask until it has, rather than guess how long that takes.
	refused := false
	for range 500 {
		if status, _ := r.do("POST", "/v1/hello", "", syncwire.Hello{}); status == http.StatusTooManyRequests {
			refused = true
			break
		}
	}
	if !refused {
		t.Fatal("requests kept being served while another held the only slot")
	}
	conn, _, err := r.dialEvents(alice.Access, "")
	if err != nil {
		t.Fatalf("an event socket was held to the request bound: %v", err)
	}
	_ = conn.Close()
	_ = feed.Close()
	if resp := <-done; resp != nil {
		_ = resp.Body.Close()
	}
	if status, _ := r.do("GET", "/v1/vaults", alice.Access, nil); status != http.StatusOK {
		t.Fatalf("a request after the slot was freed: %d", status)
	}
}
