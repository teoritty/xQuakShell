<script lang="ts">
  // Search across terminals, in the sidebar where the connection tree was: the query and its
  // options on top, the terminals to search under them, and the matches filling the rest.
  //
  // The query box keeps the keyboard. Enter and Shift+Enter walk the matches, bringing each one's
  // terminal forward, and Escape hands the keyboard back to the terminal - the loop an editor's find
  // box has taught everyone.
  import { tick } from 'svelte';
  import { t } from '../../i18n/messages';
  import { X, CaseSensitive, WholeWord, Regex } from 'lucide-svelte';
  import {
    terminalSearch,
    setSearchQuery,
    toggleSearchOption,
    closeTerminalSearch,
    stepHit,
  } from '../../stores/terminalSearch';
  import type { SearchOptions } from '../../terminal/search/matcher';
  import SearchScope from './SearchScope.svelte';
  import SearchResults from './SearchResults.svelte';

  let input: HTMLInputElement;

  const toggles: { option: keyof SearchOptions; icon: typeof CaseSensitive; label: string }[] = [
    { option: 'caseSensitive', icon: CaseSensitive, label: 'search.matchCase' },
    { option: 'wholeWord', icon: WholeWord, label: 'search.wholeWord' },
    { option: 'regex', icon: Regex, label: 'search.regex' },
  ];

  // Every press of the search hotkey lands the cursor in the box with the query selected, so typing
  // replaces it - including when the panel was already open.
  let seenFocusRequest = -1;
  $: if ($terminalSearch.focusRequest !== seenFocusRequest) {
    seenFocusRequest = $terminalSearch.focusRequest;
    void tick().then(() => input?.select());
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      closeTerminalSearch();
    } else if (e.key === 'Enter' || e.key === 'F3') {
      e.preventDefault();
      stepHit(e.shiftKey ? -1 : 1);
    }
  }

  $: s = $terminalSearch;
</script>

<div class="search-panel" role="search">
  <div class="panel-header">
    <span class="panel-title">{$t('search.title')}</span>
    <button class="icon-btn" title={$t('search.close')} on:click={closeTerminalSearch}><X size={14} /></button>
  </div>

  <div class="query-row" class:invalid={!!s.error}>
    <input
      bind:this={input}
      type="text"
      spellcheck="false"
      placeholder={$t('search.placeholder')}
      value={s.query}
      on:input={(e) => setSearchQuery(e.currentTarget.value)}
      on:keydown={onKey}
    />
    {#each toggles as toggle (toggle.option)}
      <button
        class="opt-btn"
        class:on={s.options[toggle.option]}
        aria-pressed={s.options[toggle.option]}
        title={$t(toggle.label)}
        on:click={() => toggleSearchOption(toggle.option)}
      >
        <svelte:component this={toggle.icon} size={15} />
      </button>
    {/each}
  </div>
  {#if s.error}
    <div class="query-error">{s.error}</div>
  {/if}

  <SearchScope />

  <div class="summary">
    {#if !s.query}
      {$t('search.hintKeys')}
    {:else if s.total === 0 && !s.error}
      {$t('search.noResults')}
    {:else if s.total > 0}
      {$t('search.summary', { count: s.total })}
      {#if s.truncated}<span class="truncated">{$t('search.truncated', { count: s.total })}</span>{/if}
    {/if}
  </div>

  <SearchResults />
</div>

<style>
  .search-panel {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    font-size: 12px;
  }
  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 10px 6px 12px;
  }
  .panel-title {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.5px;
    text-transform: uppercase;
    color: var(--text-secondary);
  }
  .icon-btn {
    display: inline-flex;
    padding: 2px;
    background: transparent;
    border: none;
    color: var(--text-secondary);
    cursor: pointer;
  }
  .icon-btn:hover {
    color: var(--text-primary);
  }
  .query-row {
    display: flex;
    align-items: center;
    gap: 2px;
    margin: 0 10px;
    padding: 2px 4px 2px 0;
    background: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: 3px;
  }
  .query-row:focus-within {
    border-color: var(--accent);
  }
  .query-row.invalid {
    border-color: var(--danger);
  }
  .query-row input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    padding: 4px 6px;
    color: var(--text-primary);
  }
  .opt-btn {
    display: inline-flex;
    padding: 2px;
    border: 1px solid transparent;
    border-radius: 3px;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
  }
  .opt-btn.on {
    color: var(--text-bright, #fff);
    background: color-mix(in srgb, var(--accent) 35%, transparent);
    border-color: var(--accent);
  }
  .query-error {
    margin: 4px 10px 0;
    color: var(--danger);
    font-size: 11px;
  }
  .summary {
    padding: 6px 12px;
    color: var(--text-secondary);
    font-size: 11px;
  }
  .truncated {
    display: block;
    color: var(--warning, #d29922);
  }
</style>
