//go:build e2e_sync

// Package sync runs sync scenarios out of process (spec 12.7): the real xqsyncd binary - started
// here, or installed as a service and named by E2E_SYNC_URL and E2E_SYNC_DIR - and client stacks
// composed as the application composes them, with real OPAQUE at the suite's full cost.
//
// Clients reach the server through a TLS proxy that presents the server's own certificate, so the
// identity pinning they do is the pinning a user's client does. The proxy counts the records each
// pull fetches (S1) and can lose the reply to a stored commit (S11), which is what a network that
// fails between store and reply looks like from both ends; the binary carries no fault hook.
//
// Run with `make e2e-sync`. Not part of `go test ./...`.
package synce2e

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xquakshell/internal/domain/syncwire"
)

var ctx = context.Background()

// xqsyncdBin is the binary under test: XQSYNCD_BIN, or one built for this run.
var xqsyncdBin string

func TestMain(m *testing.M) {
	xqsyncdBin = os.Getenv("XQSYNCD_BIN")
	cleanup := func() {}
	if xqsyncdBin == "" {
		dir, err := os.MkdirTemp("", "xqsyncd-e2e-bin")
		if err != nil {
			panic(err)
		}
		cleanup = func() { _ = os.RemoveAll(dir) }
		xqsyncdBin = filepath.Join(dir, "xqsyncd")
		if runtime.GOOS == "windows" {
			xqsyncdBin += ".exe"
		}
		out, err := exec.Command("go", "build", "-o", xqsyncdBin, "xquakshell/cmd/xqsyncd").CombinedOutput()
		if err != nil {
			panic("build xqsyncd: " + err.Error() + "\n" + string(out))
		}
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// xqsyncd runs one command of the binary under test and returns its output.
func xqsyncd(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command(xqsyncdBin, args...).CombinedOutput() // #nosec G204 -- the binary under test
	if err != nil {
		t.Fatalf("xqsyncd %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// server is a running xqsyncd, reached through the counting proxy.
type server struct {
	t           *testing.T
	dir         string
	upstream    string
	url         string
	fingerprint string
	mu          sync.Mutex
	records     int
	// dropNextCommitReply lets the next commit reach the server and loses its reply.
	dropNextCommitReply atomic.Bool
}

func newServer(t *testing.T) *server {
	t.Helper()
	s := &server{t: t, dir: os.Getenv("E2E_SYNC_DIR"), upstream: os.Getenv("E2E_SYNC_URL")}
	if s.upstream == "" {
		s.dir = filepath.Join(t.TempDir(), "srv")
		xqsyncd(t, "init", "--dir", s.dir, "--host", "127.0.0.1")
		s.upstream = s.start()
	} else if s.dir == "" {
		t.Fatal("E2E_SYNC_URL needs E2E_SYNC_DIR: invites and the fingerprint come from the data directory")
	}
	out := strings.Fields(xqsyncd(t, "fingerprint", "--dir", s.dir))
	s.fingerprint = strings.Join(out, " ")
	s.url = s.proxy()
	return s
}

// start runs serve on a free port and returns its URL, read from the log line serve prints.
func (s *server) start() string {
	s.t.Helper()
	cmd := exec.Command(xqsyncdBin, "serve", "--dir", s.dir, "--addr", "127.0.0.1:0") // #nosec G204 -- the binary under test
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			line := sc.Text()
			if i := strings.Index(line, "addr="); i >= 0 && strings.Contains(line, "xqsyncd listening") {
				addr <- strings.Fields(line[i+len("addr="):])[0]
			}
		}
		_, _ = io.Copy(io.Discard, stderr)
	}()
	select {
	case a := <-addr:
		return "https://" + a
	case <-time.After(30 * time.Second):
		s.t.Fatal("xqsyncd did not report its address")
		return ""
	}
}

// proxy terminates TLS with the server's own certificate and forwards to the server.
func (s *server) proxy() string {
	s.t.Helper()
	cert, err := tls.LoadX509KeyPair(filepath.Join(s.dir, "tls.crt"), filepath.Join(s.dir, "tls.key"))
	if err != nil {
		s.t.Fatalf("the proxy needs the server's certificate: %v", err)
	}
	upstream := &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{
		// The proxy talks to the server it just read the certificate of; clients do the pinning.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}}} // #nosec G402 -- test proxy
	hs := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.forward(upstream, w, r)
	}))
	hs.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}
	hs.StartTLS()
	s.t.Cleanup(hs.Close)
	return hs.URL
}

func (s *server) forward(upstream *http.Client, w http.ResponseWriter, r *http.Request) {
	if websocketUpgrade(r) {
		// Notifications are not under test here; the scheduler falls back to polling without them.
		http.Error(w, "no events through the test proxy", http.StatusNotImplemented)
		return
	}
	body, _ := io.ReadAll(r.Body)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, s.upstream+r.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	req.Header = r.Header.Clone()
	resp, err := upstream.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	reply, _ := io.ReadAll(resp.Body)
	if strings.HasSuffix(r.URL.Path, "/commit") && resp.StatusCode == http.StatusOK && s.dropNextCommitReply.CompareAndSwap(true, false) {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/changes") && resp.StatusCode == http.StatusOK {
		var c syncwire.Changes
		if json.Unmarshal(reply, &c) == nil {
			s.mu.Lock()
			for _, e := range c.Commits {
				s.records += len(e.Records)
			}
			s.mu.Unlock()
		}
	}
	for k, v := range resp.Header {
		if k != "Content-Length" {
			w.Header()[k] = v
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(reply)
}

func websocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func (s *server) servedRecords() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.records
}

// invite mints a registration invite with the admin command, as an operator does.
func (s *server) invite() string {
	s.t.Helper()
	return strings.TrimSpace(xqsyncd(s.t, "admin", "invite", "create", "--dir", s.dir, "--ttl", "1h"))
}
