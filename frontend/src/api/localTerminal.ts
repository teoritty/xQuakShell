// Atomic local terminal RPC wrappers. No logic here, matching api/surfaces.ts and api/terminal.ts
// which this sits beside: the frontend's only way to reach Go is a thin call.
import { getGateway } from '../backend/context';

/** Encodes text as base64 the way the other two terminal input paths do. */
function toBase64(data: string): string {
  const bytes = new TextEncoder().encode(data);
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary);
}

export interface LocalTerminalTab {
  id: string;
  title: string;
}

export interface ShellOption {
  id: string;
  name: string;
}

/**
 * Opens a shell on this machine.
 *
 * Takes no arguments, and must never take any. Nothing the frontend sends may influence which
 * program runs - the shell is chosen in settings by id and resolved on the Go side against a
 * closed set. A path or a command added here would be the whole command-injection surface this
 * feature was designed not to have.
 */
export async function openLocalTerminal(): Promise<LocalTerminalTab | null> {
  const app = getGateway();
  if (!app) return null;
  return app.OpenLocalTerminal();
}

export async function closeLocalTerminal(id: string): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.CloseLocalTerminal(id);
  } catch (e) {
    console.debug('[local terminal close]', id, e);
  }
}

export async function sendLocalTerminalInput(id: string, data: string): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.SendLocalTerminalInput(id, toBase64(data));
  } catch (e) {
    console.debug('[local terminal input]', id, e);
  }
}

export async function resizeLocalTerminal(id: string, cols: number, rows: number): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.ResizeLocalTerminal(id, cols, rows);
  } catch {
    // Non-critical, exactly as on the session and surface paths: the next resize supersedes this
    // one, and there is nothing useful to tell the user about a geometry that did not stick.
  }
}

/** Lists the shells this machine has, for the settings picker. */
export async function listLocalShells(): Promise<ShellOption[]> {
  const app = getGateway();
  if (!app) return [];
  try {
    return (await app.ListLocalShells()) ?? [];
  } catch (e) {
    console.debug('[local shells]', e);
    return [];
  }
}
