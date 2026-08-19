// Atomic plugin-source RPC wrappers.
//
// Guards on method presence rather than only on gateway presence, matching api/plugins.ts: a
// build whose Go side predates this handler has the gateway but not the method, and calling it
// would throw where an empty list reads as "no sources yet".
import { getGateway } from '../backend/context';
import type { PluginSourceDTO } from '../backend/gateway';
import { showError } from '../stores/appState';

export type { PluginSourceDTO };

export type PluginSourceKind = PluginSourceDTO['kind'];

export async function listPluginSources(): Promise<PluginSourceDTO[]> {
  const app = getGateway();
  if (!app?.ListPluginSources) return [];
  try {
    return await app.ListPluginSources();
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    showError(`List plugin sources: ${msg}`, e instanceof Error && e.stack ? e.stack : '');
    return [];
  }
}
