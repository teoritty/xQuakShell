<script lang="ts">
  // The multi-input controls: what the picker is waiting for, or which terminals are receiving.
  //
  // It sits in the application's top bar, in the space that is otherwise empty, rather than taking
  // a row of its own. A row appearing and disappearing would resize every terminal under it, and
  // each resize reaches the remote end as a SIGWINCH that redraws whatever full-screen program is
  // running there. Floating it over the terminals instead covered the tiles' own tab bars.
  import { onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import { t } from '../../i18n/messages';
  import { hotkeyLabel } from '../../hotkeys/hotkeys';
  import {
    multiInput,
    terminalEntries,
    toggleMultiInputMember,
    selectAllTerminals,
    selectNoTerminals,
    confirmMultiInput,
    cancelMultiInput,
    stopMultiInput,
    toggleMultiInputPicker,
  } from '../../stores/terminalTools';
  import { terminalForDigit } from '../../terminal/terminalOrder';
  import { MIN_GROUP } from '../../terminal/multiInputState';

  export let hotkey: string;
  export let stopHotkey: string;

  $: selecting = $multiInput.mode === 'selecting';
  $: chosenCount = $multiInput.mode === 'selecting' ? $multiInput.draft.length : $multiInput.mode === 'active' ? $multiInput.members.length : 0;

  // While picking, the keyboard belongs to the picker. The listener runs in the capture phase on
  // the window so a digit never reaches a terminal that still holds focus, and only unmodified keys
  // are taken - the app's own chords, the multi-input one included, keep working.
  function onPickerKey(e: KeyboardEvent) {
    if (get(multiInput).mode !== 'selecting' || e.ctrlKey || e.altKey || e.metaKey) return;
    const id = terminalForDigit(get(terminalEntries), e.key);
    const handled = id !== null || ['Enter', 'Escape', 'a', 'A', 'n', 'N'].includes(e.key);
    if (!handled) return;
    e.preventDefault();
    e.stopPropagation();
    if (id) toggleMultiInputMember(id);
    else if (e.key === 'Enter') confirmMultiInput();
    else if (e.key === 'Escape') cancelMultiInput();
    else if (e.key === 'a' || e.key === 'A') selectAllTerminals();
    else selectNoTerminals();
  }

  window.addEventListener('keydown', onPickerKey, true);
  onDestroy(() => window.removeEventListener('keydown', onPickerKey, true));
</script>

{#if $multiInput.mode !== 'off'}
  <div class="multi-input-bar" class:selecting role="status">
    {#if selecting}
      <span class="bar-title">{$t('multiInput.pickTitle')}</span>
      <span class="bar-hint">{$t('multiInput.pickHint')}</span>
      <span class="bar-count">{$t('multiInput.selected', { count: chosenCount })}</span>
      <button class="bar-btn primary" on:click={confirmMultiInput}>
        {chosenCount >= MIN_GROUP ? $t('multiInput.start') : $t('multiInput.off')}
        <kbd>Enter</kbd>
      </button>
      <button class="bar-btn" on:click={cancelMultiInput}>{$t('common.cancel')} <kbd>Esc</kbd></button>
    {:else}
      <span class="bar-title live">⇉ {$t('multiInput.typingInto', { count: chosenCount })}</span>
      <button class="bar-btn" on:click={toggleMultiInputPicker}>
        {$t('multiInput.change')} <kbd>{hotkeyLabel(hotkey)}</kbd>
      </button>
      <button class="bar-btn danger" on:click={stopMultiInput}>{$t('multiInput.stop')} <kbd>{hotkeyLabel(stopHotkey)}</kbd></button>
    {/if}
  </div>
{/if}

<style>
  .multi-input-bar {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    max-width: 100%;
    margin: 3px 8px;
    padding: 1px 4px 1px 10px;
    border-radius: 6px;
    font-size: 12px;
    color: var(--text-primary);
    background: var(--bg-secondary);
    border: 1px solid rgba(86, 196, 108, 0.55);
    white-space: nowrap;
    overflow: hidden;
  }
  .multi-input-bar.selecting {
    border-color: rgba(229, 83, 75, 0.5);
  }
  .bar-title {
    font-weight: 600;
  }
  .bar-title.live {
    color: rgba(126, 214, 142, 0.95);
    font-weight: 500;
  }
  .bar-hint {
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .bar-count {
    color: var(--text-secondary);
  }
  .bar-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 1px 8px;
    font-size: 12px;
    flex-shrink: 0;
  }
  .bar-btn.danger {
    color: var(--danger);
  }
  kbd {
    font-family: var(--font-mono, monospace);
    font-size: 10px;
    padding: 0 4px;
    border: 1px solid var(--border-color);
    border-radius: 3px;
    background: var(--bg-tertiary);
    color: var(--text-secondary);
  }
</style>
