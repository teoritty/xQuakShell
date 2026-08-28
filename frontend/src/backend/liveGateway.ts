// Moves the untyped `window as any` access to exactly one place, so the rest
// of the codebase interacts with the Wails bridge only through the typed
// AppGateway/RuntimeGateway interfaces.
import type { AppGateway, RuntimeGateway } from './gateway';

/**
 * Resolves the Wails bridge for whichever window this bundle is running in.
 *
 * There are two. The main window binds `App` in package main; the log viewer is a separate
 * process that binds `LogViewerApp` in package logwindow, and Wails builds the path from those
 * names, so the two land in different places. The viewer runs this same bundle, and when the
 * lookup knew only about the first path every backend call it made returned the fallback -
 * which is how its captions rendered as raw message keys instead of text.
 *
 * The viewer's object implements a small part of AppGateway, which is why every member is
 * optional and callBackend probes for the method before calling it.
 */
export function liveApp(): AppGateway | null {
  const go = (window as any).go;
  return ((go?.main?.App ?? go?.logwindow?.LogViewerApp) as AppGateway) ?? null;
}

export function liveRuntime(): RuntimeGateway | null {
  return ((window as any).runtime as RuntimeGateway) ?? null;
}
