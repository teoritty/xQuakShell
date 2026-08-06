<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { Terminal } from '@xterm/xterm';
  import { FitAddon } from '@xterm/addon-fit';
  import { LigaturesAddon } from '@xterm/addon-ligatures';
  import { WebLinksAddon } from '@xterm/addon-web-links';
  import type { TerminalIO } from '../terminal/terminalIO';
  import { getSettings } from '../actions/settingsActions';
  import { takePendingTerminalOutput, registerTerminalOutputConsumer, clearPendingTerminalOutput } from '../terminal/outputBuffer';
  import { getUiScaleFactor } from './uiScale';
  import { dataHasEnter, extractCommandLine } from './terminalCommandLine';
  import { getPooledTerminal, setPooledTerminal } from './terminalPool';
  import { refitGrid, ensureInitialFit as ensureGridFit } from '../terminal/xtermGrid';
  import { defaultTerminalTheme } from '../terminal/xtermTheme';

  /**
   * Where this terminal's bytes come from and go to — an SSH session or a plugin surface
   * (ADR-015). The renderer names neither producer; everything that differs between them lives
   * behind this interface.
   */
  export let io: TerminalIO;
  export let active: boolean = false;

  let containerEl: HTMLDivElement;
  /** The element xterm is opened on; lives in the pool and moves between mounts. */
  let host: HTMLDivElement | null = null;
  let term: Terminal | null = null;
  let fitAddon: FitAddon | null = null;
  let resizeObserver: ResizeObserver | null = null;
  let eventOff: (() => void) | null = null;
  let dataDisposable: { dispose: () => void } | null = null;
  let resizeDisposable: { dispose: () => void } | null = null;
  let initDone = false;
  /** Drops live TerminalOutput until subscription is installed. */
  let acceptOutput = false;
  let unregisterOutputConsumer: (() => void) | null = null;
  const mountSessionId = io.id;
  /** Captured on Enter keydown before xterm/PTY consume the line. */
  let pendingCommandLine = '';
  let baseTerminalFontSize = 14;

  function scaledTerminalFontSize(): number {
    return Math.round(baseTerminalFontSize * getUiScaleFactor());
  }

  function applyTerminalFontSize() {
    if (!term) return;
    const next = scaledTerminalFontSize();
    if (term.options.fontSize === next) return;
    term.options.fontSize = next;
    term.refresh(0, term.rows - 1);
    scheduleRefit();
  }

  function writeBytesToTerm(bytes: Uint8Array, scroll = true) {
    if (!term || bytes.length === 0) return;
    term.write(bytes, scroll ? () => term?.scrollToBottom() : undefined);
  }

  function writeTerminalPayload(output: string, scroll = true) {
    writeBytesToTerm(decodeTerminalOutput(output), scroll);
  }

  function decodeTerminalOutput(output: string): Uint8Array {
    try {
      return Uint8Array.from(atob(output), (c) => c.charCodeAt(0));
    } catch {
      return new TextEncoder().encode(output);
    }
  }

  const onUiScaleChanged = () => applyTerminalFontSize();

  let refitRaf = 0;

  /** This component's terminal and box, bound into the shared grid helpers. */
  function refit(force = false) {
    refitGrid(term, containerEl, force);
  }

  function scheduleRefit() {
    if (refitRaf) cancelAnimationFrame(refitRaf);
    refitRaf = requestAnimationFrame(() => refit());
  }

  async function pasteFromClipboard() {
    try {
      const text = await navigator.clipboard.readText();
      if (text) term?.paste(text);
    } catch {}
  }

  onMount(async () => {
    // Reuse a live terminal from the pool when this component is remounting for a
    // session that already has one (e.g. the tab was moved to another tile). This
    // preserves the full scrollback, cursor and PTY wiring across layout changes.
    const pooled = getPooledTerminal(mountSessionId);
    if (pooled) {
      term = pooled.term;
      fitAddon = pooled.fitAddon;
      host = pooled.host;
      baseTerminalFontSize = pooled.baseFontSize;
      containerEl.appendChild(host);
      initDone = true;
    } else {
      const settings = await getSettings();
      baseTerminalFontSize = settings?.terminalFontSize ?? 14;
      const fontSize = scaledTerminalFontSize();
      const fontFamily = settings?.terminalFontFamily || 'Cascadia Code, Consolas, Courier New, monospace';
      const fontColor = settings?.terminalFontColor || '#cccccc';
      const theme = { ...defaultTerminalTheme, foreground: fontColor };

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

      // xterm is opened once on its own host element; the host (with all its DOM,
      // including scrollback) is what moves between containers across remounts.
      host = document.createElement('div');
      host.className = 'terminal-host';
      containerEl.appendChild(host);
      term.open(host);
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

      setPooledTerminal(mountSessionId, { term, fitAddon, host, baseFontSize: baseTerminalFontSize });
      initDone = true;
    }

    dataDisposable = term.onData((data) => {
      const commandLine = dataHasEnter(data) ? pendingCommandLine : '';
      pendingCommandLine = '';
      io.sendInput(data, commandLine);
    });

    // fit() updates cols/rows and fires this; keep the backend PTY in sync.
    resizeDisposable = term.onResize(({ cols, rows }) => {
      io.resize(cols, rows);
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

      if (ev.key === 'Enter' && term) {
        pendingCommandLine = extractCommandLine(term);
      }

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
    void ensureGridFit(term, containerEl);
    requestAnimationFrame(scheduleRefit);

    // Any container size change: tab show (display:none -> flex), split-pane
    // drag, or layout settling all flow through this single path.
    resizeObserver = new ResizeObserver(scheduleRefit);
    resizeObserver.observe(containerEl);

    // Safety net for WebView2/window-level changes (maximize/restore, DPI).
    window.addEventListener('resize', scheduleRefit);
    window.addEventListener('ui-scale-changed', onUiScaleChanged);

    const rt = (window as any).runtime;
    if (rt) {
      for (const chunk of takePendingTerminalOutput(mountSessionId)) {
        writeBytesToTerm(chunk);
      }

      eventOff = io.subscribe((base64) => {
        if (!acceptOutput || !term) return;
        writeTerminalPayload(base64);
      });
      acceptOutput = true;
      unregisterOutputConsumer = registerTerminalOutputConsumer(mountSessionId);
      for (const chunk of takePendingTerminalOutput(mountSessionId)) {
        writeBytesToTerm(chunk);
      }
    }
  });

  onDestroy(() => {
    if (refitRaf) cancelAnimationFrame(refitRaf);
    window.removeEventListener('resize', scheduleRefit);
    window.removeEventListener('ui-scale-changed', onUiScaleChanged);
    if (resizeObserver) resizeObserver.disconnect();
    if (eventOff) eventOff();
    if (unregisterOutputConsumer) unregisterOutputConsumer();
    clearPendingTerminalOutput(mountSessionId);
    dataDisposable?.dispose();
    resizeDisposable?.dispose();
    // Detach the live terminal so it can be re-attached on the next mount (tile
    // rearrangement). The terminal is disposed only when the session actually
    // closes, via disposeTerminal() in the session lifecycle (stores/api).
    // Guard on ownership: when a session moves between tiles the new component
    // may re-attach the shared host before this one is destroyed, so only detach
    // the host while it still sits in OUR container.
    if (host && host.parentNode === containerEl) containerEl.removeChild(host);
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

  .terminal-container :global(.terminal-host) {
    width: 100%;
    height: 100%;
  }

  .terminal-container :global(.xterm) {
    padding: 0;
  }

  .terminal-container :global(.xterm-viewport) {
    overflow-y: auto;
  }
</style>
