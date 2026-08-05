package usecase

import (
	"errors"
	"sync"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func testSurface(id, pluginID, sessionID string) domainplugin.Surface {
	return domainplugin.Surface{
		ID:              id,
		PluginID:        pluginID,
		ParentSessionID: sessionID,
		ConnectionID:    "conn-1",
		Kind:            domainplugin.SurfaceKindLog,
		State:           domainplugin.SurfaceStateConnecting,
	}
}

// A surface id belonging to another plugin must be indistinguishable from one that never existed.
// Telling the two apart is an oracle: it turns a guessed id into a way to learn that some other
// plugin owns a tab, which is exactly the leak the ownership check exists to prevent.
func TestSurfaceRegistryHidesForeignSurfacesAsDenied(t *testing.T) {
	r := NewSurfaceRegistry()
	if err := r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8); err != nil {
		t.Fatalf("Add: %v", err)
	}
	_, errForeign := r.Get("srf-1", "plugin-b")
	_, errUnknown := r.Get("srf-nope", "plugin-b")
	if !errors.Is(errForeign, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("foreign surface: got %v, want ErrCapabilityDenied", errForeign)
	}
	if !errors.Is(errUnknown, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("unknown surface: got %v, want ErrCapabilityDenied", errUnknown)
	}
	if errForeign.Error() != errUnknown.Error() {
		t.Fatalf("a foreign surface id must be indistinguishable from an unknown one: %q vs %q",
			errForeign.Error(), errUnknown.Error())
	}
}

func TestSurfaceRegistryOwnerCanReadItsOwnSurface(t *testing.T) {
	r := NewSurfaceRegistry()
	_ = r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8)
	got, err := r.Get("srf-1", "plugin-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "srf-1" || got.ParentSessionID != "sess-1" {
		t.Fatalf("Get returned %+v", got)
	}
}

func TestSurfaceRegistryEnforcesPerPluginCap(t *testing.T) {
	r := NewSurfaceRegistry()
	if err := r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 1); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := r.Add(testSurface("srf-2", "plugin-a", "sess-1"), 1); !errors.Is(err, domainplugin.ErrRateLimited) {
		t.Fatalf("second Add past the cap: got %v, want ErrRateLimited", err)
	}
	if err := r.Add(testSurface("srf-3", "plugin-b", "sess-1"), 1); err != nil {
		t.Fatalf("the cap is per plugin, not global: %v", err)
	}
}

func TestSurfaceRegistryRejectsDuplicateID(t *testing.T) {
	r := NewSurfaceRegistry()
	_ = r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8)
	if err := r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8); err == nil {
		t.Fatal("expected a duplicate surface id to be rejected")
	}
}

func TestSurfaceRegistryRemoveBySessionIsIdempotent(t *testing.T) {
	r := NewSurfaceRegistry()
	_ = r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8)
	_ = r.Add(testSurface("srf-2", "plugin-a", "sess-2"), 8)
	got := r.RemoveBySession("sess-1")
	if len(got) != 1 || got[0].ID != "srf-1" {
		t.Fatalf("RemoveBySession returned %v", got)
	}
	if got := r.RemoveBySession("sess-1"); len(got) != 0 {
		t.Fatalf("second RemoveBySession must be a no-op, got %v", got)
	}
	if _, err := r.Get("srf-2", "plugin-a"); err != nil {
		t.Fatalf("a sibling session's surface must survive: %v", err)
	}
}

func TestSurfaceRegistryRemoveByPluginLeavesOtherPlugins(t *testing.T) {
	r := NewSurfaceRegistry()
	_ = r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8)
	_ = r.Add(testSurface("srf-2", "plugin-b", "sess-1"), 8)
	if got := r.RemoveByPlugin("plugin-a"); len(got) != 1 || got[0].ID != "srf-1" {
		t.Fatalf("RemoveByPlugin returned %v", got)
	}
	if _, err := r.Get("srf-2", "plugin-b"); err != nil {
		t.Fatalf("another plugin's surface must survive: %v", err)
	}
}

func TestSurfaceRegistryRemoveReportsWhetherItExisted(t *testing.T) {
	r := NewSurfaceRegistry()
	_ = r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8)
	if _, existed := r.Remove("srf-1"); !existed {
		t.Fatal("first Remove must report the surface existed")
	}
	if _, existed := r.Remove("srf-1"); existed {
		t.Fatal("second Remove must report the surface was already gone")
	}
}

func TestSurfaceRegistryUpdateMutatesUnderTheLock(t *testing.T) {
	r := NewSurfaceRegistry()
	_ = r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8)
	got, err := r.Update("srf-1", "plugin-a", func(s *domainplugin.Surface) {
		s.Title = "renamed"
		s.State = domainplugin.SurfaceStateReady
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Title != "renamed" || got.State != domainplugin.SurfaceStateReady {
		t.Fatalf("Update returned %+v", got)
	}
	stored, _ := r.Get("srf-1", "plugin-a")
	if stored.Title != "renamed" {
		t.Fatalf("mutation did not stick: %+v", stored)
	}
}

func TestSurfaceRegistryUpdateDeniesForeignPlugin(t *testing.T) {
	r := NewSurfaceRegistry()
	_ = r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8)
	_, err := r.Update("srf-1", "plugin-b", func(s *domainplugin.Surface) { s.Title = "hijacked" })
	if !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("got %v, want ErrCapabilityDenied", err)
	}
	stored, _ := r.Get("srf-1", "plugin-a")
	if stored.Title == "hijacked" {
		t.Fatal("a denied Update must not have mutated anything")
	}
}

// Get returns a copy: a caller that mutates what it got must not be mutating registry state, or
// the mutex protecting that state would be decorative.
func TestSurfaceRegistryGetReturnsACopy(t *testing.T) {
	r := NewSurfaceRegistry()
	_ = r.Add(testSurface("srf-1", "plugin-a", "sess-1"), 8)
	got, _ := r.Get("srf-1", "plugin-a")
	got.Title = "local edit"
	stored, _ := r.Get("srf-1", "plugin-a")
	if stored.Title != "" {
		t.Fatalf("registry state leaked through Get: %+v", stored)
	}
}

func TestSurfaceRegistryConcurrentAccessIsRaceFree(t *testing.T) {
	r := NewSurfaceRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "srf-" + string(rune('a'+i%26))
			_ = r.Add(testSurface(id, "plugin-a", "sess-1"), 64)
			_, _ = r.Get(id, "plugin-a")
			_, _ = r.Update(id, "plugin-a", func(s *domainplugin.Surface) { s.Title = "t" })
			r.Remove(id)
		}(i)
	}
	wg.Wait()
}
