// Atomic surface RPC wrappers (ADR-015). No logic here — matching api/terminal.ts, which this
// sits beside for the same reason: the frontend's only way to reach Go is a thin call.
import { getGateway } from '../backend/context';

/** Encodes text as base64 the way the terminal input path does. */
function toBase64(data: string): string {
  const bytes = new TextEncoder().encode(data);
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary);
}

export async function closeSurface(surfaceId: string): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.CloseSurface(surfaceId);
  } catch (e) {
    console.debug('[surface close]', surfaceId, e);
  }
}

export async function sendSurfaceInput(surfaceId: string, data: string): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.SendSurfaceInput(surfaceId, toBase64(data));
  } catch (e) {
    console.debug('[surface input]', surfaceId, e);
  }
}

export async function resizeSurface(surfaceId: string, cols: number, rows: number): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.ResizeSurface(surfaceId, cols, rows);
  } catch {
    // Resize failures are non-critical, exactly as on the session path: the next resize supersedes
    // this one, and there is nothing useful to tell the user about a geometry that did not stick.
  }
}
