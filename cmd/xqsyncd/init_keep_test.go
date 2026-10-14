package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A service manager runs init on every start (systemd ExecStartPre); with --keep-existing an
// initialised directory is left exactly as it was, and the fingerprint is still printed.
func TestInitKeepExistingLeavesAnInitialisedDirectoryAlone(t *testing.T) {
	dir := initDir(t)
	names := []string{keyFileName, certFileName, keyPEMName}
	before := map[string][]byte{}
	for _, n := range names {
		before[n], _ = os.ReadFile(filepath.Join(dir, n))
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"init", "--dir", dir, "--keep-existing"}, &out); err != nil {
		t.Fatalf("init --keep-existing on an initialised dir: %v", err)
	}
	for _, n := range names {
		after, _ := os.ReadFile(filepath.Join(dir, n))
		if !bytes.Equal(before[n], after) {
			t.Fatalf("%s changed; replacing it would lock every account out or change the identity clients pinned", n)
		}
	}
	if !strings.Contains(out.String(), "identity fingerprint:") {
		t.Fatalf("output %q; the fingerprint is printed so it can be read from the service log", out.String())
	}
	if err := run(context.Background(), []string{"init", "--dir", dir}, io.Discard); !errors.Is(err, errKeyFileExists) {
		t.Fatalf("plain init still refuses: %v", err)
	}
}

func TestInitKeepExistingInitialisesAnEmptyDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "srv")
	if err := run(context.Background(), []string{"init", "--dir", dir, "--keep-existing"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{keyFileName, certFileName, keyPEMName} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Fatalf("%s missing after a first init: %v", n, err)
		}
	}
}

// syncBuffer is a log sink the serve goroutine writes while the test reads.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// serve logs the bound address, so a harness can listen on port 0 and read the port it got.
func TestServeLogsTheBoundAddress(t *testing.T) {
	dir := initDir(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var logs syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	cfg := serveConfig{dir: dir, certPath: filepath.Join(dir, certFileName), keyPath: filepath.Join(dir, keyPEMName),
		log: slog.New(slog.NewTextHandler(&logs, nil))}
	go func() { done <- serve(ctx, cfg, ln) }()
	want := "addr=" + ln.Addr().String()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(logs.String(), want) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if !strings.Contains(logs.String(), "xqsyncd listening") || !strings.Contains(logs.String(), want) {
		t.Fatalf("log %q; want an 'xqsyncd listening' line with %s", logs.String(), want)
	}
}
