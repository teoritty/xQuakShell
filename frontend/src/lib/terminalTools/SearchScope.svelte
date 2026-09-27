<script lang="ts">
  // Which terminals the search covers. One chip per open terminal, numbered as the multi-input
  // picker numbers them, so "terminal 3" means the same thing in both tools.
  import { get } from 'svelte/store';
  import { t } from '../../i18n/messages';
  import { activeTabId } from '../../stores/appState';
  import { terminalEntries } from '../../stores/terminalTools';
  import { terminalSearch, setSearchScope, toggleSearchScope } from '../../stores/terminalSearch';

  $: scope = new Set($terminalSearch.scope);
  $: allChosen = $terminalEntries.length > 0 && $terminalEntries.every((e) => scope.has(e.id));

  function current() {
    const focused = get(activeTabId);
    if ($terminalEntries.some((e) => e.id === focused)) setSearchScope([focused]);
  }
</script>

<div class="scope">
  <div class="scope-head">
    <span>{$t('search.scope')}</span>
    <span class="scope-quick">
      <button class="link" on:click={current}>{$t('search.scopeCurrent')}</button>
      <button class="link" class:on={allChosen} on:click={() => setSearchScope($terminalEntries.map((e) => e.id))}>
        {$t('search.scopeAll')}
      </button>
    </span>
  </div>
  {#if $terminalEntries.length === 0}
    <div class="scope-empty">{$t('search.noTerminals')}</div>
  {:else}
    <div class="chips">
      {#each $terminalEntries as entry (entry.id)}
        <button
          class="chip"
          class:on={scope.has(entry.id)}
          aria-pressed={scope.has(entry.id)}
          title={entry.title}
          on:click={() => toggleSearchScope(entry.id)}
        >
          <span class="chip-number">{entry.number}</span>
          <span class="chip-title">{entry.title}</span>
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .scope {
    padding: 8px 10px 0;
  }
  .scope-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    color: var(--text-secondary);
    font-size: 11px;
    margin-bottom: 4px;
  }
  .scope-quick {
    display: flex;
    gap: 8px;
  }
  .link {
    background: none;
    border: none;
    padding: 0;
    font-size: 11px;
    color: var(--accent);
    cursor: pointer;
  }
  .link.on {
    text-decoration: underline;
  }
  .scope-empty {
    color: var(--text-secondary);
    font-size: 11px;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    max-height: 84px;
    overflow-y: auto;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    max-width: 100%;
    padding: 2px 8px 2px 3px;
    font-size: 11px;
    border-radius: 10px;
    border: 1px solid var(--border-color);
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    cursor: pointer;
  }
  .chip.on {
    color: var(--text-primary);
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 22%, transparent);
  }
  .chip-number {
    min-width: 15px;
    height: 15px;
    border-radius: 8px;
    font-weight: 700;
    font-size: 10px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-hover);
  }
  .chip.on .chip-number {
    background: var(--accent);
    color: #fff;
  }
  .chip-title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 140px;
  }
</style>
