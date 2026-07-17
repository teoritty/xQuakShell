package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

func TestPluginAuthAttemptRegistry_AuthorizeRejectsForeignAuthMethodID(t *testing.T) {
	r := NewPluginAuthAttemptRegistry()
	a, err := r.Begin("plugin-a", "conn-1", "otp")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Authorize("plugin-a", a.ID, "other", ""); err != domainplugin.ErrSessionNotBound {
		t.Fatalf("Authorize() = %v, want ErrSessionNotBound", err)
	}
}

func TestPluginAuthBridge_AuthorizeFailureBlocksRPC(t *testing.T) {
	caller := &fakePluginCaller{}
	bridge := NewPluginAuthBridge(caller, NewPluginAuthAttemptRegistry())
	method := domain.PluginAuthMethod{PluginID: "p1", AuthMethodID: "otp", Kind: domain.AuthProviderKindKeyboardInteractive}
	_, err := bridge.Prepare(context.Background(), "missing-attempt", method)
	if !errors.Is(err, domainplugin.ErrSessionNotBound) {
		t.Fatalf("Prepare() = %v, want ErrSessionNotBound", err)
	}
	if len(caller.methods) != 0 {
		t.Fatal("expected no RPC when authorize fails")
	}
}

func TestPluginAuthBridge_PrepareSendsConnectionContext(t *testing.T) {
	r := NewPluginAuthAttemptRegistry()
	a, err := r.Begin("p1", "conn-99", "otp")
	if err != nil {
		t.Fatal(err)
	}
	caller := &recordingAuthCaller{}
	bridge := NewPluginAuthBridge(caller, r)
	method := domain.PluginAuthMethod{
		PluginID: "p1", AuthMethodID: "otp", Kind: domain.AuthProviderKindKeyboardInteractive,
		ConnectionID: "conn-99", Fields: map[string]string{"tenant": "acme"},
	}
	if _, err := bridge.Prepare(context.Background(), a.ID, method); err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	if err := json.Unmarshal(caller.lastParams, &params); err != nil {
		t.Fatal(err)
	}
	if params["connectionId"] != "conn-99" {
		t.Fatalf("connectionId = %v", params["connectionId"])
	}
	fields := params["fields"].(map[string]any)
	if fields["tenant"] != "acme" {
		t.Fatalf("fields = %v", fields)
	}
}

func TestPluginAuthBridge_AllMethodsUseDistinctTimeouts(t *testing.T) {
	r := NewPluginAuthAttemptRegistry()
	a, _ := r.Begin("p1", "conn-1", "otp")
	caller := &fakePluginCaller{}
	bridge := NewPluginAuthBridge(caller, r)
	method := domain.PluginAuthMethod{PluginID: "p1", AuthMethodID: "otp", Kind: domain.AuthProviderKindKeyboardInteractive}
	ctx := context.Background()

	if _, err := bridge.Prepare(ctx, a.ID, method); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := bridge.AnswerChallenge(ctx, a.ID, method, domain.PluginAuthChallenge{Name: "x"}); err != nil {
		t.Fatalf("challenge: %v", err)
	}
	if _, err := bridge.Sign(ctx, a.ID, method, domain.PluginAuthSignRequest{DataToSign: []byte{1}}); err != nil {
		t.Fatalf("sign: %v", err)
	}

	caller.mu.Lock()
	defer caller.mu.Unlock()
	if len(caller.timeouts) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(caller.timeouts))
	}
	if caller.timeouts[1] == caller.timeouts[2] {
		t.Fatal("challenge and sign timeouts must differ")
	}
	if caller.timeouts[1] != authChallengeTimeout {
		t.Fatalf("challenge timeout = %v, want %v", caller.timeouts[1], authChallengeTimeout)
	}
	if caller.timeouts[2] != authSignTimeout {
		t.Fatalf("sign timeout = %v, want %v", caller.timeouts[2], authSignTimeout)
	}
}

func TestPluginAuthBridge_AnswerChallengeMapsTimeout(t *testing.T) {
	r := NewPluginAuthAttemptRegistry()
	a, _ := r.Begin("p1", "conn-1", "otp")
	bridge := NewPluginAuthBridge(&timeoutAuthCaller{}, r)
	method := domain.PluginAuthMethod{PluginID: "p1", AuthMethodID: "otp", Kind: domain.AuthProviderKindKeyboardInteractive}
	_, err := bridge.AnswerChallenge(context.Background(), a.ID, method, domain.PluginAuthChallenge{Name: "x"})
	if !errors.Is(err, domainplugin.ErrAuthChallengeTimeout) {
		t.Fatalf("AnswerChallenge() = %v, want ErrAuthChallengeTimeout", err)
	}
}

func TestPluginManager_CallWithTimeout_AuthChallengeDeadline(t *testing.T) {
	reg := NewPluginRegistry()
	if err := reg.Register(domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: "p1", Name: "p1", Version: "1", Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "x"}},
	}); err != nil {
		t.Fatal(err)
	}
	host := &timeoutAuthHost{running: []domainplugin.ProcessInstance{{PluginID: "p1", State: domainplugin.ProcessRunning}}}
	m := NewPluginManagerWithConfig(PluginManagerConfig{
		Registry:    reg,
		Host:        host,
		InstallRoot: t.TempDir(),
	})
	_, err := m.CallWithTimeout(context.Background(), "p1", "auth.answerChallenge", json.RawMessage(`{}`), time.Millisecond)
	if !errors.Is(err, domainplugin.ErrAuthChallengeTimeout) {
		t.Fatalf("CallWithTimeout() = %v, want ErrAuthChallengeTimeout", err)
	}
}

