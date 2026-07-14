// Atomic terminal RPC wrappers. Matches the original stores/api.ts bodies
// exactly — these were already atomic (no separate orchestration wrapper).
import { getGateway } from '../backend/context';

export async function sendTerminalInput(sessionId: string, data: string, commandLine = ''): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.SendTerminalInput(sessionId, data, commandLine);
  } catch (e) {
    console.debug('[terminal input]', sessionId, e);
  }
}

export async function terminalResize(sessionId: string, cols: number, rows: number): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.TerminalResize(sessionId, cols, rows);
  } catch (e) {
    // resize errors are non-critical
  }
}
