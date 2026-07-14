// Atomic plugin-runtime RPC wrappers. These guard on capability (method
// presence on the gateway) before calling, matching the original
// stores/api.ts bodies exactly, so they are implemented directly against
// getGateway/showError rather than through callBackend (which only guards
// on gateway presence, not per-method availability).
import { getGateway } from '../backend/context';
import { showError } from '../stores/appState';
import type { FieldGroup } from '../actions/protocolActions';

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

export interface PluginCommand {
  pluginId: string;
  id: string;
  fullId: string;
  title: string;
  category?: string;
}

export interface PluginContributions {
  commands: PluginCommand[];
  views: PluginView[];
  statusBar: PluginStatusBarItem[];
  authMethods: PluginAuthMethodContribution[];
  tunnelProviders: PluginTunnelProviderContribution[];
}

export interface PluginAuthMethodContribution {
  pluginId: string;
  id: string;
  label: string;
  kind: string;
  fields?: FieldGroup[];
}

export interface PluginTunnelProviderContribution {
  pluginId: string;
  id: string;
  label: string;
}

export interface PluginView {
  pluginId: string;
  id: string;
  fullId: string;
  location: string;
  title: string;
  type?: string;
  entry?: string;
  assetUrl: string;
}

export interface PluginStatusBarItem {
  pluginId: string;
  id: string;
  text: string;
  tooltip?: string;
  priority?: number;
}

export async function getPluginContributions(): Promise<PluginContributions> {
  const app = getGateway();
  if (!app?.GetPluginContributions) {
    return { commands: [], views: [], statusBar: [], authMethods: [], tunnelProviders: [] };
  }
  try {
    // GetPluginContributions returns PluginContributionsDTO (backend wire
    // type); PluginContributions is the frontend domain type. They're
    // structurally equivalent at runtime (matches original stores/api.ts,
    // which relied on `app: any`).
    return (await app.GetPluginContributions()) as unknown as PluginContributions;
  } catch (e) {
    handleError(e, 'Load plugin contributions');
    return { commands: [], views: [], statusBar: [], authMethods: [], tunnelProviders: [] };
  }
}

export async function executePluginCommand(
  pluginId: string,
  commandId: string,
  args?: Record<string, unknown>
): Promise<Record<string, string>> {
  const app = getGateway();
  if (!app?.ExecutePluginCommand) {
    throw new Error('Plugin commands are unavailable');
  }
  const rawArgs = args ? JSON.stringify(args) : null;
  const result = await app.ExecutePluginCommand(pluginId, commandId, rawArgs);
  if (!result) return {};
  if (typeof result === 'string') {
    try {
      return JSON.parse(result);
    } catch {
      return { message: result };
    }
  }
  return result as Record<string, string>;
}

export async function preparePluginViewPanel(pluginId: string, panelId: string): Promise<string> {
  const app = getGateway();
  if (!app?.PreparePluginViewPanel) {
    throw new Error('Plugin view relay is unavailable');
  }
  return await app.PreparePluginViewPanel(pluginId, panelId);
}

export async function relayPluginViewMessage(
  token: string,
  message: Record<string, unknown>
): Promise<void> {
  const app = getGateway();
  if (!app?.RelayPluginViewMessage) {
    throw new Error('Plugin view relay is unavailable');
  }
  const raw = JSON.stringify(message ?? {});
  await app.RelayPluginViewMessage(token, raw);
}

export function releasePluginViewPanel(token: string): void {
  const app = getGateway();
  app?.ReleasePluginViewPanel?.(token);
}
