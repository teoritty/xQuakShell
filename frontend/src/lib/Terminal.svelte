<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { init, Terminal, FitAddon } from 'ghostty-web';
  import { sendTerminalInput, terminalResize, getSettings } from '../stores/api';

  export let sessionId: string;
  export let active: boolean = false;

  let containerEl: HTMLDivElement;
  let term: InstanceType<typeof Terminal> | null = null;
  let fitAddon: InstanceType<typeof FitAddon> | null = null;
  let resizeObserver: ResizeObserver | null = null;
  let eventOff: (() => void) | null = null;
  let fitTimer: ReturnType<typeof setTimeout> | null = null;
  let dataDisposable: { dispose: () => void } | null = null;
  let selectionDisposable: { dispose: () => void } | null = null;
  let initDone = false;
  /** Drops TerminalOutput until subscription is installed (avoids stale lines). */
  let acceptOutput = false;
  const mountSessionId = sessionId;
  let firstAcceptedChunkLogged = false;
  let mouseModeEnabled = false;
  let inAltScreen = false;

  function countNonEmptyLines(): number {
    if (!term) return -1;
    try {
      const b = (term as any).buffer?.active;
      if (!b) return -1;
      let nonEmpty = 0;
      const length = b.length ?? 0;
      for (let i = 0; i < length; i++) {
        const line = b.getLine(i);
        if (!line) continue;
        const txt = line.translateToString(true);
        if (txt.trim().length > 0) nonEmpty++;
      }
      return nonEmpty;
    } catch {
      return -1;
    }
  }

  function firstNonEmptyLines(limit = 3): string[] {
    if (!term) return [];
    try {
      const b = (term as any).buffer?.active;
      if (!b) return [];
      const out: string[] = [];
      const length = b.length ?? 0;
      for (let i = 0; i < length && out.length < limit; i++) {
        const line = b.getLine(i);
        if (!line) continue;
        const txt = line.translateToString(true);
        if (txt.trim().length > 0) out.push(txt.slice(0, 120));
      }
      return out;
    } catch {
      return [];
    }
  }

  function hardClearTerminalState() {
    if (!term) return;
    try {
      // Ghostty/xterm renderer can keep stale buffer state between remounts in some reconnect paths.
      // This sequence force-resets both viewport and scrollback before new session output arrives.
      term.reset();
      term.clear();
      term.write('\x1bc\x1b[2J\x1b[3J\x1b[H');
      term.clear();
    } catch {}
  }

  function sendMouseFallback(e: MouseEvent) {
    if (!term || !containerEl) return;
    const rect = containerEl.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return;
    const x = Math.max(0, Math.min(rect.width - 1, e.clientX - rect.left));
    const y = Math.max(0, Math.min(rect.height - 1, e.clientY - rect.top));
    const col = Math.max(1, Math.min(term.cols, Math.floor((x / rect.width) * term.cols) + 1));
    const row = Math.max(1, Math.min(term.rows, Math.floor((y / rect.height) * term.rows) + 1));
    const btn = e.button === 2 ? 2 : e.button === 1 ? 1 : 0;
    // Some terminal backends do not surface native mouse escape sequences from canvas clicks.
    // For TUI apps (like htop), we synthesize standard SGR mouse press/release sequences.
    const press = `\x1b[<${btn};${col};${row}M`;
    const release = `\x1b[<${btn};${col};${row}m`;
    sendTerminalInput(sessionId, press);
    sendTerminalInput(sessionId, release);
  }

  function isAltBufferActive(): boolean {
    if (!term) return false;
    try {
      const b = (term as any).buffer;
      // Some TUIs switch to alternate screen without reliably exposing mouse-mode
      // toggles in stream chunks. Buffer identity is a robust fallback signal.
      return !!(b && b.active && b.alternate && b.active === b.alternate);
    } catch {
      return false;
    }
  }

  const defaultTheme = {
    background: '#1e1e1e',
    foreground: '#cccccc',
    cursor: '#ffffff',
    selectionBackground: 'rgba(255, 255, 255, 0.30)',
    selectionForeground: '#000000',
    selectionInactiveBackground: 'rgba(255, 255, 255, 0.15)',
    black: '#1e1e1e',
    red: '#f44747',
    green: '#6a9955',
    yellow: '#d7ba7d',
    blue: '#569cd6',
    magenta: '#c586c0',
    cyan: '#4ec9b0',
    white: '#d4d4d4',
    brightBlack: '#808080',
    brightRed: '#f44747',
    brightGreen: '#6a9955',
    brightYellow: '#d7ba7d',
    brightBlue: '#569cd6',
    brightMagenta: '#c586c0',
    brightCyan: '#4ec9b0',
    brightWhite: '#e0e0e0',
  };

  function debouncedFit() {
    if (fitTimer) clearTimeout(fitTimer);
    fitTimer = setTimeout(() => {
      if (fitAddon && term && containerEl?.offsetHeight > 0) {
        try { fitAddon.fit(); } catch {}
      }
    }, 30);
  }

  onMount(async () => {
    try {
      await init();
      initDone = true;
    } catch (e) {
      console.error('ghostty-web init failed:', e);
      return;
    }

    const settings = await getSettings();
    const fontSize = settings?.terminalFontSize ?? 14;
    const fontFamily = settings?.terminalFontFamily || 'Cascadia Code, Consolas, Courier New, monospace';
    const fontColor = settings?.terminalFontColor || '#cccccc';
    const theme = { ...defaultTheme, foreground: fontColor };

    term = new Terminal({
      cursorBlink: true,
      fontSize,
      fontFamily,
      theme,
      allowTransparency: false,
      scrollback: 5000,
      convertEol: false,
      scrollOnUserInput: true,
      scrollOnTerminalWrite: true,
    });

    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(containerEl);
    await tick();

    dataDisposable = term.onData((data) => {
      sendTerminalInput(sessionId, data);
    });

    selectionDisposable = term.onSelectionChange(() => {
      const sel = term?.getSelection();
      if (sel) navigator.clipboard.writeText(sel).catch(() => {});
    });

    containerEl.addEventListener('contextmenu', async (e) => {
      e.preventDefault();
      try {
        const text = await navigator.clipboard.readText();
        if (text) sendTerminalInput(sessionId, text);
      } catch {}
    });
    containerEl.addEventListener('mousedown', (e) => {
      if (e.button === 0 || e.button === 1 || e.button === 2) {
        const altBuffer = isAltBufferActive();
        // Keep native selection/click behavior in normal shell mode.
        // Enable synthetic mouse only for TUI-like modes:
        // - explicit mouse reporting enabled by app, OR
        // - alternate screen detected (htop/less/vim style apps).
        if (!mouseModeEnabled && !inAltScreen && !altBuffer) return;
        e.preventDefault();
        e.stopPropagation();
        sendMouseFallback(e);
      }
    });

    requestAnimationFrame(() => {
      if (fitAddon && containerEl.offsetHeight > 0) {
        try {
          fitAddon.fit();
          if (term) terminalResize(sessionId, term.cols, term.rows);
        } catch {}
      }
    });

    resizeObserver = new ResizeObserver(() => {
      if (fitAddon && active && containerEl.offsetHeight > 0) {
        try {
          fitAddon.fit();
          if (term) terminalResize(sessionId, term.cols, term.rows);
        } catch {}
      }
    });
    resizeObserver.observe(containerEl);

    hardClearTerminalState();
    requestAnimationFrame(() => {
      if (fitAddon && containerEl.offsetHeight > 0) {
        try {
          fitAddon.fit();
          if (term) terminalResize(sessionId, term.cols, term.rows);
        } catch {}
      }
    });

    const rt = (window as any).runtime;
    if (rt) {
      const stripOsc = (arr: Uint8Array): Uint8Array => {
        const out: number[] = [];
        let i = 0;
        while (i < arr.length) {
          if (arr[i] === 0x1b && i + 1 < arr.length && arr[i + 1] === 0x5d) {
            let j = i + 2;
            for (; j < arr.length; j++) {
              if (arr[j] === 0x07) { i = j + 1; break; }
              if (arr[j] === 0x1b && j + 1 < arr.length && arr[j + 1] === 0x5c) { i = j + 2; break; }
            }
            if (j >= arr.length) { out.push(arr[i]); i++; }
          } else {
            out.push(arr[i]);
            i++;
          }
        }
        return new Uint8Array(out);
      };
      const handler = (data: { sessionId: string; output: string }) => {
        if (!acceptOutput || data.sessionId !== mountSessionId || !term) return;
        try {
          const decoded = Uint8Array.from(atob(data.output), (c) => c.charCodeAt(0));
          const filtered = stripOsc(decoded);
          let plain = '';
          try {
            plain = new TextDecoder().decode(filtered);
          } catch {}
          if (plain.includes('\x1b[?1000h') || plain.includes('\x1b[?1002h') || plain.includes('\x1b[?1003h') || plain.includes('\x1b[?1006h')) {
            mouseModeEnabled = true;
          }
          if (plain.includes('\x1b[?1000l') || plain.includes('\x1b[?1002l') || plain.includes('\x1b[?1003l') || plain.includes('\x1b[?1006l')) {
            mouseModeEnabled = false;
          }
          if (plain.includes('\x1b[?1049h') || plain.includes('\x1b[?47h') || plain.includes('\x1b[?1047h')) {
            inAltScreen = true;
          }
          if (plain.includes('\x1b[?1049l') || plain.includes('\x1b[?47l') || plain.includes('\x1b[?1047l')) {
            inAltScreen = false;
          }
          if (filtered.length > 0) {
            if (!firstAcceptedChunkLogged) {
              firstAcceptedChunkLogged = true;
              let preview = '';
              try {
                preview = new TextDecoder().decode(filtered).replace(/[^\x20-\x7E\r\n\t]/g, '').slice(0, 200);
              } catch {}
              const nonEmptyLinesBeforeWrite = countNonEmptyLines();
              const linesBeforeWrite = firstNonEmptyLines(3);
              const previewHead = preview.slice(0, 48).trim();
              const previewHeadTail = previewHead.length > 1 ? previewHead.slice(1) : previewHead;
              const hasPreviewAlready =
                previewHead.length > 0 &&
                linesBeforeWrite.some((line) => {
                  const norm = line.trim();
                  return (
                    norm.includes(previewHead) ||
                    previewHead.includes(norm) ||
                    norm.includes(previewHeadTail) ||
                    preview.includes(norm)
                  );
                });
              if (nonEmptyLinesBeforeWrite > 0 && hasPreviewAlready) {
                // Guard against duplicated first frame: if buffer already contains same welcome/prompt
                // before the first write, clear once more and only then render the incoming chunk.
                hardClearTerminalState();
              }
            }
            term.write(filtered, () => term?.scrollToBottom());
          }
        } catch {
          term.write(data.output);
        }
      };
      const unsubscribe = rt.EventsOn('TerminalOutput', handler);
      eventOff = unsubscribe;
      acceptOutput = true;
    }
  });

  onDestroy(() => {
    if (fitTimer) clearTimeout(fitTimer);
    if (resizeObserver) resizeObserver.disconnect();
    if (eventOff) eventOff();
    dataDisposable?.dispose();
    selectionDisposable?.dispose();
    if (term) term.dispose();
  });

  $: if (active && fitAddon && initDone) {
    setTimeout(() => {
      if (fitAddon && containerEl && containerEl.offsetHeight > 0) {
        try { fitAddon.fit(); } catch {}
      }
      term?.focus();
      term?.scrollToBottom();
    }, 50);
  }
</script>

<div class="terminal-container" bind:this={containerEl}></div>

<style>
  .terminal-container {
    flex: 1;
    min-height: 0;
    width: 100%;
    position: relative;
    overflow: hidden;
    background: #1e1e1e;
    user-select: none;
    -webkit-user-select: none;
    box-sizing: border-box;
  }

  .terminal-container :global(canvas) {
    display: block;
  }

  .terminal-container :global([role="application"]) {
    position: absolute;
    inset: 0;
    padding: 0;
    width: 100%;
    height: 100%;
  }
</style>
