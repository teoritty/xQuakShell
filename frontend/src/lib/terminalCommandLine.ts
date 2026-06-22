import type { Terminal } from '@xterm/xterm';

/** Returns true when data contains an Enter key transmission. */
export function dataHasEnter(data: string): boolean {
  return data.includes('\r') || data.includes('\n');
}

/**
 * Reads the current shell input line from the xterm buffer at Enter time.
 * The remote shell (readline) owns line editing; the buffer reflects what is on screen.
 */
export function extractCommandLine(term: Terminal): string {
  const buf = term.buffer.active;
  if (buf.type === 'alternate') {
    return '';
  }

  const row = buf.baseY + buf.cursorY;
  const line = buf.getLine(row);
  if (!line) return '';

  const raw = line.translateToString(true);
  return stripShellPrompt(raw).trim();
}

/** Removes common interactive shell prompt prefixes from a buffer line. */
export function stripShellPrompt(line: string): string {
  // Bracket prompts: [user@host path]$ command
  const bracket = line.match(/\][#$%]\s+(.*)$/);
  if (bracket?.[1] !== undefined) {
    return bracket[1];
  }

  const markers = ['$ ', '# ', '% ', '> '];
  let bestIdx = -1;
  let bestLen = 0;
  for (const m of markers) {
    const idx = line.lastIndexOf(m);
    if (idx > bestIdx) {
      bestIdx = idx;
      bestLen = m.length;
    }
  }
  if (bestIdx >= 0) {
    return line.slice(bestIdx + bestLen);
  }
  return line;
}
