<script lang="ts">
  // The log surface viewer (ADR-015 §1): follow, search, stdout/stderr distinction, export.
  //
  // It consumes the same output buffer the terminal path uses, keyed by surfaceId, so bytes that
  // arrived before this component mounted are not lost — the start of a log is exactly the part a
  // user opened it for.
  import { onMount, onDestroy, tick } from 'svelte';
  import { LogBuffer, type LogLine } from '../logSurface/buffer';
  import { appendMatches, indexOfSeq, stepMatch } from '../logSurface/search';
  import { computeLogWindow, scrollTopForLine, type LogWindow } from '../logSurface/window';
  import { exportLines, exportFileName } from '../logSurface/export';
  import {
    takePendingTerminalOutput,
    registerTerminalOutputConsumer,
    decodeTerminalOutput,
  } from '../terminal/outputBuffer';
  import { Search, Download, ArrowDown, ChevronUp, ChevronDown } from 'lucide-svelte';
  import { subscribeByIdRaw } from '../terminal/terminalIO';

  export let surfaceId: string;
  export let title: string = '';

  /** Row height in pixels. Fixed, and the reason the rows do not wrap — see the .lines style. */
  const ROW_HEIGHT = 18;

  const buffer = new LogBuffer();

  let lines: readonly LogLine[] = [];
  let truncated = false;
  let follow = true;
  let query = '';
  let caseSensitive = false;
  /** The seq of every match, not its position: positions move when old lines are evicted. */
  let hits: number[] = [];
  let hitIndex = -1;
  /** The newest line already searched, so a repaint only scans what arrived since. */
  let searchedThroughSeq = -1;
  let listEl: HTMLDivElement;
  let viewportHeight = 0;
  let scrollTop = 0;
  let win: LogWindow = { first: 0, count: 0, topPad: 0, totalHeight: 0 };
  let unsubscribe: (() => void) | null = null;
  let unregisterConsumer: (() => void) | null = null;
  let frame = 0;

  /**
   * Schedules a repaint at most once per animation frame.
   *
   * Output arrives batched from the host, but several batches can still land inside one frame, and
   * every repaint walks the visible window. Coalescing here keeps that work bounded by the display
   * rather than by how talkative the producer is.
   */
  function scheduleRefresh() {
    if (frame !== 0) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      refresh();
    });
  }

  function refresh() {
    // The array itself, not a copy: only a window of it is rendered, and copying up to 200 000
    // entries per repaint is the cost this viewer exists to avoid.
    lines = buffer.snapshot();
    truncated = buffer.truncated();
    extendHits();
    if (follow) void scrollToBottom();
    else recomputeWindow();
  }

  async function scrollToBottom() {
    await tick();
    if (!listEl) return;
    listEl.scrollTop = listEl.scrollHeight;
    // Reading back rather than assuming: a clamped scrollTop is what the window must be computed
    // from, or the last rows are rendered one screen away from where they are drawn.
    scrollTop = listEl.scrollTop;
    recomputeWindow();
  }

  function recomputeWindow() {
    win = computeLogWindow(lines.length, scrollTop, viewportHeight, ROW_HEIGHT);
  }

  /** Adds matches from lines that arrived since the last pass. */
  function extendHits() {
    if (query === '') {
      hits = [];
      hitIndex = -1;
      searchedThroughSeq = lines.length > 0 ? lines[lines.length - 1].seq : -1;
      return;
    }
    hits = appendMatches(hits, lines, searchedThroughSeq, query, caseSensitive);
    searchedThroughSeq = lines.length > 0 ? lines[lines.length - 1].seq : -1;
    if (hits.length === 0) hitIndex = -1;
    else if (hitIndex < 0 || hitIndex >= hits.length) hitIndex = 0;
  }

  /** Starts the search over. Called when the query or its case sensitivity changes. */
  function restartSearch() {
    hits = [];
    hitIndex = -1;
    searchedThroughSeq = -1;
    extendHits();
  }

  async function jumpTo(index: number) {
    hitIndex = index;
    if (hitIndex < 0) return;
    // Jumping detaches from the bottom: continuing to follow would yank the view away from the
    // match the user just asked to see.
    follow = false;
    const row = indexOfSeq(lines, hits[hitIndex]);
    if (row < 0 || !listEl) return;
    // Scrolled by arithmetic rather than scrollIntoView: the matching row may be outside the
    // rendered window, in which case there is no element to scroll to yet.
    listEl.scrollTop = scrollTopForLine(row, lines.length, viewportHeight, ROW_HEIGHT);
    scrollTop = listEl.scrollTop;
    recomputeWindow();
    await tick();
  }

  function onScroll() {
    if (!listEl) return;
    scrollTop = listEl.scrollTop;
    viewportHeight = listEl.clientHeight;
    const atBottom = listEl.scrollHeight - listEl.scrollTop - listEl.clientHeight < ROW_HEIGHT;
    // Follow re-arms by itself when the user scrolls back down, so getting back to live output
    // does not need a button nobody would look for.
    follow = atBottom;
    recomputeWindow();
  }

  function download() {
    const text = exportLines(lines, true);
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = exportFileName(title || surfaceId);
    a.click();
    URL.revokeObjectURL(url);
  }

  onMount(() => {
    for (const chunk of takePendingTerminalOutput(surfaceId)) {
      buffer.append(chunk, 'stdout');
    }
    unregisterConsumer = registerTerminalOutputConsumer(surfaceId);

    // The stream field decides the colour, so it is read from the payload rather than through
    // subscribeById's data-only callback: this is the one consumer for which stdout and stderr are
    // not the same bytes.
    unsubscribe = subscribeByIdRaw<{ data?: string; stream?: string }>(
      'PluginSurfaceOutput',
      'surfaceId',
      surfaceId,
      (payload) => {
        buffer.append(
          decodeTerminalOutput(payload.data ?? ''),
          payload.stream === 'stderr' ? 'stderr' : 'stdout'
        );
        scheduleRefresh();
      }
    );
    if (listEl) viewportHeight = listEl.clientHeight;
    refresh();
  });

  onDestroy(() => {
    unsubscribe?.();
    unregisterConsumer?.();
    if (frame !== 0) cancelAnimationFrame(frame);
    buffer.flush();
  });

  // A changed query invalidates every hit, so this is the one path that scans the whole buffer.
  $: query, caseSensitive, restartSearch();
  $: visible = lines.slice(win.first, win.first + win.count);
