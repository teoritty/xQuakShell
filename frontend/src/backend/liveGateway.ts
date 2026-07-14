// Moves the untyped `window as any` access to exactly one place, so the rest
// of the codebase interacts with the Wails bridge only through the typed
// AppGateway/RuntimeGateway interfaces.
import type { AppGateway, RuntimeGateway } from './gateway';

export function liveApp(): AppGateway | null {
  return ((window as any).go?.main?.App as AppGateway) ?? null;
}

export function liveRuntime(): RuntimeGateway | null {
  return ((window as any).runtime as RuntimeGateway) ?? null;
}
