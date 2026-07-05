package pinger

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestTCPPingerReachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		_ = conn.Close()
	}()

	p := NewTCPPinger(time.Second)
	if err := p.Ping(context.Background(), "tcp", ln.Addr().String()); err != nil {
		t.Fatalf("Ping() = %v, want nil", err)
	}
}

func TestTCPPingerUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	p := NewTCPPinger(100 * time.Millisecond)
	if err := p.Ping(context.Background(), "tcp", addr); err == nil {
		t.Fatal("Ping() = nil, want error for closed port")
	}
}

func TestNewTCPPingerDefaultTimeout(t *testing.T) {
	p := NewTCPPinger(0)
	if p.timeout != defaultPingTimeout {
		t.Fatalf("timeout = %v, want %v", p.timeout, defaultPingTimeout)
	}
}
