// Atomic plugin-management RPC wrappers. These guard on capability (method
// presence on the gateway) before calling, matching the original
// stores/api.ts bodies exactly, so they are implemented directly against
// getGateway/showError rather than through callBackend (which only guards
// on gateway presence, not per-method availability).
//
// installPluginRpc is deliberately atomic-only: it performs ONLY the
// InstallPlugin RPC. The composed cache-invalidate-then-refresh side effect
// lives in actions/protocolActions.ts (installPlugin), which wraps this
// function.
import { getGateway } from '../backend/context';
import type { PluginSettingsSaveResult } from '../backend/gateway';
import { showError } from '../stores/appState';

export type { PluginSettingsSaveResult };

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
  /**
   * The OS-level boundary this plugin's running processes are behind: `enforced`,
   * `enforced-partial`, `unavailable`, `disabled`, or absent when the plugin is not
   * running. `enforced-partial` is a real boundary with a dimension missing — a Linux
   * kernel below 6.7 confines the filesystem and not the network — and it is a
   * separate value so the UI cannot round it up. Where a plugin has several processes
   * the weakest one is reported, because a summary that showed the confined one would
   * claim containment the user does not have.
   */
  sandboxMode?: string;
  /**
   * Discovery icons (ADR-014), keyed by the plugin's own iconId, each value an
   * already-encoded `data:` URI. They ride along on ListPlugins on purpose:
   * icon bytes are read and cached once when the plugin enters the registry, so
   * there is no endpoint that takes a plugin id and an asset name from the
   * frontend.
   */
  discoveryIcons?: Record<string, string>;
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
  execAccessWarning?: boolean;
  unsignedWarning: boolean;
  untrustedSignatureWarning: boolean;
  permissions: string[];
}

export interface PluginSettings {
  trustedPublisherKeys: string[];
  requireSignedPlugins: boolean;
  /**
   * Let a plugin start unconfined when this platform CAN confine it and the
   * attempt failed. It does not affect a platform that cannot confine at all,
   * where plugins start unconfined regardless and no opt-in is involved.
   *
   * Off by default, and off is the only safe default: the alternative to
   * refusing a broken sandbox is a silent downgrade nobody finds out about.
   */
  allowUnsandboxedFallback: boolean;
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

/**
 * Asks a running plugin to answer, and returns what it answered with.
 *
 * The two departures from the wrappers around it are both deliberate. The backend has always
 * returned the plugin's pong payload and this threw it away, which is why a successful ping looked
 * exactly like a broken button. And a failure is raised rather than routed to the global error
 * dialog: a ping that gets no answer is the diagnosis the user asked for, not an application error,
 * and a modal that has to be dismissed is the wrong shape for a question asked from a row of icons.
 * The caller renders both outcomes.
 */
export async function pingPlugin(pluginId: string): Promise<Record<string, string>> {
  const app = getGateway();
  if (!app?.PingPlugin) throw new Error('plugin ping is unavailable');
  const result = await app.PingPlugin(pluginId);
  return result?.result ?? {};
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
    return { trustedPublisherKeys: [], requireSignedPlugins: false, allowUnsandboxedFallback: false };
  }
  try {
    return await app.GetPluginSettings();
  } catch (e) {
    handleError(e, 'Load plugin settings');
    return { trustedPublisherKeys: [], requireSignedPlugins: false, allowUnsandboxedFallback: false };
  }
}

/**
 * Persist plugin trust settings.
 *
 * `masterPassword` is only consulted by the backend when the change weakens the trust anchor —
 * adding a publisher key, dropping the signature requirement, opting out of the sandbox. Callers
 * send an empty string first and retry with the password only if the result asks for it, so the
 * rule for which changes need it stays in Go and is never duplicated here.
 */
export async function savePluginSettings(
  settings: PluginSettings,
  masterPassword = '',
): Promise<PluginSettingsSaveResult> {
  const app = getGateway();
  if (!app?.SavePluginSettings) return { saved: false, reauthRequired: false };
  try {
    return await app.SavePluginSettings(settings, masterPassword);
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
 * invalidate/refresh the protocol cache; that composition lives in
 * actions/protocolActions.ts (`installPlugin`).
 */
export async function installPluginRpc(
  sourceDir: string,
  grantSecretAccess = false,
  grantAuthProviderAccess = false,
  grantTunnelProviderAccess = false,
  grantMultiSessionAccess = false,
  grantArbitraryNetworkAccess = false,
  grantExecAccess = false,
): Promise<PluginInfo> {
  const app = getGateway();
  if (!app?.InstallPlugin) {
    throw new Error('Plugin install is unavailable');
  }
  try {
    return await app.InstallPlugin(sourceDir, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess);
  } catch (e) {
    handleError(e, 'Install plugin');
    throw e;
  }
}
