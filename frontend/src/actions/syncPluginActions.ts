// Plugin sync orchestration (spec 6.4-6.5): the panel's two lists, installing an offered plugin with
// the consents the install dialog collected, and the per-plugin data actions. Each returns the
// message to show - an empty string is success - like the other sync actions.
import {
  installSyncPluginOfferRpc,
  keepSyncPluginRpc,
  listSyncPluginDataRpc,
  listSyncPluginOffersRpc,
  restartSyncPluginRpc,
  setSyncPluginDataRpc,
  type SyncPluginData,
  type SyncPluginOffer,
} from '../api/syncPlugins';
import { errorMessage } from './syncActions';
import type { ConsentAnswers } from '../lib/plugins/installConsent';

async function attempt(fn: () => Promise<unknown>): Promise<string> {
  try {
    await fn();
    return '';
  } catch (e) {
    return errorMessage(e);
  }
}

export interface PluginSyncLists {
  offers: SyncPluginOffer[];
  data: SyncPluginData[];
  error: string;
}

/**
 * Loads both lists. One failing does not blank the other: offers and data come from different
 * services and fail for different reasons.
 */
export async function loadPluginSync(): Promise<PluginSyncLists> {
  let error = '';
  const keep = (e: unknown) => {
    error ||= errorMessage(e);
  };
  const [offers, data] = await Promise.all([
    listSyncPluginOffersRpc().catch((e: unknown) => (keep(e), [] as SyncPluginOffer[])),
    listSyncPluginDataRpc().catch((e: unknown) => (keep(e), [] as SyncPluginData[])),
  ]);
  return { offers: offers ?? [], data: data ?? [], error };
}

/**
 * Installs an offered plugin. status is 'mismatch' when the release differs from the build the
 * other device recorded: nothing was installed, and the caller asks whether to keep it anyway.
 */
export async function installOfferedPlugin(
  pluginId: string,
  consents: ConsentAnswers,
  acceptDifferent: boolean,
): Promise<{ status: '' | 'installed' | 'mismatch'; error: string }> {
  try {
    const res = await installSyncPluginOfferRpc({
      pluginId,
      secret: consents.secret,
      auth: consents.auth,
      tunnel: consents.tunnel,
      multiSession: consents.multiSession,
      network: consents.network,
      exec: consents.exec,
      acceptDifferent,
    });
    return { status: res?.status ?? '', error: res ? '' : 'Sync is unavailable.' };
  } catch (e) {
    return { status: '', error: errorMessage(e) };
  }
}

export const keepPlugin = (pluginId: string) => attempt(() => keepSyncPluginRpc(pluginId));
export const setPluginDataSync = (pluginId: string, enabled: boolean) =>
  attempt(() => setSyncPluginDataRpc(pluginId, enabled));
export const restartPluginForData = (pluginId: string) => attempt(() => restartSyncPluginRpc(pluginId));
