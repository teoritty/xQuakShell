package usecase_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func dataPlugin(id string) domainplugin.InstalledPlugin {
	return domainplugin.InstalledPlugin{Manifest: domainplugin.Manifest{ID: id, Name: id, Version: "1.0.0",
		Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
		Sync:   &domainplugin.SyncSpec{Paths: []string{"state.json"}}}}
}

func TestEnsureRunningHoldsTheStartGuardAcrossTheStart(t *testing.T) {
	host := &stubProcessHost{}
	manager, _ := newHookedManager(t, host)
	if err := manager.Registry().Load([]domainplugin.InstalledPlugin{dataPlugin("com.example.p")}); err != nil {
		t.Fatal(err)
	}
	var events []string
	manager.SetStartGuard(func(_ context.Context, id string) func() {
		events = append(events, fmt.Sprintf("guard %s, %d starts", id, host.startCount()))
		return func() { events = append(events, fmt.Sprintf("release, %d starts", host.startCount())) }
	})
	if err := manager.EnsureRunning(context.Background(), "com.example.p"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"guard com.example.p, 0 starts", "release, 1 starts"}; !slices.Equal(events, want) {
		t.Fatalf("events = %v, want %v: the guard is taken before the start and released after it", events, want)
	}
}

func TestProcessIdleOnlyWhenNoProcessCanTouchTheData(t *testing.T) {
	for state, idle := range map[domainplugin.ProcessState]bool{
		domainplugin.ProcessRunning: false, domainplugin.ProcessStarting: false, domainplugin.ProcessStopping: false,
		domainplugin.ProcessCrashed: false, domainplugin.ProcessStopped: true, domainplugin.ProcessSuspended: true,
	} {
		host := &stubProcessHost{instances: []domainplugin.ProcessInstance{{PluginID: "com.example.p", State: state}}}
		manager, _ := newHookedManager(t, host)
		if got := manager.ProcessIdle("com.example.p"); got != idle {
			t.Errorf("ProcessIdle with a %s process = %v, want %v", state, got, idle)
		}
	}
}

func TestSyncedManifestsListsInstalledPlugins(t *testing.T) {
	manager, _ := newHookedManager(t, &stubProcessHost{})
	if err := manager.Registry().Load([]domainplugin.InstalledPlugin{dataPlugin("com.example.p")}); err != nil {
		t.Fatal(err)
	}
	if got := manager.SyncedManifests(); len(got) != 1 || got[0].ID != "com.example.p" || got[0].Sync == nil {
		t.Fatalf("manifests = %+v", got)
	}
}

func TestProcessIdleHandlerFiresOnStopAndSuspendOnly(t *testing.T) {
	host := &stubProcessHost{instances: []domainplugin.ProcessInstance{{PluginID: "p1", SessionID: "s1", State: domainplugin.ProcessRunning}}}
	manager, _ := newHookedManager(t, host)
	var idle []string
	manager.SetProcessIdleHandler(func(id string) { idle = append(idle, id) })
	manager.SetPluginRetentionChecker(func(string) bool { return false })
	manager.SuspendIdlePlugins(context.Background(), 0)
	manager.OnProcessCrashed("p1", "s1")
	if len(idle) != 1 || idle[0] != "p1" {
		t.Fatalf("idle = %v; a suspension is idle, a crash is not", idle)
	}
}

func TestRestartPluginStopsThenStartsThroughTheGuard(t *testing.T) {
	host := &stubProcessHost{}
	manager, _ := newHookedManager(t, host)
	if err := manager.Registry().Load([]domainplugin.InstalledPlugin{dataPlugin("com.example.p")}); err != nil {
		t.Fatal(err)
	}
	guarded := 0
	manager.SetStartGuard(func(context.Context, string) func() { guarded++; return func() {} })
	if err := manager.RestartPlugin(context.Background(), "com.example.p"); err != nil {
		t.Fatal(err)
	}
	if host.startCount() != 1 || guarded != 1 {
		t.Fatalf("starts %d, guarded %d; a restart applies waiting data through the start guard, then starts once", host.startCount(), guarded)
	}
	if err := manager.RestartPlugin(context.Background(), "com.example.missing"); err == nil {
		t.Fatal("restarting a plugin that is not installed succeeded")
	}
}
