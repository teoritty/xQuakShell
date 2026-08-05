// The bounded line buffer behind a log surface (ADR-015 §1).
//
// This buffer is the reason a log surface can offer search and export at all — a terminal's
// scrollback is screen cells, not lines — and a bound is the reason it cannot become an unattended
// memory leak on a chatty producer. Whichever limit is hit first wins; past either, the oldest
// lines are dropped and `truncated()` says so, because a log that silently starts later than it
// did is worse than one that admits it lost the beginning.

export type LogStream = 'stdout' | 'stderr';

export interface LogLine {
  /** Monotonic, never reused. Used as a stable key and by search to address a match. */
  seq: number;
  text: string;
  stream: LogStream;
}

/** Mirrors the host's MaxLogSurfaceBytes / MaxLogSurfaceLines (internal/domain/plugin/ui_limits.go). */
export const MAX_LOG_BYTES = 8 << 20;
export const MAX_LOG_LINES = 200000;

export class LogBuffer {
  private lines: LogLine[] = [];
  private bytes = 0;
  private seq = 0;
  private dropped = false;
  /**
   * Per-stream partial line and decoder. A chunk boundary is not a line boundary, and stdout and
   * stderr interleave — one shared partial would splice a half-line of one into the other.
   */
  private partial: Record<LogStream, string> = { stdout: '', stderr: '' };
  private decoders: Record<LogStream, TextDecoder> = {
    stdout: new TextDecoder(),
    stderr: new TextDecoder(),
  };

  constructor(
    private readonly maxBytes: number = MAX_LOG_BYTES,
    private readonly maxLines: number = MAX_LOG_LINES
  ) {}

  /**
   * Appends a chunk, splitting on newlines.
   *
   * Decoding is streaming (`{ stream: true }`) so a multi-byte character split across two chunks
   * is not mangled into replacement characters — the failure that makes a log of non-Latin text
   * look corrupted for no reason the user can act on.
   */
  append(chunk: Uint8Array, stream: LogStream): void {
    if (chunk.length === 0) return;
    const text = this.partial[stream] + this.decoders[stream].decode(chunk, { stream: true });
    const parts = text.split('\n');
    // The last element is whatever followed the final newline: a complete line only once its
    // newline arrives.
    this.partial[stream] = parts.pop() ?? '';
    for (const part of parts) {
      this.push(part.replace(/\r$/, ''), stream);
    }
  }

  /**
   * Promotes any pending partial line to a complete one. Called when the stream ends, so a
   * producer that never wrote a trailing newline does not lose its last line.
   */
  flush(): void {
    for (const stream of ['stdout', 'stderr'] as LogStream[]) {
      if (this.partial[stream].length > 0) {
        this.push(this.partial[stream], stream);
        this.partial[stream] = '';
      }
    }
  }

  private push(text: string, stream: LogStream): void {
    this.lines.push({ seq: this.seq++, text, stream });
    this.bytes += text.length;
    while (this.lines.length > this.maxLines || this.bytes > this.maxBytes) {
      const removed = this.lines.shift();
      if (!removed) break;
      this.bytes -= removed.text.length;
      this.dropped = true;
    }
  }

  snapshot(): readonly LogLine[] {
    return this.lines;
  }

  truncated(): boolean {
    return this.dropped;
  }

  clear(): void {
    this.lines = [];
    this.bytes = 0;
    this.dropped = false;
    this.partial = { stdout: '', stderr: '' };
  }
}
