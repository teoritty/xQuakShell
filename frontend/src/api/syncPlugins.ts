// Plugin sync RPC wrappers (spec 6.4-6.5), one per backend method. Errors are rethrown for the sync
// panel to show next to the plugin they concern, as api/sync.ts does for the rest of the page.
import { callBackend } from '../backend/callBackend';
import type {
  SyncPluginData,
  SyncPluginDataReport,
  SyncPluginInstallRequest,
  SyncPluginInstallResult,
  SyncPluginOffer,
} from '../backend/syncGateway';
import type { GitHubPluginPreview } from './githubPlugins';

export type { SyncPluginData, SyncPluginDataReport, SyncPluginInstallRequest, SyncPluginInstallResult, SyncPluginOffer };

const inline = { rethrow: true, silence: () => true };

export function listSyncPluginOffersRpc(): Promise<SyncPluginOffer[]> {
  return callBackend<SyncPluginOffer[]>('Plugins from other devices', [], (app) => app.ListSyncPluginOffers(), inline);
}

export function previewSyncPluginOfferRpc(pluginId: string): Promise<GitHubPluginPreview | null> {
  return callBackend<GitHubPluginPreview | null>(
    'Preview plugin',
    null,
    // reason: the generated DTO and GitHubPluginPreview are the same JSON shape; the frontend type
    // is the one the install dialog already reads.
    async (app) => (await app.PreviewSyncPluginOffer(pluginId)) as unknown as GitHubPluginPreview,
    inline,
  );
}

export function installSyncPluginOfferRpc(req: SyncPluginInstallRequest): Promise<SyncPluginInstallResult | null> {
  return callBackend<SyncPluginInstallResult | null>('Install plugin', null, (app) => app.InstallSyncPluginOffer(req), inline);
}

export function keepSyncPluginRpc(pluginId: string): Promise<void> {
  return callBackend('Keep plugin', undefined, (app) => app.KeepSyncPlugin(pluginId), inline);
}

export function listSyncPluginDataRpc(): Promise<SyncPluginData[]> {
  return callBackend<SyncPluginData[]>('Plugin data', [], (app) => app.ListSyncPluginData(), inline);
}

export function setSyncPluginDataRpc(pluginId: string, enabled: boolean): Promise<void> {
  return callBackend('Plugin data sync', undefined, (app) => app.SetSyncPluginData(pluginId, enabled), inline);
}

export function restartSyncPluginRpc(pluginId: string): Promise<void> {
  return callBackend('Restart plugin', undefined, (app) => app.RestartSyncPlugin(pluginId), inline);
}
