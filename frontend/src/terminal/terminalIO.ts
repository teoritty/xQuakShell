import { getRuntime } from '../backend/context';

// What a terminal renderer needs from whatever is on the other end of it.
//
// There are two producers of terminal bytes now — an SSH session and a plugin-owned surface
// (ADR-015) — and exactly one renderer worth having. Everything that differs between them is
// here: the id the output buffer and the terminal pool are keyed by, how output arrives, and
// where input and resizes go. Terminal.svelte knows none of it.
export interface TerminalIO {
  /** Key for the shared output buffer and the terminal pool. Unique across both producers. */
  readonly id: string;
  /**
   * Starts delivering this stream's output. The callback receives base64 exactly as it arrived:
   * decoding belongs to the renderer, which is the only part that knows the bytes are meant for a
   * terminal at all.
   *
   * Returns an unsubscribe function.
   */
  subscribe(onData: (base64: string) => void): () => void;
  sendInput(data: string, commandLine: string): void;
  resize(cols: number, rows: number): void;
}

/**
 * Subscribes to a Wails event, keeping only the payloads whose id field matches.
 *
 * Both producers broadcast one event for every stream, so filtering by id is what makes a
 * subscription per-stream. Doing it here rather than in each consumer is what keeps them from
 * knowing which id field a payload carries.
 */
export function subscribeByIdRaw<T extends Record<string, unknown>>(
  event: string,
  idField: string,
  id: string,
  onPayload: (payload: T) => void
): () => void {
  const rt = getRuntime();
  if (!rt) return () => {};
  const off = rt.EventsOn(event, (payload: T) => {
    if (!payload || payload[idField] !== id) return;
    onPayload(payload);
  });
  return typeof off === 'function' ? off : () => {};
}

/**
 * The common case: one field of the payload is the data and nothing else matters. The log surface
 * uses subscribeByIdRaw instead, because for it stdout and stderr are not the same bytes.
 */
export function subscribeById(
  event: string,
  idField: string,
  id: string,
  dataField: string,
  onData: (base64: string) => void
): () => void {
  return subscribeByIdRaw<Record<string, string>>(event, idField, id, (payload) => {
    onData(payload[dataField] ?? '');
  });
}
