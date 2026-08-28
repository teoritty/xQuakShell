// Atomic plugin-source RPC wrapper.
//
// Unlike api/plugins.ts this reports nothing to the error store. That is not a style difference:
// api/* modules may not depend on stores/appState value bindings, and the older plugin wrappers
// sit on a closed, reviewed allowlist that a new file has no business joining. Throwing is the
// better shape anyway - the caller in actions/pluginsActions.ts knows whether an empty source
// list should be an error banner or a quiet fallback, and this module does not.
//
// The method guard stays: a build whose Go side predates this handler has the gateway but not the
// method, and "no sources yet" is the honest reading of that, not a failure.
import { getGateway } from '../backend/context';
import type { PluginSourceDTO } from '../backend/gateway';

export type { PluginSourceDTO };

export type PluginSourceKind = PluginSourceDTO['kind'];

export async function listPluginSources(): Promise<PluginSourceDTO[]> {
  const app = getGateway();
  if (!app?.ListPluginSources) return [];
  return await app.ListPluginSources();
}
