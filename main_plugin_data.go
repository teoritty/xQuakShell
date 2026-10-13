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

// wirePluginDataSync keeps the files plugins declare in sync.paths in step with the vault mirror
// sync carries (spec §6.5). The vault's change observer already asks the sync controller for a
// round whenever the mirror changes, so the service needs no notifier of its own.
//
// Three moments write a plugin's directory, all under the plugin's lock: the start guard, before a
// process comes up; a stop or a suspension; and the periodic scan, which writes only for a plugin
// that is idle at that moment.
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
}
