package usecase

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// PluginDataScanInterval is how often the declared files of running plugins are scanned (spec §6.5):
// plugins write their data directory directly, so nothing announces a change.
const PluginDataScanInterval = 60 * time.Second

// PluginDataCatalog is what plugin data sync needs to know about installed plugins.
type PluginDataCatalog interface {
	// SyncedManifests lists the installed plugins' manifests.
	SyncedManifests() []domainplugin.Manifest
	// ProcessIdle reports that no process of the plugin is running, starting, stopping or crashed
	// awaiting a restart: the only state in which its data directory may be written.
	ProcessIdle(pluginID string) bool
}

// pluginDataVault is the slice of the vault plugin data sync reads and writes.
type pluginDataVault interface {
	GetData() (*domain.VaultData, error)
	UpdateData(ctx context.Context, mutate func(*domain.VaultData) error) error
}

// PluginDataSyncDeps wires PluginDataSync.
type PluginDataSyncDeps struct {
	Zone    domain.SyncedFileZone
	Vault   pluginDataVault
	Catalog PluginDataCatalog
	// DataDir names a plugin's data directory under the zone's root.
	DataDir func(pluginID string) string
	// Changed asks for a sync round after the vault mirror took local changes.
	Changed func()
	// Reported, when set, receives every report with something to say: conflicts, skipped files.
	Reported func(PluginDataReport)
}

// PluginDataReport is what one reconcile of one plugin did.
type PluginDataReport struct {
	Plugin  string
	Pushed  int
	Applied int
	// Waiting counts incoming versions held back because the plugin is running.
	Waiting int
	// Skipped names files left out: over a size limit, or not declared by this device's manifest.
	Skipped []string
	// Conflicts names files edited both here and elsewhere; the local edit won.
	Conflicts []string
}

// PluginDataSync keeps the files plugins declared in sync.paths in step with the vault mirror the
// sync engine carries between devices (spec §6.5). Every pass over a plugin holds that plugin's
// lock, and so does every process start, through GuardStart: a file is written only while no
// process of the plugin can be reading it.
type PluginDataSync struct {
	d       PluginDataSyncDeps
	mu      sync.Mutex
	locks   map[string]*sync.Mutex
	waiting map[string]int
}

// NewPluginDataSync creates the service.
func NewPluginDataSync(d PluginDataSyncDeps) *PluginDataSync {
	return &PluginDataSync{d: d, locks: map[string]*sync.Mutex{}, waiting: map[string]int{}}
}

func (s *PluginDataSync) lockFor(pluginID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.locks[pluginID]
	if !ok {
		l = &sync.Mutex{}
		s.locks[pluginID] = l
	}
	return l
}

// Reconcile scans one plugin's declared files and, when the plugin is idle, applies what arrived.
func (s *PluginDataSync) Reconcile(ctx context.Context, pluginID string) (PluginDataReport, error) {
	l := s.lockFor(pluginID)
	l.Lock()
	defer l.Unlock()
	return s.reconcileLocked(ctx, pluginID, s.d.Catalog.ProcessIdle(pluginID))
}

// GuardStart applies what arrived for a plugin about to start and holds its lock until the returned
// release is called, once the start has finished: a reconcile that ran between the apply and the
// start would otherwise see an idle plugin and write under a process coming up.
func (s *PluginDataSync) GuardStart(ctx context.Context, pluginID string) func() {
	l := s.lockFor(pluginID)
	l.Lock()
	if _, err := s.reconcileLocked(ctx, pluginID, true); err != nil && !errors.Is(err, domain.ErrVaultLocked) {
		slog.Warn("plugin data: apply before start failed", "pluginId", pluginID, "err", err)
	}
	return l.Unlock
}

// ReconcileAll reconciles every installed plugin that declares synced data.
func (s *PluginDataSync) ReconcileAll(ctx context.Context) {
	for _, m := range s.d.Catalog.SyncedManifests() {
		if m.Sync == nil || len(m.Sync.Paths) == 0 {
			continue
		}
		if _, err := s.Reconcile(ctx, m.ID); err != nil && !errors.Is(err, domain.ErrVaultLocked) {
			slog.Warn("plugin data: reconcile failed", "pluginId", m.ID, "err", err)
		}
	}
}

// Run scans every PluginDataScanInterval until ctx ends.
func (s *PluginDataSync) Run(ctx context.Context) {
	t := time.NewTicker(PluginDataScanInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.ReconcileAll(ctx)
		}
	}
}

// Waiting reports, per plugin, the incoming files held back because the plugin was running.
func (s *PluginDataSync) Waiting() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.waiting))
	for id, n := range s.waiting {
		if n > 0 {
			out[id] = n
		}
	}
	return out
}

// SetSyncEnabled turns syncing of one plugin's data on or off on this device only.
func (s *PluginDataSync) SetSyncEnabled(ctx context.Context, pluginID string, enabled bool) error {
	if err := domainplugin.ValidateID(pluginID); err != nil {
		return err
	}
	return s.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync == nil {
			d.Sync = &domain.VaultSync{}
		}
		d.Sync.PluginDataOff = slices.DeleteFunc(d.Sync.PluginDataOff, func(id string) bool { return id == pluginID })
		if !enabled {
			d.Sync.PluginDataOff = append(d.Sync.PluginDataOff, pluginID)
		}
		return nil
	})
}

func (s *PluginDataSync) setWaiting(pluginID string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waiting[pluginID] = n
}
