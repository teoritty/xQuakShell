package usecase

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

// ErrPluginDigestMismatch is an install whose manifest or SHA256SUMS differs from the plugin the
// user's other devices recorded: a tag moved, or a release was replaced, behind the same name.
var ErrPluginDigestMismatch = errors.New("the installed plugin differs from the one on your other devices")

// PluginOfferState says what the user can do about a plugin another device recorded.
type PluginOfferState string

const (
	// PluginOfferInstall is a GitHub plugin this device lacks.
	PluginOfferInstall PluginOfferState = "install"
	// PluginOfferManual is a plugin installed from a file elsewhere; it can only be installed by hand.
	PluginOfferManual PluginOfferState = "manual"
	// PluginOfferMismatch is a plugin installed here whose build differs from the recorded one.
	PluginOfferMismatch PluginOfferState = "mismatch"
	// PluginOfferRemoved is a plugin another device uninstalled; it is disabled here.
	PluginOfferRemoved PluginOfferState = "removed"
	// PluginOfferUpdate is a plugin another device upgraded or changed; this device has the build it
	// recorded before.
	PluginOfferUpdate PluginOfferState = "update"
)

// PluginOffer is one plugin the Plugins from your other devices list shows.
type PluginOffer struct {
	Plugin domain.SyncPlugin
	State  PluginOfferState
}

// PluginReferenceDeps wires PluginReferenceSync.
type PluginReferenceDeps struct {
	Vault pluginDataVault
	// Installed lists the plugins installed on this device.
	Installed func() []domainplugin.InstalledPlugin
	// Disable turns a plugin off on this device, for one another device uninstalled.
	Disable func(ctx context.Context, pluginID string) error
}

// PluginReferenceSync mirrors the plugins installed on this device into the vault by reference and
// tells the user about the ones installed elsewhere (spec §6.4). Nothing here installs or
// uninstalls: an offer is installed through the normal pipeline, and an uninstall elsewhere only
// disables the plugin here, because uninstalling deletes its data.
type PluginReferenceSync struct {
	d  PluginReferenceDeps
	mu sync.Mutex
}

// NewPluginReferenceSync creates the service.
func NewPluginReferenceSync(d PluginReferenceDeps) *PluginReferenceSync {
	return &PluginReferenceSync{d: d}
}

// refOf is the reference a plugin installed here would be recorded as. Bundled plugins ship with
// the application and are not recorded.
func refOf(p domainplugin.InstalledPlugin) (domain.SyncPlugin, bool) {
	if p.Source != domainplugin.SourceUser || p.ManifestDigest == "" {
		return domain.SyncPlugin{}, false
	}
	ref := domain.SyncPlugin{ID: p.Manifest.ID, Version: p.Manifest.Version, ManifestDigest: p.ManifestDigest,
		SumsDigest: p.ChecksumsDigest, Source: domain.SyncPluginSource{Kind: domain.PluginSourceManual}}
	if m := p.InstallMeta; m != nil && m.Source == domainplugin.InstallMetaSourceGitHub && m.ReleaseTag != "" {
		if repo, ok := githubRepoOf(m.RepositoryURL); ok {
			ref.Source = domain.SyncPluginSource{Kind: domain.PluginSourceGitHub, Repo: repo, Tag: m.ReleaseTag}
		}
	}
	return ref, true
}

// githubRepoOf reads owner/name from a GitHub repository URL.
func githubRepoOf(url string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(url), "https://github.com/")
	if !ok {
		return "", false
	}
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git")
	owner, name, ok := strings.Cut(rest, "/")
	return rest, ok && owner != "" && name != "" && !strings.Contains(name, "/")
}

func (s *PluginReferenceSync) installedRefs() map[string]domain.SyncPlugin {
	out := map[string]domain.SyncPlugin{}
	for _, p := range s.d.Installed() {
		if ref, ok := refOf(p); ok {
			out[ref.ID] = ref
		}
	}
	return out
}

// Record brings the vault's plugin records in step with what is installed here.
func (s *PluginReferenceSync) Record(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	installed := s.installedRefs()
	var removed []string
	err := s.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync == nil || d.Sync.Account == nil {
			return nil
		}
		removed = recordPlugins(d.Sync, installed)
		return nil
	})
	if err != nil {
		return err
	}
	for _, id := range removed {
		if err := s.d.Disable(ctx, id); err != nil {
			slog.Warn("plugin removed on another device; disabling it here failed", "pluginId", id, "err", err)
		}
	}
	return nil
}

