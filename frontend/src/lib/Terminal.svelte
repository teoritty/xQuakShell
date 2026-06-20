<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { Terminal } from '@xterm/xterm';
  import { FitAddon } from '@xterm/addon-fit';
  import { LigaturesAddon } from '@xterm/addon-ligatures';
  import { WebLinksAddon } from '@xterm/addon-web-links';
  import { sendTerminalInput, terminalResize, getSettings } from '../stores/api';

  export let sessionId: string;
  export let active: boolean = false;

  let containerEl: HTMLDivElement;
  let term: Terminal | null = null;
  let fitAddon: FitAddon | null = null;
  let resizeObserver: ResizeObserver | null = null;
  let eventOff: (() => void) | null = null;
  let dataDisposable: { dispose: () => void } | null = null;
  let resizeDisposable: { dispose: () => void } | null = null;
  let initDone = false;
  /** Drops TerminalOutput until subscription is installed (avoids stale lines). */
  let acceptOutput = false;
  const mountSessionId = sessionId;

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

  let refitRaf = 0;

  /** Scrollbar gutter xterm reserves when scrollback is enabled (matches FitAddon). */
  const SCROLLBAR_GUTTER = 14;

  /**
   * Measure cols/rows from the container's painted pixel box.
   *
   * FitAddon.proposeDimensions() reads getComputedStyle(parent).height, which in
   * our flex layout (WebView2) often reports ~40% of the real height on first
   * paint — terminal grid stays ~80×24, black bar below. getBoundingClientRect()
   * reflects the actual allocated flex area.
   */
  function measureGrid(): { cols: number; rows: number } | null {
    if (!term || !containerEl || !term.element) return null;
    const rect = containerEl.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return null;

    const cell = (term as any)._core?._renderService?.dimensions?.css?.cell;
    if (!cell?.width || !cell?.height) return null;

    const xtermStyle = window.getComputedStyle(term.element);
    const padX =
      (parseFloat(xtermStyle.paddingLeft) || 0) +
      (parseFloat(xtermStyle.paddingRight) || 0);
    const padY =
      (parseFloat(xtermStyle.paddingTop) || 0) +
      (parseFloat(xtermStyle.paddingBottom) || 0);
    const gutter = term.options.scrollback === 0 ? 0 : SCROLLBAR_GUTTER;

    const cols = Math.max(2, Math.floor((rect.width - padX - gutter) / cell.width));
    const rows = Math.max(1, Math.floor((rect.height - padY) / cell.height));
    if (!isFinite(cols) || !isFinite(rows)) return null;
    return { cols, rows };
  }

  /**
   * Recompute the terminal grid to match its container. Coalesced via rAF so
   * bursts of ResizeObserver/window events collapse into one fit per frame.
   * term.resize fires onResize which pushes cols/rows to the PTY.
   */
  function refit(force = false) {
    if (!term || !containerEl) return;
    if (containerEl.offsetWidth <= 0 || containerEl.offsetHeight <= 0) return;
    try {
      const dims = measureGrid();
      if (!dims) return;
      if (!force && dims.cols === term.cols && dims.rows === term.rows) return;
      term.resize(dims.cols, dims.rows);
    } catch {}
  }

  /** Keep refitting until the grid catches up with the painted container. */
  async function ensureInitialFit() {
    let stable = 0;
    let lastRows = 0;
    for (let i = 0; i < 90; i++) {
      await new Promise<void>((r) => requestAnimationFrame(() => r()));
      if (!term || !containerEl) return;
      const rect = containerEl.getBoundingClientRect();
      if (rect.height <= 0) continue;
      const dims = measureGrid();
      if (!dims) continue;
      if (dims.cols !== term.cols || dims.rows !== term.rows) {
        term.resize(dims.cols, dims.rows);
        stable = 0;
        lastRows = dims.rows;
        continue;
      }
      if (dims.rows === lastRows && dims.rows > 0) stable++;
      else lastRows = dims.rows;
      // Two consecutive matching frames with a sensible row count → layout settled.
      if (stable >= 2 && dims.rows >= 10) break;
    }
  }

  function scheduleRefit() {
    if (refitRaf) cancelAnimationFrame(refitRaf);
    refitRaf = requestAnimationFrame(refit);
  }

  async function pasteFromClipboard() {
    try {
      const text = await navigator.clipboard.readText();
      if (text) term?.paste(text);
    } catch {}
  }

  onMount(async () => {
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
      scrollback: 5000,
      convertEol: false,
      allowProposedApi: true,
    });

    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);

    // Load the real font before open() so xterm measures correct glyph metrics.
    try {
      await (document as any).fonts?.load?.(`${fontSize}px "Cascadia Code"`);
    } catch {}
    try {
      await (document as any).fonts?.ready;
    } catch {}

    term.open(containerEl);
    await tick();

    // NOTE: we intentionally do NOT use @xterm/addon-webgl. In WebView2 at
    // devicePixelRatio > 1 the WebGL renderer paints its canvas at the wrong
    // scale (terminal visually fills only a fraction of the container while the
    // grid size is correct), and only a window resize forces it to recover.
    // The default DOM renderer sizes correctly under HiDPI and also renders
    // ligatures more reliably.

    // Programming ligatures (fallback set in non-Node environments like WebView2).
    try { term.loadAddon(new LigaturesAddon()); } catch {}

    // Clickable URLs.
    try { term.loadAddon(new WebLinksAddon()); } catch {}

    initDone = true;

    dataDisposable = term.onData((data) => {
      sendTerminalInput(sessionId, data);
    });

    // fit() updates cols/rows and fires this; keep the backend PTY in sync.
    resizeDisposable = term.onResize(({ cols, rows }) => {
      terminalResize(sessionId, cols, rows);
    });

    // Right-click behaves like a classic console: copy a current selection, or
    // paste when there is nothing selected.
    containerEl.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      e.stopPropagation();
      if (term?.hasSelection()) {
        const sel = term.getSelection();
        if (sel) navigator.clipboard.writeText(sel).catch(() => {});
        term.clearSelection();
      } else {
        pasteFromClipboard();
      }
    }, true);

    // Ctrl+V / Shift+Insert paste. In xterm.js, returning false from the custom
    // key handler PREVENTS the terminal from processing the key (inverse of the
    // ghostty-web convention), so we consume the paste shortcut here.
    term.attachCustomKeyEventHandler((ev: KeyboardEvent) => {
      if (ev.type !== 'keydown') return true;
      const isPaste =
        ((ev.ctrlKey || ev.metaKey) && !ev.shiftKey && !ev.altKey && ev.code === 'KeyV') ||
        (ev.shiftKey && !ev.ctrlKey && !ev.altKey && (ev.code === 'Insert' || ev.key === 'Insert'));
      if (isPaste) {
        ev.preventDefault();
        pasteFromClipboard();
        return false;
      }
      return true;
    });

    // First fit: retry until flex layout reports the real container height.
    void ensureInitialFit();
    requestAnimationFrame(scheduleRefit);

    // Any container size change: tab show (display:none -> flex), split-pane
    // drag, or layout settling all flow through this single path.
    resizeObserver = new ResizeObserver(scheduleRefit);
    resizeObserver.observe(containerEl);

    // Safety net for WebView2/window-level changes (maximize/restore, DPI).
    window.addEventListener('resize', scheduleRefit);

    const rt = (window as any).runtime;
    if (rt) {
      const handler = (data: { sessionId: string; output: string }) => {
        if (!acceptOutput || data.sessionId !== mountSessionId || !term) return;
        try {
          const bytes = Uint8Array.from(atob(data.output), (c) => c.charCodeAt(0));
          term.write(bytes, () => term?.scrollToBottom());
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
    if (refitRaf) cancelAnimationFrame(refitRaf);
    window.removeEventListener('resize', scheduleRefit);
    if (resizeObserver) resizeObserver.disconnect();
    if (eventOff) eventOff();
    dataDisposable?.dispose();
    resizeDisposable?.dispose();
    if (term) term.dispose();
  });

  $: if (active && term && initDone) {
    scheduleRefit();
    term.focus();
    term.scrollToBottom();
  }
</script>

<div class="terminal-container" bind:this={containerEl}></div>

<style>
  .terminal-container {
    flex: 1 1 0;
    min-height: 0;
    width: 100%;
    position: relative;
    overflow: hidden;
    background: #1e1e1e;
    box-sizing: border-box;
  }

  .terminal-container :global(.xterm) {
    padding: 0;
  }

  .terminal-container :global(.xterm-viewport) {
    overflow-y: auto;
  }
</style>
