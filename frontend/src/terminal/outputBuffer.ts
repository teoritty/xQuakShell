const MAX_PENDING_TERMINAL_BYTES = 256 << 10;
const pendingTerminalOutput = new Map<string, Uint8Array[]>();
const terminalOutputConsumers = new Map<string, number>();

export function decodeTerminalOutput(output: string): Uint8Array {
  try {
    return Uint8Array.from(atob(output), (c) => c.charCodeAt(0));
  } catch {
    return new TextEncoder().encode(output);
  }
}

export function appendPendingTerminalOutput(sessionId: string, bytes: Uint8Array): void {
  if (bytes.length === 0) return;
  let chunks = pendingTerminalOutput.get(sessionId);
  if (!chunks) {
    chunks = [];
    pendingTerminalOutput.set(sessionId, chunks);
  }
  chunks.push(bytes);
  let total = 0;
  for (const chunk of chunks) {
    total += chunk.length;
  }
  while (total > MAX_PENDING_TERMINAL_BYTES && chunks.length > 1) {
    const removed = chunks.shift()!;
    total -= removed.length;
  }
  if (total > MAX_PENDING_TERMINAL_BYTES && chunks.length === 1) {
    const overflow = total - MAX_PENDING_TERMINAL_BYTES;
    chunks[0] = chunks[0].slice(overflow);
  }
}

/** Returns buffered output emitted before the terminal component mounted. */
export function takePendingTerminalOutput(sessionId: string): Uint8Array[] {
  const chunks = pendingTerminalOutput.get(sessionId) ?? [];
  pendingTerminalOutput.delete(sessionId);
  return chunks;
}

export function clearPendingTerminalOutput(sessionId: string): void {
  pendingTerminalOutput.delete(sessionId);
}

/**
 * Marks a session as having a live terminal subscriber (skip global buffering).
 * Ref-counted: during a tile rearrangement the new Terminal component can mount
 * (and register) before the old one unmounts (and unregisters), so a plain flag
 * would briefly drop to "no consumer" and cause api.ts to buffer output that the
 * live terminal is already displaying — producing duplicated lines on the next
 * mount. Counting keeps the session marked as consumed throughout the overlap.
 */
export function registerTerminalOutputConsumer(sessionId: string): () => void {
  terminalOutputConsumers.set(sessionId, (terminalOutputConsumers.get(sessionId) ?? 0) + 1);
  let released = false;
  return () => {
    if (released) return;
    released = true;
    const next = (terminalOutputConsumers.get(sessionId) ?? 0) - 1;
    if (next <= 0) terminalOutputConsumers.delete(sessionId);
    else terminalOutputConsumers.set(sessionId, next);
  };
}

export function hasTerminalOutputConsumer(id: string) {
  return terminalOutputConsumers.has(id);
}

export function clearTerminalOutputConsumer(sessionId: string): void {
  terminalOutputConsumers.delete(sessionId);
}
