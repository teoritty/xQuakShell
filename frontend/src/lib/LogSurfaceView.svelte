<script lang="ts">
  // The log surface viewer (ADR-015 §1): follow, search, stdout/stderr distinction, export.
  //
  // It consumes the same output buffer the terminal path uses, keyed by surfaceId, so bytes that
  // arrived before this component mounted are not lost — the start of a log is exactly the part a
  // user opened it for.
  import { onMount, onDestroy, tick } from 'svelte';
  import { LogBuffer, type LogLine } from '../logSurface/buffer';
  import { searchLines, stepMatch } from '../logSurface/search';
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

  const buffer = new LogBuffer();

  let lines: readonly LogLine[] = [];
  let truncated = false;
  let follow = true;
  let query = '';
  let caseSensitive = false;
  let hits: number[] = [];
  let hitIndex = -1;
  let listEl: HTMLDivElement;
  let unsubscribe: (() => void) | null = null;
  let unregisterConsumer: (() => void) | null = null;

  function refresh() {
    lines = [...buffer.snapshot()];
    truncated = buffer.truncated();
    recomputeHits();
    if (follow) void scrollToBottom();
  }

  async function scrollToBottom() {
    await tick();
    if (listEl) listEl.scrollTop = listEl.scrollHeight;
  }

  function recomputeHits() {
    hits = searchLines(lines, query, caseSensitive);
    if (hits.length === 0) {
      hitIndex = -1;
    } else if (hitIndex < 0 || hitIndex >= hits.length) {
      hitIndex = 0;
    }
  }

  async function jumpTo(index: number) {
    hitIndex = index;
    if (hitIndex < 0) return;
    // Jumping detaches from the bottom: continuing to follow would yank the view away from the
    // match the user just asked to see.
    follow = false;
    await tick();
    const row = listEl?.querySelector(`[data-row="${hits[hitIndex]}"]`);
    row?.scrollIntoView({ block: 'center' });
  }

  function onScroll() {
    if (!listEl) return;
    const atBottom = listEl.scrollHeight - listEl.scrollTop - listEl.clientHeight < 4;
    // Follow re-arms by itself when the user scrolls back down, so getting back to live output
    // does not need a button nobody would look for.
    follow = atBottom;
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
        refresh();
      }
    );
    refresh();
  });

  onDestroy(() => {
    unsubscribe?.();
    unregisterConsumer?.();
    buffer.flush();
  });

  $: query, caseSensitive, recomputeHits();
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

  <div class="lines" bind:this={listEl} on:scroll={onScroll}>
    {#each lines as line, i (line.seq)}
      <div
        class="line"
        class:stderr={line.stream === 'stderr'}
        class:hit={hitIndex >= 0 && hits[hitIndex] === i}
        data-row={i}
      >
        {line.text}
      </div>
    {/each}
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
    padding: 4px 8px;
    font-family: var(--font-mono, monospace);
    font-size: 12px;
    line-height: 1.45;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .line.stderr {
    color: var(--danger, #f44747);
  }

  .line.hit {
    background: var(--bg-tertiary);
  }
</style>
