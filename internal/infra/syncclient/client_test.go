package syncclient_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/syncclient"
)

var ctx = context.Background()

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv, Leaf: leaf}
}

// fakeServer is a TLS server that answers hello the way xqsyncd does, signing the SPKI of whichever
// certificate it currently presents unless a test swaps the signature out.
type fakeServer struct {
	t        *testing.T
	srv      *httptest.Server
	url      string
	mux      *http.ServeMux
	identity ed25519.PrivateKey
	mu       sync.Mutex
	cert     tls.Certificate
	signed   []byte // SPKI the hello signature covers
	reply    func(*syncwire.HelloReply)
	hits     atomic.Int32
	lastAuth atomic.Value
}

func newFake(t *testing.T) *fakeServer {
	t.Helper()
	_, id, _ := ed25519.GenerateKey(rand.Reader)
	f := &fakeServer{t: t, mux: http.NewServeMux(), identity: id, cert: selfSigned(t)}
	f.signed = f.cert.Leaf.RawSubjectPublicKeyInfo
	f.mux.HandleFunc("POST /v1/hello", f.hello)
	f.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.lastAuth.Store(r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/hello" {
			f.hits.Add(1)
		}
		f.mux.ServeHTTP(w, r)
	}))
	// StartTLS would install httptest's own certificate, which wins over GetCertificate whenever the
	// client sends no SNI - and a client dialling 127.0.0.1 sends none. Wrapping the listener keeps
	// the certificate the test chose, including after rotateCertificate.
	f.srv.Listener = tls.NewListener(f.srv.Listener, &tls.Config{GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		c := f.cert
		return &c, nil
	}})
	f.srv.Start()
	f.url = "https://" + f.srv.Listener.Addr().String()
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeServer) hello(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	signed := f.signed
	f.mu.Unlock()
	msg, _ := vaultsync.ServerTLSMessage(signed)
	v := syncwire.Version{Major: vaultsync.CurrentProtocol.Major, Minor: vaultsync.CurrentProtocol.Minor, Patch: vaultsync.CurrentProtocol.Patch}
	reply := syncwire.HelloReply{ServerMin: v, ServerMax: v, Suites: []uint16{uint16(vaultsync.CurrentSuite)},
		ServerIdentityKey: f.identity.Public().(ed25519.PublicKey), TLSKeySig: ed25519.Sign(f.identity, msg)}
	if f.reply != nil {
		f.reply(&reply)
	}
	writeJSON(w, http.StatusOK, reply)
}

func (f *fakeServer) rotateCertificate(signNew bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cert = selfSigned(f.t)
	if signNew {
		f.signed = f.cert.Leaf.RawSubjectPublicKeyInfo
	}
}

