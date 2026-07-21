package usecase

import (
	"context"
	"testing"

	"xquakshell/internal/domain"
)

func TestParseGetentName(t *testing.T) {
	if got := parseGetentName("alice:x:1000:1000::/home/alice:/bin/bash", "1000"); got != "alice" {
		t.Fatalf("got %q", got)
	}
	if got := parseGetentName("", "1000"); got != "1000" {
		t.Fatalf("fallback got %q", got)
	}
}

func TestIsNumericID(t *testing.T) {
	if !isNumericID("1000") {
		t.Fatal("expected numeric")
	}
	if isNumericID("alice") {
		t.Fatal("expected non-numeric")
	}
	if isNumericID("") {
		t.Fatal("empty is not numeric")
	}
}

type mockRemoteExec struct {
	execFn func(sessionID, cmd string) (string, error)
}

func (m *mockRemoteExec) Exec(sessionID, cmd string) (string, error) {
	return m.execFn(sessionID, cmd)
}
func (m *mockRemoteExec) GetRemoteFS(string) (domain.RemoteFS, error) { return nil, nil }
func (m *mockRemoteExec) GetSessionContext(string) (context.Context, error) {
	return context.Background(), nil
}

func TestRemoteFSServiceResolveOwnerCache(t *testing.T) {
	calls := 0
	svc := NewRemoteFSService(&mockRemoteExec{
		execFn: func(_, cmd string) (string, error) {
			calls++
			if cmd == "getent passwd 1000" {
				return "alice:x:1000:1000", nil
			}
			return "", nil
		},
	})
	if got := svc.resolveOwner("s1", "1000"); got != "alice" {
		t.Fatalf("first resolve: got %q", got)
	}
	if got := svc.resolveOwner("s1", "1000"); got != "alice" {
		t.Fatalf("cached resolve: got %q", got)
	}
	if calls != 1 {
		t.Fatalf("expected 1 exec call, got %d", calls)
	}
}

func TestRemoteFSServiceResolveOwnerNonNumeric(t *testing.T) {
	svc := NewRemoteFSService(&mockRemoteExec{})
	if got := svc.resolveOwner("s1", "root"); got != "root" {
		t.Fatalf("got %q", got)
	}
}

func TestRemoteFSServiceClearSessionCache(t *testing.T) {
	svc := NewRemoteFSService(&mockRemoteExec{
		execFn: func(_, _ string) (string, error) { return "alice:x:1000", nil },
	})
	_ = svc.resolveOwner("s1", "1000")
	svc.ClearSessionCache("s1")
	calls := 0
	svc.sessions = &mockRemoteExec{
		execFn: func(_, _ string) (string, error) {
			calls++
			return "alice:x:1000", nil
		},
	}
	_ = svc.resolveOwner("s1", "1000")
	if calls != 1 {
		t.Fatalf("expected cache miss after clear, calls=%d", calls)
	}
}
