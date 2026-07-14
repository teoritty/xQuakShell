// Atomic plugin-management RPC wrappers. These guard on capability (method
// presence on the gateway) before calling, matching the original
// stores/api.ts bodies exactly, so they are implemented directly against
// getGateway/showError rather than through callBackend (which only guards
// on gateway presence, not per-method availability).
//
// installPluginRpc is deliberately atomic-only: unlike the original
// installPlugin (which also invalidated/refreshed the protocol cache), this
// function performs ONLY the InstallPlugin RPC. The protocol-refresh side
// effect is recomposed at the barrel level (stores/api.ts) for now, and will
// move to actions/protocolActions.ts in a later task.
import { getGateway } from '../backend/context';
import { showError } from '../stores/appState';

export interface PluginInfo {
  id: string;
  name: string;
  version: string;
  description: string;
  source: string;
  state: string;
  requiresSecretAccess: boolean;
  signed: boolean;
  enabled: boolean;
}

export interface PluginInstallPreview {
  id: string;
  name: string;
  version: string;
  description: string;
  signed: boolean;
  signatureVerified: boolean;
  checksumPresent: boolean;
  requiresSecretAccess: boolean;
  requiresAuthProviderAccess?: boolean;
  requiresTunnelProviderAccess?: boolean;
  multiSessionWarning?: boolean;
  arbitraryNetworkWarning?: boolean;
  unsignedWarning: boolean;
  untrustedSignatureWarning: boolean;
  permissions: string[];
}

export interface PluginSettings {
  trustedPublisherKeys: string[];
  requireSignedPlugins: boolean;
}

export interface PluginPublisherKeyPair {
  publicKey: string;
  privateKey: string;
}

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

export async function listPlugins(): Promise<PluginInfo[]> {
  const app = getGateway();
  if (!app?.ListPlugins) return [];
  try {
    return await app.ListPlugins();
  } catch (e) {
    handleError(e, 'List plugins');
    return [];
  }
}

export async function pingPlugin(pluginId: string): Promise<void> {
  const app = getGateway();
  if (!app?.PingPlugin) return;
  try {
    await app.PingPlugin(pluginId);
  } catch (e) {
    handleError(e, 'Ping plugin');
  }
}

export async function setPluginEnabled(pluginId: string, enabled: boolean): Promise<void> {
  const app = getGateway();
  if (!app?.SetPluginEnabled) return;
  try {
    await app.SetPluginEnabled(pluginId, enabled);
  } catch (e) {
    handleError(e, 'Set plugin enabled');
  }
}

export async function selectPluginSourceDir(): Promise<string> {
  const app = getGateway();
  if (!app?.SelectPluginSourceDir) return '';
  try {
    return await app.SelectPluginSourceDir();
  } catch (e) {
    handleError(e, 'Select plugin folder');
    return '';
  }
}

export async function selectPluginBundleFile(): Promise<string> {
  const app = getGateway();
  if (!app?.SelectPluginBundleFile) return '';
  try {
    return await app.SelectPluginBundleFile();
  } catch (e) {
    handleError(e, 'Select plugin bundle');
    return '';
  }
}

export async function getPluginSettings(): Promise<PluginSettings> {
  const app = getGateway();
  if (!app?.GetPluginSettings) {
    return { trustedPublisherKeys: [], requireSignedPlugins: false };
  }
  try {
    return await app.GetPluginSettings();
  } catch (e) {
    handleError(e, 'Load plugin settings');
    return { trustedPublisherKeys: [], requireSignedPlugins: false };
  }
}

export async function savePluginSettings(settings: PluginSettings): Promise<void> {
  const app = getGateway();
  if (!app?.SavePluginSettings) return;
  try {
    await app.SavePluginSettings(settings);
  } catch (e) {
    handleError(e, 'Save plugin settings');
    throw e;
  }
}

export async function generatePluginPublisherKeyPair(): Promise<PluginPublisherKeyPair> {
  const app = getGateway();
  if (!app?.GeneratePluginPublisherKeyPair) {
    return { publicKey: '', privateKey: '' };
  }
  try {
    return await app.GeneratePluginPublisherKeyPair();
  } catch (e) {
    handleError(e, 'Generate publisher keys');
    return { publicKey: '', privateKey: '' };
  }
}

export async function previewPluginInstall(sourceDir: string): Promise<PluginInstallPreview> {
  const app = getGateway();
  if (!app?.PreviewPluginInstall) {
    throw new Error('Plugin install is unavailable');
  }
  return await app.PreviewPluginInstall(sourceDir);
}

/**
 * Atomic install RPC — performs ONLY the InstallPlugin call. Does NOT
 * invalidate/refresh the protocol cache; that composition lives at the
 * barrel level (stores/api.ts `installPlugin`) until Task 3.4 relocates it
 * to actions/protocolActions.ts.
 */
export async function installPluginRpc(
  sourceDir: string,
  grantSecretAccess = false,
  grantAuthProviderAccess = false,
  grantTunnelProviderAccess = false,
  grantMultiSessionAccess = false,
  grantArbitraryNetworkAccess = false,
): Promise<PluginInfo> {
  const app = getGateway();
  if (!app?.InstallPlugin) {
    throw new Error('Plugin install is unavailable');
  }
  try {
    return await app.InstallPlugin(sourceDir, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess);
  } catch (e) {
    handleError(e, 'Install plugin');
    throw e;
  }
}
