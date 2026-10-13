package main

import (
	"context"
	"log"
	"path/filepath"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/persistence"
	infraplugin "xquakshell/internal/infra/plugin"
	"xquakshell/internal/pkg/safego"
	"xquakshell/internal/usecase"
)

// wirePluginDataSync wires both halves of plugin sync. It keeps the files plugins declare in sync.paths in step with the vault mirror
// sync carries (spec §6.5). The vault's change observer already asks the sync controller for a
// round whenever the mirror changes, so the service needs no notifier of its own.
//
// Three moments write a plugin's directory, all under the plugin's lock: the start guard, before a
// process comes up; a stop or a suspension; and the periodic scan, which writes only for a plugin
// that is idle at that moment. The plugins themselves are recorded by reference on the same cadence
// (spec §6.4).
func wirePluginDataSync(rt *pluginRuntime, zone domain.SyncedFileZone, vaultRepo *persistence.VaultRepo, dataRoot string) {
	if rt == nil || rt.manager == nil {
		return
	}
	svc := usecase.NewPluginDataSync(usecase.PluginDataSyncDeps{
		Zone:    zone,
		Vault:   vaultRepo,
		Catalog: rt.manager,
		DataDir: func(pluginID string) string {
			rel, err := filepath.Rel(dataRoot, infraplugin.PluginDataDir(dataRoot, pluginID))
			if err != nil {
				return filepath.Join("plugins", "invalid")
			}
			return filepath.ToSlash(rel)
		},
	})
	// The scan loop lives as long as the process; shutdown cancels it with the rest of the runtime.
	ctx, cancel := context.WithCancel(context.Background())
	rt.stopPluginData = cancel
	rt.pluginData = svc
	rt.manager.SetStartGuard(svc.GuardStart)
	rt.manager.SetProcessIdleHandler(func(pluginID string) {
		safego.GoNamed("pluginData.onIdle", func() {
			if _, err := svc.Reconcile(ctx, pluginID); err != nil {
				log.Printf("plugin data: reconcile %s after stop: %v", pluginID, err)
			}
		})
	})
	safego.GoNamed("pluginData.scan", func() { svc.Run(ctx) })

	manager := rt.manager
	refs := usecase.NewPluginReferenceSync(usecase.PluginReferenceDeps{
		Vault:     vaultRepo,
		Installed: manager.Registry().List,
		// A plugin uninstalled on another device is disabled and stopped here; uninstalling it would
		// delete its data, which is the user's call on this device.
		Disable: func(ctx context.Context, pluginID string) error {
			if err := manager.SetPluginEnabled(ctx, pluginID, false); err != nil {
				return err
			}
			return manager.StopPlugin(ctx, pluginID)
		},
	})
	rt.pluginRefs = refs
	safego.GoNamed("pluginRefs.record", func() { refs.Run(ctx) })
}