func TestPluginManager_OutboundAuthAuditSanitizesParams(t *testing.T) {
	reg := NewPluginRegistry()
	if err := reg.Register(domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: "p1", Name: "p1", Version: "1", Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "x"}},
	}); err != nil {
		t.Fatal(err)
	}
	host := &okAuthHost{
		running: []domainplugin.ProcessInstance{{PluginID: "p1", State: domainplugin.ProcessRunning}},
		result:  json.RawMessage(`{"signatureBase64":"c2VjcmV0cGFzc3dvcmQ="}`),
	}
	m := NewPluginManagerWithConfig(PluginManagerConfig{
		Registry:    reg,
		Host:        host,
		InstallRoot: t.TempDir(),
	})
	var audited []string
	m.SetOutboundAuthAudit(func(pluginID, method, sanitized string) {
		audited = append(audited, pluginID+":"+method+":"+sanitized)
	})
	params, _ := json.Marshal(map[string]string{"dataBase64": "c2VjcmV0cGFzc3dvcmQ="})
	if _, err := m.CallWithTimeout(context.Background(), "p1", "auth.sign", params, time.Second); err != nil {
		t.Fatal(err)
	}
	if len(audited) != 2 {
		t.Fatalf("expected params+result audit, got %d lines", len(audited))
	}
	if strings.Contains(audited[0], "c2VjcmV0") {
		t.Fatalf("audit must not contain raw secret: %s", audited[0])
	}
	if !strings.Contains(audited[0], "redacted") {
		t.Fatalf("expected redacted params audit: %s", audited[0])
	}
	if strings.Contains(audited[1], "c2VjcmV0") {
		t.Fatalf("audit must not contain raw result secret: %s", audited[1])
	}
}

type recordingAuthCaller struct {
	lastParams json.RawMessage
}

type fakePluginCaller struct {
	mu       sync.Mutex
	methods  []string
	timeouts []time.Duration
}

func (f *fakePluginCaller) CallWithTimeout(_ context.Context, _, method string, _ json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	f.mu.Lock()
	f.methods = append(f.methods, method)
	f.timeouts = append(f.timeouts, timeout)
	f.mu.Unlock()
	switch method {
	case "auth.prepare":
		return json.Marshal(map[string]string{"publicKeyBlobBase64": "AQID"})
	case "auth.answerChallenge":
		return json.Marshal(map[string][]string{"answers": {"ok"}})
	case "auth.sign":
		return json.Marshal(map[string]string{"signatureBase64": "AQID", "signatureFormat": "ssh-ed25519"})
	default:
		return json.RawMessage(`{}`), nil
	}
}

func (r *recordingAuthCaller) CallWithTimeout(_ context.Context, _, _ string, params json.RawMessage, _ time.Duration) (json.RawMessage, error) {
	r.lastParams = append(json.RawMessage(nil), params...)
	return json.Marshal(map[string]string{"publicKeyBlobBase64": "AQID"})
}

type timeoutAuthCaller struct{}

func (timeoutAuthCaller) CallWithTimeout(_ context.Context, _, _ string, _ json.RawMessage, _ time.Duration) (json.RawMessage, error) {
	return nil, domainplugin.ErrAuthChallengeTimeout
}

type timeoutAuthHost struct {
	running []domainplugin.ProcessInstance
}

func (timeoutAuthHost) Start(context.Context, domainplugin.InstalledPlugin, string) error { return nil }
func (timeoutAuthHost) Stop(context.Context, string, string) error                        { return nil }
func (timeoutAuthHost) StopAll(context.Context)                                           {}
func (timeoutAuthHost) Call(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}
func (timeoutAuthHost) CallWithTimeout(ctx context.Context, _, _, _ string, _ json.RawMessage, timeout time.Duration) (json.RawMessage, error) {
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	<-callCtx.Done()
	return nil, callCtx.Err()
}
func (timeoutAuthHost) Notify(context.Context, string, string, string, json.RawMessage) error {
	return nil
}
func (timeoutAuthHost) State(string, string) domainplugin.ProcessState {
	return domainplugin.ProcessRunning
}
func (h *timeoutAuthHost) RunningInstances() []domainplugin.ProcessInstance { return h.running }
func (timeoutAuthHost) BindSession(string, string) error                    { return nil }
func (timeoutAuthHost) UnbindSession(string, string)                        {}

type okAuthHost struct {
	mu      sync.Mutex
	running []domainplugin.ProcessInstance
	result  json.RawMessage
}

func (*okAuthHost) Start(context.Context, domainplugin.InstalledPlugin, string) error { return nil }
func (*okAuthHost) Stop(context.Context, string, string) error                        { return nil }
func (*okAuthHost) StopAll(context.Context)                                           {}
func (*okAuthHost) Call(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}
func (h *okAuthHost) CallWithTimeout(_ context.Context, _, _, _ string, _ json.RawMessage, _ time.Duration) (json.RawMessage, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append(json.RawMessage(nil), h.result...), nil
}
func (*okAuthHost) Notify(context.Context, string, string, string, json.RawMessage) error {
	return nil
}
func (*okAuthHost) State(string, string) domainplugin.ProcessState {
	return domainplugin.ProcessRunning
}
func (h *okAuthHost) RunningInstances() []domainplugin.ProcessInstance { return h.running }
func (*okAuthHost) BindSession(string, string) error                   { return nil }
func (*okAuthHost) UnbindSession(string, string)                       {}
