package usecase

import (
	"fmt"
	"sync"

	domainplugin "xquakshell/internal/domain/plugin"
)

// SurfaceRegistry holds every open plugin surface and is the sole owner of the mutex protecting
// them (ADR-015 §Use case).
//
// This file makes exactly one promise, and it is checkable by reading only this file: no outbound
// call of any kind happens here — no RPC to a plugin, no event to the frontend, no audit write. A
// lock held across any of those is how a registry like this deadlocks against the thing it is
// telling, and the only durable way to prevent it is to keep the callers and the lock in different
// files. Every method takes the lock, works on maps, and returns copies.
type SurfaceRegistry struct {
	mu        sync.Mutex
	byID      map[string]domainplugin.Surface
	byPlugin  map[string]map[string]struct{}
	bySession map[string]map[string]struct{}
}

// NewSurfaceRegistry creates an empty registry.
func NewSurfaceRegistry() *SurfaceRegistry {
	return &SurfaceRegistry{
		byID:      make(map[string]domainplugin.Surface),
		byPlugin:  make(map[string]map[string]struct{}),
		bySession: make(map[string]map[string]struct{}),
	}
}

// Add registers a surface, refusing to take the plugin past max concurrently open surfaces.
//
// The cap is passed in rather than read from a manifest here: the registry has no opinion about
// where a plugin's budget comes from, and taking the manifest as a dependency would drag capability
// resolution into the one file that must stay free of everything but map operations.
func (r *SurfaceRegistry) Add(s domainplugin.Surface, max int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[s.ID]; exists {
		return fmt.Errorf("surface %q already exists", s.ID)
	}
	if max > 0 && len(r.byPlugin[s.PluginID]) >= max {
		return fmt.Errorf("%w: plugin has %d open surfaces", domainplugin.ErrRateLimited, len(r.byPlugin[s.PluginID]))
	}

	r.byID[s.ID] = s
	addToIndex(r.byPlugin, s.PluginID, s.ID)
	addToIndex(r.bySession, s.ParentSessionID, s.ID)
	return nil
}

// Get returns a copy of the surface, provided pluginID owns it.
//
// A surface owned by another plugin and a surface that does not exist return the SAME error. The
// distinction would be an oracle: a plugin could probe ids and learn which ones belong to someone
// else, which is precisely what the ownership check is for.
func (r *SurfaceRegistry) Get(surfaceID, pluginID string) (domainplugin.Surface, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lookupLocked(surfaceID, pluginID)
}

// Update applies mutate to the stored surface under the lock and returns the updated copy.
//
// The mutation runs inside the critical section on purpose: a read-modify-write done by the caller
// would race two concurrent title changes into a lost update. mutate must not call back into the
// registry, and it is only ever a few field assignments — see surface_io.go, its sole caller.
func (r *SurfaceRegistry) Update(surfaceID, pluginID string, mutate func(*domainplugin.Surface)) (domainplugin.Surface, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, err := r.lookupLocked(surfaceID, pluginID)
	if err != nil {
		return domainplugin.Surface{}, err
	}
	mutate(&current)
	// Identity fields are not the caller's to change: a mutate that moved a surface to another
	// plugin or session would leave the indexes describing a placement that no longer exists.
	current.ID = surfaceID
	current.PluginID = r.byID[surfaceID].PluginID
	current.ParentSessionID = r.byID[surfaceID].ParentSessionID
	r.byID[surfaceID] = current
	return current, nil
}

// Remove drops a surface by id and reports whether it was there. The bool is what makes close
// idempotent at every layer above: a second close finds nothing and does nothing, rather than
// emitting a second round of notifications.
func (r *SurfaceRegistry) Remove(surfaceID string) (domainplugin.Surface, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.byID[surfaceID]
	if !ok {
		return domainplugin.Surface{}, false
	}
	r.deleteLocked(s)
	return s, true
}

// RemoveByPlugin drops every surface owned by a plugin and returns them, for the caller to
// announce. Returns an empty slice when there are none, so a caller can range over it unguarded.
func (r *SurfaceRegistry) RemoveByPlugin(pluginID string) []domainplugin.Surface {
	return r.removeIndexed(r.byPlugin, pluginID)
}

// RemoveBySession drops every surface bound to a parent session and returns them.
func (r *SurfaceRegistry) RemoveBySession(sessionID string) []domainplugin.Surface {
	return r.removeIndexed(r.bySession, sessionID)
}

// Exists reports whether an id is present, with no ownership check.
//
// It answers the one question Get deliberately will not, and it is safe only because of where it
// is used: after Get has already refused, to turn a denial into a silent no-op for a surface that
// has simply gone away (see surface_io.go). It must never be used to decide whether to serve a
// request — that decision is Get's, ownership included.
func (r *SurfaceRegistry) Exists(surfaceID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.byID[surfaceID]
	return ok
}

// Lookup returns a surface by id without an ownership check, for host-originated delivery: user
// input and resizes arrive from the UI, which addresses a tab, and the owner is what the registry
// is being asked to supply rather than to verify.
func (r *SurfaceRegistry) Lookup(surfaceID string) (domainplugin.Surface, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[surfaceID]
	return s, ok
}

// CountForPlugin reports how many surfaces a plugin currently holds. Used by tests and by the
// presentation layer's diagnostics; the cap itself is enforced inside Add, where the check and the
// insertion share one critical section.
func (r *SurfaceRegistry) CountForPlugin(pluginID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byPlugin[pluginID])
}

func (r *SurfaceRegistry) removeIndexed(index map[string]map[string]struct{}, key string) []domainplugin.Surface {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := index[key]
	removed := make([]domainplugin.Surface, 0, len(ids))
	for id := range ids {
		s, ok := r.byID[id]
		if !ok {
			continue
		}
		r.deleteLocked(s)
		removed = append(removed, s)
	}
	return removed
}

// lookupLocked resolves a surface and its owner. Callers hold r.mu.
func (r *SurfaceRegistry) lookupLocked(surfaceID, pluginID string) (domainplugin.Surface, error) {
	s, ok := r.byID[surfaceID]
	if !ok || s.PluginID != pluginID {
		return domainplugin.Surface{}, fmt.Errorf("%w: unknown surface", domainplugin.ErrCapabilityDenied)
	}
	return s, nil
}

// deleteLocked removes a surface from all three maps. Callers hold r.mu.
func (r *SurfaceRegistry) deleteLocked(s domainplugin.Surface) {
	delete(r.byID, s.ID)
	removeFromIndex(r.byPlugin, s.PluginID, s.ID)
	removeFromIndex(r.bySession, s.ParentSessionID, s.ID)
}

func addToIndex(index map[string]map[string]struct{}, key, id string) {
	set, ok := index[key]
	if !ok {
		set = make(map[string]struct{})
		index[key] = set
	}
	set[id] = struct{}{}
}

func removeFromIndex(index map[string]map[string]struct{}, key, id string) {
	set, ok := index[key]
	if !ok {
		return
	}
	delete(set, id)
	if len(set) == 0 {
		delete(index, key)
	}
}