func (f *fakeServer) pub() []byte { return f.identity.Public().(ed25519.PublicKey) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func newClient(t *testing.T, f *fakeServer, pinned []byte) *syncclient.Client {
	t.Helper()
	c, err := syncclient.New(syncclient.Config{BaseURL: f.url, PinnedIdentity: pinned})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestTrustOnFirstUseReturnsTheIdentity(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, nil)
	got, err := c.Hello(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.IdentityKey, f.pub()) {
		t.Fatal("hello did not report the server's identity key for the user to compare")
	}
}

func TestPinnedIdentity(t *testing.T) {
	f := newFake(t)
	if _, err := newClient(t, f, f.pub()).Hello(ctx); err != nil {
		t.Fatalf("matching pin refused: %v", err)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := newClient(t, f, other).Hello(ctx); !errors.Is(err, synctrust.ErrServerIdentity) {
		t.Fatalf("a server with another identity key: %v, want ErrServerIdentity", err)
	}
}

// A man in the middle can replay the real server's hello, signature and all, but the signature
// covers the real server's TLS key, not the one the client is talking to.
func TestReplayedSignatureForAnotherTLSKeyIsRefused(t *testing.T) {
	f := newFake(t)
	f.signed = selfSigned(t).Leaf.RawSubjectPublicKeyInfo
	if _, err := newClient(t, f, f.pub()).Hello(ctx); !errors.Is(err, synctrust.ErrServerIdentity) {
		t.Fatalf("err = %v, want ErrServerIdentity", err)
	}
}

func TestBadSignatureIsRefused(t *testing.T) {
	f := newFake(t)
	f.reply = func(r *syncwire.HelloReply) { r.TLSKeySig[0] ^= 1 }
	if _, err := newClient(t, f, nil).Hello(ctx); !errors.Is(err, synctrust.ErrServerIdentity) {
		t.Fatalf("err = %v, want ErrServerIdentity even on first use", err)
	}
}

func TestNothingIsSentBeforeHello(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, f.pub())
	if _, _, err := c.Changes(ctx, vaultsync.ID{1}, 0, 10); !errors.Is(err, synctrust.ErrServerIdentity) {
		t.Fatalf("err = %v, want ErrServerIdentity", err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("a request reached a server whose TLS key was never verified")
	}
}

// Every connection is checked against the verified key, not just the first: a certificate swapped
// after hello reaches no handler until a new hello verifies it.
func TestCertificateSwappedAfterHelloIsRefused(t *testing.T) {
	f := newFake(t)
	f.mux.HandleFunc("GET /v1/vaults", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, syncwire.VaultList{}) })
	c := newClient(t, f, f.pub())
	if _, err := c.Hello(ctx); err != nil {
		t.Fatal(err)
	}
	f.rotateCertificate(false)
	c.CloseIdleConnections()
	if _, err := c.Vaults(ctx); !errors.Is(err, synctrust.ErrServerIdentity) {
		t.Fatalf("err = %v, want ErrServerIdentity", err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("a request reached a server presenting an unverified certificate")
	}
	f.rotateCertificate(true)
	if _, err := c.Hello(ctx); err != nil {
		t.Fatalf("a legitimately renewed certificate did not verify: %v", err)
	}
	if _, err := c.Vaults(ctx); err != nil {
		t.Fatalf("after re-verification: %v", err)
	}
}

func TestPlainHTTPIsRefused(t *testing.T) {
	for _, u := range []string{"http://example.com", "example.com", "ftp://x", "https://"} {
		if _, err := syncclient.New(syncclient.Config{BaseURL: u}); err == nil {
			t.Errorf("%q accepted; the transport must be TLS", u)
		}
	}
}

func TestVersionAndSuiteNegotiation(t *testing.T) {
	for name, mutate := range map[string]func(*syncwire.HelloReply){
		"server too new":  func(r *syncwire.HelloReply) { r.ServerMin.Major++; r.ServerMax.Major++ },
		"server too old":  func(r *syncwire.HelloReply) { r.ServerMin.Major--; r.ServerMax.Major-- },
		"no shared suite": func(r *syncwire.HelloReply) { r.Suites = []uint16{999} },
	} {
		f := newFake(t)
		f.reply = mutate
		if _, err := newClient(t, f, nil).Hello(ctx); !errors.Is(err, synctrust.ErrRemoteUnsupported) {
			t.Errorf("%s: err = %v, want ErrRemoteUnsupported", name, err)
		}
	}
}

func helloed(t *testing.T, f *fakeServer) *syncclient.Client {
	t.Helper()
	c := newClient(t, f, f.pub())
	if _, err := c.Hello(ctx); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestErrorCodesMapToDomainErrors(t *testing.T) {
	for code, want := range map[string]error{
		syncwire.CodeMalformed:          synctrust.ErrRemoteMalformed,
		syncwire.CodeUnauthorized:       synctrust.ErrRemoteUnauthorized,
		syncwire.CodeForbidden:          synctrust.ErrRemoteForbidden,
		syncwire.CodeNotFound:           synctrust.ErrRemoteNotFound,
		syncwire.CodeTooLarge:           synctrust.ErrRemoteTooLarge,
		syncwire.CodeUnsupportedVersion: synctrust.ErrRemoteUnsupported,
		syncwire.CodeFrozen:             synctrust.ErrRemoteFrozen,
		syncwire.CodeQuotaExceeded:      synctrust.ErrRemoteQuota,
		syncwire.CodeInternal:           synctrust.ErrRemoteInternal,
		syncwire.CodeHistoryPruned:      synctrust.ErrHistoryPruned,
		"something_new":                 synctrust.ErrRemoteProtocol,
	} {
		f := newFake(t)
		f.mux.HandleFunc("GET /v1/vaults", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusBadRequest, syncwire.ErrorBody{Error: code})
		})
		if _, err := helloed(t, f).Vaults(ctx); !errors.Is(err, want) {
			t.Errorf("code %s: err = %v, want %v", code, err, want)
		}
	}
}

