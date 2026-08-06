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
  /** UTF-8 size, kept so eviction subtracts what insertion added. */
  bytes: number;
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
   * Incremented on every mutation.
   *
   * The viewer watches this instead of copying the line array on each chunk: the copy was O(n) per
   * write, which on a chatty producer is the whole buffer re-walked hundreds of times a second.
   */
  private rev = 0;
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
    const bytes = utf8Length(text);
    this.lines.push({ seq: this.seq++, text, stream, bytes });
    this.bytes += bytes;
    this.rev++;
    while (this.lines.length > this.maxLines || this.bytes > this.maxBytes) {
      const removed = this.lines.shift();
      if (!removed) break;
      this.bytes -= removed.bytes;
      this.dropped = true;
    }
  }

  /**
   * The live line array. Not a copy: the viewer renders a window of it and re-reads on `revision`,
   * so handing back a duplicate of up to MAX_LOG_LINES entries per chunk would be the cost this
   * buffer exists to avoid. Callers must not mutate it.
   */
  snapshot(): readonly LogLine[] {
    return this.lines;
  }

  /** Changes on every append or drop. The viewer's only reason to re-read. */
  revision(): number {
    return this.rev;
  }

  truncated(): boolean {
    return this.dropped;
  }

  clear(): void {
    this.lines = [];
    this.bytes = 0;
    this.dropped = false;
    this.rev++;
    this.partial = { stdout: '', stderr: '' };
  }
}

/**
 * Bytes a string occupies in UTF-8.
 *
 * The host bounds a log surface in bytes (MaxLogSurfaceBytes), and `String.length` counts UTF-16
 * code units — for anything but ASCII the two differ by two or three times, so the buffer would
 * hold that much more or less than the limit it claims to mirror.
 */
function utf8Length(text: string): number {
  let bytes = 0;
  for (let i = 0; i < text.length; i++) {
    const code = text.charCodeAt(i);
    if (code < 0x80) bytes += 1;
    else if (code < 0x800) bytes += 2;
    else if (code >= 0xd800 && code <= 0xdbff) {
      // A surrogate pair is one 4-byte character; skip its low half.
      bytes += 4;
      i++;
    } else bytes += 3;
  }
  return bytes;
}
