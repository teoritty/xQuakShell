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
  .terminal-mark {
    position: absolute;
    inset: 0;
    z-index: 20;
    pointer-events: none;
    box-sizing: border-box;
    border: 2px solid var(--multi-input-on, #3fb950);
  }

  .terminal-mark.selecting {
    pointer-events: auto;
    cursor: pointer;
    border: 3px dashed var(--multi-input-off, #e5534b);
    background: rgba(0, 0, 0, 0.35);
  }

  .terminal-mark.selecting.chosen {
    border: 3px solid var(--multi-input-on, #3fb950);
    background: rgba(63, 185, 80, 0.12);
  }

  .mark-number {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    min-width: 64px;
    height: 64px;
    padding: 0 12px;
    box-sizing: border-box;
    border-radius: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 36px;
    font-weight: 700;
    color: #fff;
    background: var(--multi-input-off, #e5534b);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.5);
  }

  .chosen .mark-number {
    background: var(--multi-input-on, #3fb950);
  }

  .mark-live {
    position: absolute;
    top: 4px;
    right: 18px;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    color: #fff;
    background: var(--multi-input-on, #3fb950);
  }
</style>