func TestConflictAndRateLimit(t *testing.T) {
	f := newFake(t)
	f.mux.HandleFunc("POST /v1/vaults/{id}/commit", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusConflict, syncwire.ConflictReply{Error: syncwire.CodeConflict, CurrentSeq: 7})
	})
	f.mux.HandleFunc("GET /v1/vaults", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3")
		writeJSON(w, http.StatusTooManyRequests, syncwire.ErrorBody{Error: syncwire.CodeRateLimited})
	})
	c := helloed(t, f)
	_, err := c.Commit(ctx, vaultsync.Commit{Head: vaultsync.Head{Vault: vaultsync.ID{1}, Seq: 3}})
	var conflict *synctrust.ConflictError
	if !errors.As(err, &conflict) || conflict.CurrentSeq != 7 {
		t.Fatalf("commit: %v, want ConflictError at 7", err)
	}
	_, err = c.Vaults(ctx)
	var limited *synctrust.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter != 3*time.Second {
		t.Fatalf("vaults: %v, want RateLimitedError of 3s", err)
	}
}

func TestHostileRepliesAreProtocolErrors(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"not json":      func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("<html>")) },
		"unknown field": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"vaults":[],"x":1}`)) },
		"oversize": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"vaults":[{"vault":"` + strings.Repeat("a", 2<<20) + `"}]}`))
		},
		"error text": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("panic: table accounts"))
		},
		"bad vault id": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 200, syncwire.VaultList{Vaults: []syncwire.VaultSummary{{Vault: "zz"}}})
		},
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://example.com/", http.StatusFound)
		},
	} {
		f := newFake(t)
		f.mux.HandleFunc("GET /v1/vaults", handler)
		if _, err := helloed(t, f).Vaults(ctx); !errors.Is(err, synctrust.ErrRemoteProtocol) {
			t.Errorf("%s: err = %v, want ErrRemoteProtocol", name, err)
		}
	}
}

func TestBearerTokenOnlyAfterItIsSet(t *testing.T) {
	f := newFake(t)
	f.mux.HandleFunc("GET /v1/vaults", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, syncwire.VaultList{}) })
	c := helloed(t, f)
	if f.lastAuth.Load() != "" {
		t.Fatal("hello carried an Authorization header")
	}
	c.SetAccessToken("tok")
	if _, err := c.Vaults(ctx); err != nil {
		t.Fatal(err)
	}
	if f.lastAuth.Load() != "Bearer tok" {
		t.Fatalf("authorization = %q", f.lastAuth.Load())
	}
	c.SetAccessToken("")
	_, _ = c.Vaults(ctx)
	if f.lastAuth.Load() != "" {
		t.Fatal("a cleared token was still sent")
	}
}

