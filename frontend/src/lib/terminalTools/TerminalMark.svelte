<script lang="ts">
  // The multi-input state of one terminal, drawn over it.
  //
  // In the picker every terminal is outlined and carries its number: red and dashed until chosen,
  // green and solid once it is. The whole terminal is the click target, because the terminal is
  // what is being chosen. While a group is active its members keep the green outline, so which
  // terminals a keystroke will reach is always on screen, not only in the bar that started it.
  import { t } from '../../i18n/messages';
  import { multiInput, terminalEntries, toggleMultiInputMember } from '../../stores/terminalTools';
  import { isChosen } from '../../terminal/multiInputState';

  export let id: string;

  $: entry = $terminalEntries.find((e) => e.id === id);
  $: selecting = $multiInput.mode === 'selecting';
  $: chosen = isChosen($multiInput, id);
  $: shown = !!entry && (selecting || chosen);
</script>

{#if shown && entry}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    class="terminal-mark"
    class:selecting
    class:chosen
    role="checkbox"
    aria-checked={chosen}
    aria-label={$t('multiInput.markLabel', { number: entry.number, title: entry.title })}
    tabindex="-1"
    on:mousedown|preventDefault|stopPropagation
    on:click|stopPropagation={() => selecting && toggleMultiInputMember(id)}
  >
    {#if selecting}
      <span class="mark-number">{entry.number}</span>
    {:else}
      <span class="mark-live">⇉ {entry.number}</span>
    {/if}
  </div>
{/if}

<style>
  /* Restrained on purpose: the outline marks state, it is not an alarm. Thin lines, muted colour,
     numbers drawn as outlined labels rather than solid blocks, and the terminal underneath stays
     readable while the picker is open. */
  .terminal-mark {
    --mark-off: rgba(229, 83, 75, 0.55);
    --mark-on: rgba(86, 196, 108, 0.7);
    position: absolute;
    inset: 0;
    z-index: 20;
    pointer-events: none;
    box-sizing: border-box;
    border: 1px solid var(--mark-on);
  }

  .terminal-mark.selecting {
    pointer-events: auto;
    cursor: pointer;
    border: 1px dashed var(--mark-off);
    background: rgba(0, 0, 0, 0.18);
    transition: background 0.12s, border-color 0.12s;
  }

  .terminal-mark.selecting:hover {
    background: rgba(0, 0, 0, 0.1);
  }

  .terminal-mark.selecting.chosen {
    border: 1px solid var(--mark-on);
    background: rgba(86, 196, 108, 0.05);
  }

  .mark-number {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    min-width: 40px;
    height: 40px;
    padding: 0 10px;
    box-sizing: border-box;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 20px;
    font-weight: 600;
    color: rgba(240, 130, 122, 0.95);
    background: rgba(24, 24, 24, 0.82);
    border: 1px solid var(--mark-off);
    box-shadow: 0 2px 10px rgba(0, 0, 0, 0.35);
  }

  .chosen .mark-number {
    color: rgba(126, 214, 142, 0.95);
    border-color: var(--mark-on);
  }

  .mark-live {
    position: absolute;
    top: 5px;
    right: 18px;
    padding: 0 5px;
    border-radius: 3px;
    font-size: 10px;
    font-weight: 500;
    line-height: 16px;
    color: rgba(126, 214, 142, 0.9);
    background: rgba(24, 24, 24, 0.75);
    border: 1px solid rgba(86, 196, 108, 0.45);
  }
</style>
