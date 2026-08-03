// Single source of the try/catch/showError boilerplate previously duplicated
// across ~90 call sites in stores/api.ts. Every backend RPC should route
// through callBackend (or callBackendVoid) instead of hand-rolling its own
// error handling.
import type { AppGateway } from './gateway';
import { getGateway } from './context';
import { showError } from '../stores/appState';

function toMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export async function callBackend<T>(
  context: string,
  fallback: T,
  fn: (app: AppGateway) => Promise<T>,
  opts: { rethrow?: boolean; silence?: (msg: string) => boolean } = {},
): Promise<T> {
  const app = getGateway();
  if (!app) return fallback;
  try {
    return await fn(app);
  } catch (e) {
    const msg = toMessage(e);
    if (!opts.silence?.(msg)) {
      const details = e instanceof Error && e.stack ? e.stack : '';
      showError(`${context}: ${msg}`, details);
    }
    if (opts.rethrow) throw e;
    return fallback;
  }
}

export function callBackendVoid(
  context: string,
  fn: (app: AppGateway) => Promise<void>,
  opts?: { rethrow?: boolean; silence?: (msg: string) => boolean },
): Promise<void> {
  return callBackend(context, undefined as void, fn, opts);
}