func TestChangesDecodeAndConvert(t *testing.T) {
	f := newFake(t)
	env := vaultsync.Envelope{Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 1, Epoch: 1, Suite: vaultsync.Suite1,
		WrappedDEK: bytes.Repeat([]byte{5}, 72), Body: bytes.Repeat([]byte{6}, 296), Author: vaultsync.ID{7},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
	head := vaultsync.Head{Vault: vaultsync.ID{1}, Seq: 1, Commit: vaultsync.ID{3}, Author: vaultsync.ID{7},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
	var gotQuery string
	f.mux.HandleFunc("GET /v1/vaults/{id}/changes", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		writeJSON(w, 200, syncwire.Changes{More: true, Commits: []syncwire.CommitEntry{
			{Seq: 1, Head: syncwire.HeadFrom(&head), Records: []syncwire.Envelope{syncwire.EnvelopeFrom(&env)}}}})
	})
	commits, more, err := helloed(t, f).Changes(ctx, vaultsync.ID{1}, 0, 50)
	if err != nil || !more || len(commits) != 1 || commits[0].Seq != 1 || len(commits[0].Commit.Records) != 1 {
		t.Fatalf("changes = %+v, %v, %v", commits, more, err)
	}
	if gotQuery != "limit=50&since=0" {
		t.Fatalf("query = %q", gotQuery)
	}
}

func TestRetryAfterIsBounded(t *testing.T) {
	for header, want := range map[string]time.Duration{"0": time.Second, "-5": time.Second, "abc": time.Second, "": time.Second,
		"3600": time.Hour, "999999": time.Hour} {
		f := newFake(t)
		f.mux.HandleFunc("GET /v1/vaults", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", header)
			writeJSON(w, http.StatusTooManyRequests, syncwire.ErrorBody{Error: syncwire.CodeRateLimited})
		})
		_, err := helloed(t, f).Vaults(ctx)
		var limited *synctrust.RateLimitedError
		if !errors.As(err, &limited) || limited.RetryAfter != want {
			t.Errorf("Retry-After %q: %v, want %s; a hostile server must not park sync forever", header, err, want)
		}
	}
}

func TestConflictWithoutTheConflictCodeIsAProtocolError(t *testing.T) {
	f := newFake(t)
	f.mux.HandleFunc("POST /v1/vaults/{id}/commit", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusConflict, syncwire.ConflictReply{Error: syncwire.CodeInternal, CurrentSeq: 7})
	})
	_, err := helloed(t, f).Commit(ctx, vaultsync.Commit{Head: vaultsync.Head{Vault: vaultsync.ID{1}, Seq: 3}})
	if !errors.Is(err, synctrust.ErrRemoteProtocol) {
		t.Fatalf("err = %v, want ErrRemoteProtocol", err)
	}
}

