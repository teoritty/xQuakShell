package usecase

import (
	"errors"
	"net"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

func listenOn(t *testing.T, addr string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("cannot listen on %s in this environment: %v", addr, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func TestVerifyLoopbackListenerAcceptsLoopback(t *testing.T) {
	ln := listenOn(t, "127.0.0.1:0")

	if err := verifyLoopbackListener(ln, "rule-1"); err != nil {
		t.Fatalf("verifyLoopbackListener on 127.0.0.1 = %v, want nil", err)
	}
}

// This is the case domain.IsLoopbackBind cannot see. It validates the string, and the string
// "localhost" means whatever the resolver says: a hosts-file line pointing it at an external
// interface turns a forward the validator called loopback-only into one on the local network.
// 0.0.0.0 stands in for that here because it is the same outcome - a listener reachable from off
// the machine - and it does not require editing the test host's resolver to reproduce.
func TestVerifyLoopbackListenerRejectsAWildcardBinding(t *testing.T) {
	ln := listenOn(t, "0.0.0.0:0")

	err := verifyLoopbackListener(ln, "rule-1")

	if !errors.Is(err, domain.ErrForwardBindNotLoopback) {
		t.Fatalf("verifyLoopbackListener on 0.0.0.0 = %v, want ErrForwardBindNotLoopback", err)
	}
}

// Refusing while leaving the socket open would be worse than not checking: the port stays
// reachable from the network and nothing owns it any more.
func TestVerifyLoopbackListenerClosesTheSocketItRejects(t *testing.T) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("cannot listen on 0.0.0.0: %v", err)
	}
	addr := ln.Addr().String()

	if err := verifyLoopbackListener(ln, "rule-1"); err == nil {
		_ = ln.Close()
		t.Fatal("expected a refusal")
	}

	conn, dialErr := net.Dial("tcp", addr)
	if dialErr == nil {
		_ = conn.Close()
		t.Fatal("the rejected listener is still accepting connections")
	}
}

func TestVerifyLoopbackListenerNamesTheRule(t *testing.T) {
	ln := listenOn(t, "0.0.0.0:0")

	err := verifyLoopbackListener(ln, "forward-42")

	if err == nil || !strings.Contains(err.Error(), "forward-42") {
		t.Fatalf("error %v does not name the rule it refers to", err)
	}
}