// recordPlugins applies one Record pass to the vault's sync state and returns the plugins that
// another device just uninstalled. PluginsHere is what tells the cases apart: a plugin this device
// recorded and no longer has was uninstalled here; one it recorded whose record is gone was
// uninstalled elsewhere; one whose record changed while the build here did not was changed
// elsewhere and is offered, not reverted; one it never recorded whose record differs is an
// unverified build.
func recordPlugins(vs *domain.VaultSync, installed map[string]domain.SyncPlugin) []string {
	records := map[string]domain.SyncPlugin{}
	for _, p := range vs.Plugins {
		records[p.ID] = p
	}
	if vs.PluginsHere == nil {
		vs.PluginsHere = map[string]string{}
	}
	var removed []string
	for _, id := range slices.Sorted(maps.Keys(installed)) {
		ref := installed[id]
		rec, has := records[id]
		last, here := vs.PluginsHere[id]
		switch {
		case slices.Contains(vs.PluginsRemovedElsewhere, id):
		case here && !has:
			vs.PluginsRemovedElsewhere = append(vs.PluginsRemovedElsewhere, id)
			removed = append(removed, id)
		case has && rec != ref && (!here || buildOf(ref) == last):
			// An unverified build, or a change made elsewhere: left for the user.
		default:
			upsertPlugin(vs, ref)
			vs.PluginsHere[id] = buildOf(ref)
		}
	}
	for id := range vs.PluginsHere {
		if _, ok := installed[id]; !ok {
			delete(vs.PluginsHere, id)
			vs.Plugins = slices.DeleteFunc(vs.Plugins, func(p domain.SyncPlugin) bool { return p.ID == id })
		}
	}
	vs.PluginsRemovedElsewhere = slices.DeleteFunc(vs.PluginsRemovedElsewhere, func(id string) bool {
		_, ok := installed[id]
		return !ok
	})
	return removed
}

// buildOf identifies the exact build of a plugin: its manifest and its checksum list.
func buildOf(ref domain.SyncPlugin) string { return ref.ManifestDigest + ":" + ref.SumsDigest }

func upsertPlugin(vs *domain.VaultSync, ref domain.SyncPlugin) {
	if i := slices.IndexFunc(vs.Plugins, func(p domain.SyncPlugin) bool { return p.ID == ref.ID }); i >= 0 {
		if vs.Plugins[i] != ref {
			vs.Plugins[i] = ref
		}
		return
	}
	vs.Plugins = append(vs.Plugins, ref)
}

// Offers lists the plugins another device recorded that need the user here.
func (s *PluginReferenceSync) Offers(_ context.Context) ([]PluginOffer, error) {
	data, err := s.d.Vault.GetData()
	if err != nil || data.Sync == nil {
		return nil, err
	}
	installed := s.installedRefs()
	var out []PluginOffer
	for _, rec := range data.Sync.Plugins {
		ref, isInstalled := installed[rec.ID]
		switch {
		case !isInstalled && rec.Source.Kind == domain.PluginSourceManual:
			out = append(out, PluginOffer{Plugin: rec, State: PluginOfferManual})
		case !isInstalled:
			out = append(out, PluginOffer{Plugin: rec, State: PluginOfferInstall})
		case ref == rec:
		case data.Sync.PluginsHere[rec.ID] == "":
			out = append(out, PluginOffer{Plugin: rec, State: PluginOfferMismatch})
		case data.Sync.PluginsHere[rec.ID] == buildOf(ref):
			out = append(out, PluginOffer{Plugin: rec, State: PluginOfferUpdate})
		}
	}
	for _, id := range data.Sync.PluginsRemovedElsewhere {
		if ref, ok := installed[id]; ok {
			out = append(out, PluginOffer{Plugin: ref, State: PluginOfferRemoved})
		}
	}
	slices.SortFunc(out, func(a, b PluginOffer) int { return strings.Compare(a.Plugin.ID, b.Plugin.ID) })
	return out, nil
}

// VerifyInstalled compares a plugin just installed from an offer with its record. A mismatch is
// returned for the caller to roll the install back unless the user accepts it as a new version.
func (s *PluginReferenceSync) VerifyInstalled(ctx context.Context, pluginID string) error {
	ref, ok := s.installedRefs()[pluginID]
	if !ok {
		return nil
	}
	data, err := s.d.Vault.GetData()
	if err != nil || data.Sync == nil {
		return err
	}
	i := slices.IndexFunc(data.Sync.Plugins, func(p domain.SyncPlugin) bool { return p.ID == pluginID })
	if i < 0 {
		return s.Record(ctx)
	}
	if data.Sync.Plugins[i] != ref {
		return ErrPluginDigestMismatch
	}
	return s.Record(ctx)
}

// AcceptInstalled keeps the build installed here: as the new version when it differs from the
// record, or again after another device uninstalled the plugin.
func (s *PluginReferenceSync) AcceptInstalled(ctx context.Context, pluginID string) error {
	ref, ok := s.installedRefs()[pluginID]
	if !ok {
		return domainplugin.ErrPluginNotFound
	}
	s.mu.Lock()
	err := s.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync == nil {
			return nil
		}
		d.Sync.PluginsRemovedElsewhere = slices.DeleteFunc(d.Sync.PluginsRemovedElsewhere, func(id string) bool { return id == pluginID })
		upsertPlugin(d.Sync, ref)
		if d.Sync.PluginsHere == nil {
			d.Sync.PluginsHere = map[string]string{}
		}
		d.Sync.PluginsHere[pluginID] = buildOf(ref)
		return nil
	})
	s.mu.Unlock()
	return err
}

// Run records every PluginDataScanInterval until ctx ends; installs and uninstalls also record at
// once through their callers.
func (s *PluginReferenceSync) Run(ctx context.Context) {
	t := time.NewTicker(PluginDataScanInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Record(ctx); err != nil && !errors.Is(err, domain.ErrVaultLocked) {
				slog.Warn("plugin references: record failed", "err", err)
			}
		}
	}
}