func TestTLS12ServerIsRefused(t *testing.T) {
	f := newFake(t)
	f.srv.Close()
	cert := selfSigned(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MaxVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: f.mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	f.signed = cert.Leaf.RawSubjectPublicKeyInfo
	c, _ := syncclient.New(syncclient.Config{BaseURL: "https://" + ln.Addr().String()})
	defer c.Close()
	if _, err := c.Hello(ctx); err == nil {
		t.Fatal("a server limited to TLS 1.2 completed a hello")
	}
}

var upgrader = websocket.Upgrader{}

func TestEventsDeliverAndCloseOnGarbage(t *testing.T) {
	f := newFake(t)
	f.mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(syncwire.Event{Vault: syncwire.HexID(vaultsync.ID{1}), Seq: 4})
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"vault":"zz"}`))
		_ = conn.WriteJSON(syncwire.Event{Vault: syncwire.HexID(vaultsync.ID{2}), Seq: 5})
		_, _, _ = conn.ReadMessage()
	})
	c := helloed(t, f)
	c.SetAccessToken("tok")
	ch, err := c.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := <-ch
	if !ok || ev.Vault != (vaultsync.ID{1}) || ev.Seq != 4 {
		t.Fatalf("first event = %+v, %v", ev, ok)
	}
	if ev, ok := <-ch; ok {
		t.Fatalf("an event after a malformed frame was delivered: %+v; a garbled stream must be dropped", ev)
	}
}

func TestEventsNeedAVerifiedServer(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, f.pub())
	if _, err := c.Events(ctx); !errors.Is(err, synctrust.ErrServerIdentity) {
		t.Fatalf("err = %v, want ErrServerIdentity", err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("the events socket was opened to an unverified server")
	}
}

func TestEventsEndWithTheContext(t *testing.T) {
	f := newFake(t)
	f.mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	})
	c := helloed(t, f)
	cctx, cancel := context.WithCancel(ctx)
	ch, err := c.Events(cctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("an event appeared from nowhere")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the events channel stayed open after its context ended")
	}
}

func TestEventsRefusedByTheServer(t *testing.T) {
	f := newFake(t)
	f.mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusUnauthorized, syncwire.ErrorBody{Error: syncwire.CodeUnauthorized})
	})
	if _, err := helloed(t, f).Events(ctx); !errors.Is(err, synctrust.ErrRemoteUnauthorized) {
		t.Fatalf("err = %v, want ErrRemoteUnauthorized", err)
	}
}

func TestPasswordRoutesCarryTheTokenAndTheSignature(t *testing.T) {
	f := newFake(t)
	var got syncwire.PasswordFinish
	f.mux.HandleFunc("POST /v1/account/password/init", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, syncwire.PasswordInitReply{OPAQUE: []byte("resp")})
	})
	f.mux.HandleFunc("POST /v1/account/password/finish", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		writeJSON(w, 200, struct{}{})
	})
	c := helloed(t, f)
	c.SetAccessToken("tok")
	resp, err := c.PasswordInit(ctx, []byte("req"))
	if err != nil || string(resp) != "resp" || f.lastAuth.Load() != "Bearer tok" {
		t.Fatalf("init = %q, %v, auth %v", resp, err, f.lastAuth.Load())
	}
	if err := c.PasswordFinish(ctx, []byte("rec"), []byte("bun"), []byte("sig")); err != nil {
		t.Fatal(err)
	}
	if string(got.Record) != "rec" || string(got.Bundle) != "bun" || string(got.Sig) != "sig" {
		t.Fatalf("finish carried %+v", got)
	}
}

func TestVersionsDecodeAndConvert(t *testing.T) {
	f := newFake(t)
	env := vaultsync.Envelope{Vault: vaultsync.ID{1}, Record: vaultsync.ID{2}, Version: 3, Epoch: 1, Suite: vaultsync.Suite1,
		WrappedDEK: bytes.Repeat([]byte{5}, 72), Body: bytes.Repeat([]byte{6}, 296), Author: vaultsync.ID{7},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
	var gotPath string
	f.mux.HandleFunc("GET /v1/vaults/{id}/records/{rid}/versions", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeJSON(w, 200, syncwire.Versions{Records: []syncwire.Envelope{syncwire.EnvelopeFrom(&env)}})
	})
	got, err := helloed(t, f).Versions(ctx, vaultsync.ID{1}, vaultsync.ID{2})
	if err != nil || len(got) != 1 || got[0].Version != 3 {
		t.Fatalf("versions = %+v, %v", got, err)
	}
	if want := "/v1/vaults/" + syncwire.HexID(vaultsync.ID{1}) + "/records/" + syncwire.HexID(vaultsync.ID{2}) + "/versions"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestCommitSendsPurges(t *testing.T) {
	f := newFake(t)
	var got syncwire.Commit
	f.mux.HandleFunc("POST /v1/vaults/{id}/commit", func(w http.ResponseWriter, r *http.Request) {
		_ = syncwire.Decode(r.Body, 1<<20, &got)
		writeJSON(w, 200, syncwire.CommitReply{Seq: 2})
	})
	head := vaultsync.Head{Vault: vaultsync.ID{1}, Seq: 2, Commit: vaultsync.ID{3}, Author: vaultsync.ID{7},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
	if _, err := helloed(t, f).Commit(ctx, vaultsync.Commit{Head: head, Purged: []vaultsync.ID{{9}}}); err != nil {
		t.Fatal(err)
	}
	if len(got.Purged) != 1 || got.ExpectSeq != 1 {
		t.Fatalf("sent %+v; the purged ids must reach the server", got)
	}
}