</script>

<div class="log-surface">
  <div class="toolbar">
    <div class="search">
      <Search size={12} />
      <input bind:value={query} placeholder="Search" spellcheck="false" />
      <button
        class="ghost toggle"
        class:on={caseSensitive}
        title="Match case"
        on:click={() => (caseSensitive = !caseSensitive)}>Aa</button
      >
      <span class="count">
        {#if query === ''}
          &nbsp;
        {:else}
          {hits.length === 0 ? 'no matches' : `${hitIndex + 1}/${hits.length}`}
        {/if}
      </span>
      <button
        class="ghost"
        title="Previous match"
        disabled={hits.length === 0}
        on:click={() => jumpTo(stepMatch(hits, hitIndex, -1))}><ChevronUp size={12} /></button
      >
      <button
        class="ghost"
        title="Next match"
        disabled={hits.length === 0}
        on:click={() => jumpTo(stepMatch(hits, hitIndex, 1))}><ChevronDown size={12} /></button
      >
    </div>
    <div class="actions">
      <button
        class="ghost toggle"
        class:on={follow}
        title="Follow output"
        on:click={() => {
          follow = !follow;
          if (follow) void scrollToBottom();
        }}><ArrowDown size={12} /></button
      >
      <button class="ghost" title="Save to file" on:click={download}><Download size={12} /></button>
    </div>
  </div>

  {#if truncated}
    <div class="truncated">Older lines were dropped — this log exceeds the buffer limit.</div>
  {/if}

  <!-- Only the visible rows exist in the DOM; the spacer and the total height are what keep the
       scrollbar describing the whole log (ADR-015 §1 bounds it at 200 000 lines). -->
  <div class="lines" bind:this={listEl} on:scroll={onScroll}>
    <div class="spacer" style="height: {win.totalHeight}px">
      <div class="rows" style="transform: translateY({win.topPad}px)">
        {#each visible as line (line.seq)}
          <div
            class="line"
            class:stderr={line.stream === 'stderr'}
            class:hit={hitIndex >= 0 && hits[hitIndex] === line.seq}
          >
            {line.text}
          </div>
        {/each}
      </div>
    </div>
  </div>
</div>

<style>
  .log-surface {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    background: var(--bg-primary);
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 4px 8px;
    border-bottom: 1px solid var(--border-color);
    flex-shrink: 0;
  }

  .search {
    display: flex;
    align-items: center;
    gap: 4px;
    color: var(--text-secondary);
  }

  .search input {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    color: var(--text-primary);
    border-radius: 3px;
    padding: 2px 6px;
    font-size: 12px;
    width: 180px;
  }

  .count {
    font-size: 11px;
    color: var(--text-secondary);
    min-width: 64px;
  }

  .toggle.on {
    color: var(--accent, #4b8bbf);
  }

  .truncated {
    padding: 3px 8px;
    font-size: 11px;
    color: var(--text-secondary);
    background: var(--bg-tertiary);
    border-bottom: 1px solid var(--border-color);
  }

  .lines {
    flex: 1;
    overflow: auto;
    padding: 4px 0;
    font-family: var(--font-mono, monospace);
    font-size: 12px;
  }

  .spacer {
    position: relative;
  }

  /* width: max-content so a long line extends the scrollable area instead of being clipped;
     min-width keeps short logs filling the pane so row backgrounds span it. */
  .rows {
    position: absolute;
    top: 0;
    left: 0;
    width: max-content;
    min-width: 100%;
  }

  /* Fixed height and no wrapping: the viewer renders a window of the buffer, and knowing which
     line sits at a given scroll offset is only possible when every row is the same height. A long
     line scrolls sideways instead of growing downwards. */
  .line {
    height: 18px;
    line-height: 18px;
    padding: 0 8px;
    white-space: pre;
  }

  .line.stderr {
    color: var(--danger, #f44747);
  }

  .line.hit {
    background: var(--bg-tertiary);
  }
</style>
