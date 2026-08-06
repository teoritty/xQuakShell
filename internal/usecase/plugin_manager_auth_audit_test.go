package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func auditedAuthManager(t *testing.T, result json.RawMessage) (*PluginManager, *[]string) {
	t.Helper()
	reg := NewPluginRegistry()
	if err := reg.Register(domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{ID: "p1", Name: "p1", Version: "1", Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "x"}},
	}); err != nil {
		t.Fatal(err)
	}
	host := &okAuthHost{
		running: []domainplugin.ProcessInstance{{PluginID: "p1", State: domainplugin.ProcessRunning}},
		result:  result,
	}
	m := NewPluginManagerWithConfig(PluginManagerConfig{Registry: reg, Host: host, InstallRoot: t.TempDir()})
	audited := &[]string{}
	m.SetOutboundAuthAudit(func(pluginID, method, sanitized string) {
		*audited = append(*audited, pluginID+":"+method+":"+sanitized)
	})
	return m, audited
}

// Call is the second door to the same process host. The audit rule used to live
// inside CallWithTimeout alone, so an auth.* method sent through Call was never
// recorded. Nothing routes auth that way today; this test is what keeps it that
// way once the hook is no longer written out at each call site.
func TestPluginManager_CallAuditsAuthParamsAndResult(t *testing.T) {
	m, audited := auditedAuthManager(t, json.RawMessage(`{"signatureBase64":"c2VjcmV0"}`))
	params, _ := json.Marshal(map[string]string{"dataBase64": "c2VjcmV0"})

	if _, err := m.Call(context.Background(), "p1", "auth.sign", params); err != nil {
		t.Fatal(err)
	}

	if len(*audited) != 2 {
		t.Fatalf("expected params+result audit, got %d lines: %v", len(*audited), *audited)
	}
	for _, line := range *audited {
		if strings.Contains(line, "c2VjcmV0") {
			t.Fatalf("audit line carries the raw secret: %s", line)
		}
		if !strings.Contains(line, "redacted") {
			t.Fatalf("audit line is not redacted: %s", line)
		}
	}
	if !strings.HasSuffix((*audited)[1][:strings.Index((*audited)[1], ":{")], "#result") {
		t.Fatalf("second line must be the result audit: %s", (*audited)[1])
	}
}

func TestPluginManager_CallLeavesNonAuthMethodsUnaudited(t *testing.T) {
	m, audited := auditedAuthManager(t, json.RawMessage(`{"ok":true}`))

	if _, err := m.Call(context.Background(), "p1", "command.execute", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}

	if len(*audited) != 0 {
		t.Fatalf("non-auth method audited: %v", *audited)
	}
}
