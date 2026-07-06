package usecase

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestPluginAuthAttemptRegistry_ConcurrentBeginAuthorizeEnd(t *testing.T) {
	r := NewPluginAuthAttemptRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := r.Begin("plugin-a", "conn-1", "otp")
			if err != nil {
				t.Errorf("begin: %v", err)
				return
			}
			if err := r.Authorize("plugin-a", a.ID, "otp"); err != nil {
				t.Errorf("authorize own attempt: %v", err)
			}
			if err := r.Authorize("plugin-b", a.ID, "otp"); err == nil {
				t.Fatal("expected foreign plugin authorize to fail")
			}
			r.End(a.ID)
		}()
	}
	wg.Wait()
}

func TestPluginAuthAttemptRegistry_MaxConcurrent(t *testing.T) {
	r := NewPluginAuthAttemptRegistry()
	ids := make([]string, 0, MaxConcurrentAuthAttempts)
	for i := 0; i < MaxConcurrentAuthAttempts; i++ {
		a, err := r.Begin("plugin-a", "conn-1", "otp")
		if err != nil {
			t.Fatalf("begin %d: %v", i, err)
		}
		ids = append(ids, a.ID)
	}
	if _, err := r.Begin("plugin-a", "conn-2", "otp"); err != domainplugin.ErrAuthProviderBusy {
		t.Fatalf("expected busy, got %v", err)
	}
	for _, id := range ids {
		r.End(id)
	}
}

type fakePluginCaller struct {
	mu       sync.Mutex
	timeouts []time.Duration
	methods  []string
}

func (f *fakePluginCaller) CallWithTimeout(_ context.Context, pluginID, method string, _ json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	f.mu.Lock()
	f.timeouts = append(f.timeouts, timeout)
	f.methods = append(f.methods, method)
	f.mu.Unlock()
	switch method {
	case "auth.prepare":
		return json.Marshal(map[string]string{"publicKeyBlobBase64": "AQID"})
	case "auth.answerChallenge":
		return json.Marshal(map[string][]string{"answers": {"123456"}})
	case "auth.sign":
		return json.Marshal(map[string]string{"signatureBase64": "AQID", "signatureFormat": "ssh-ed25519"})
	default:
		return nil, domainplugin.ErrNotImplemented
	}
}
