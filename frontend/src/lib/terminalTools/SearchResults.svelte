<script lang="ts">
  // The matches, grouped by terminal the way an editor groups them by file. A click - or Enter in
  // the query box - brings the match's terminal forward and selects it there.
  import { t } from '../../i18n/messages';
  import { ChevronDown, ChevronRight } from 'lucide-svelte';
  import { terminalEntries } from '../../stores/terminalTools';
  import { terminalSearch, showHit } from '../../stores/terminalSearch';
  import { flattenHits, previewOf } from '../../terminal/search/searchRun';

  let collapsed = new Set<string>();

  $: entries = new Map($terminalEntries.map((e) => [e.id, e]));
  $: activeHit = flattenHits($terminalSearch.groups)[$terminalSearch.active] ?? null;

  function toggleGroup(id: string) {
    collapsed.has(id) ? collapsed.delete(id) : collapsed.add(id);
    collapsed = collapsed;
  }

  // The current match scrolls into view as Enter walks the list.
  function keepVisible(node: HTMLElement, isActive: boolean) {
    const apply = (active: boolean) => active && node.scrollIntoView({ block: 'nearest' });
    apply(isActive);
    return { update: apply };
  }
</script>

<div class="results">
  {#each $terminalSearch.groups as group (group.terminalId)}
    {@const entry = entries.get(group.terminalId)}
    <button class="group-head" on:click={() => toggleGroup(group.terminalId)}>
      {#if collapsed.has(group.terminalId)}<ChevronRight size={13} />{:else}<ChevronDown size={13} />{/if}
      <span class="group-number">{entry?.number ?? '?'}</span>
      <span class="group-title">{entry?.title ?? group.terminalId}</span>
      <span class="group-count">{group.hits.length}</span>
    </button>
    {#if !collapsed.has(group.terminalId)}
      {#each group.hits as hit}
        {@const preview = previewOf(hit)}
        <button
          class="hit"
          class:active={hit === activeHit}
          title={$t('search.lineTitle', { line: hit.line.row + 1 })}
          use:keepVisible={hit === activeHit}
          on:click={() => showHit(hit)}
        >
          <span class="hit-line">{hit.line.row + 1}</span>
          <span class="hit-text">{preview.before}<mark>{preview.match}</mark>{preview.after}</span>
        </button>
      {/each}
    {/if}
  {/each}
</div>

<style>
  .results {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding-bottom: 8px;
  }
  button {
    display: flex;
    align-items: center;
    width: 100%;
    border: none;
    background: transparent;
    color: var(--text-primary);
    text-align: left;
    cursor: pointer;
    font-size: 12px;
  }
  .group-head {
    gap: 5px;
    padding: 3px 8px;
    font-weight: 600;
  }
  .group-head:hover,
  .hit:hover {
    background: var(--bg-hover);
  }
  .group-number {
    min-width: 16px;
    height: 16px;
    border-radius: 4px;
    font-size: 10px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-tertiary);
    border: 1px solid var(--border-color);
  }
  .group-title {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .group-count {
    font-size: 10px;
    padding: 0 6px;
    border-radius: 8px;
    background: var(--bg-tertiary);
    color: var(--text-secondary);
  }
  .hit {
    gap: 8px;
    padding: 2px 8px 2px 26px;
    font-family: var(--font-mono, monospace);
    font-size: 11.5px;
  }
  .hit.active {
    background: color-mix(in srgb, var(--accent) 30%, transparent);
    outline: 1px solid var(--accent);
    outline-offset: -1px;
  }
  .hit-line {
    flex-shrink: 0;
    min-width: 32px;
    text-align: right;
    color: var(--text-secondary);
  }
  .hit-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: pre;
  }
  mark {
    background: #a36a0a;
    color: #fff;
    border-radius: 2px;
  }
</style>
