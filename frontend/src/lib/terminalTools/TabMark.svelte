<script lang="ts">
  // A terminal's number on its tab, while the picker is open or while it is in the group.
  //
  // A tab that is not its tile's active one has no terminal on screen to draw a number over, so the
  // tab carries it instead; the picker can reach every terminal, not only the visible ones.
  import { multiInput, terminalEntries } from '../../stores/terminalTools';
  import { isChosen } from '../../terminal/multiInputState';

  export let id: string;

  $: entry = $terminalEntries.find((e) => e.id === id);
  $: chosen = isChosen($multiInput, id);
  $: shown = !!entry && ($multiInput.mode === 'selecting' || chosen);
</script>

{#if shown && entry}
  <span class="tab-mark" class:chosen>{entry.number}</span>
{/if}

<style>
  .tab-mark {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 16px;
    height: 16px;
    padding: 0 3px;
    box-sizing: border-box;
    border-radius: 4px;
    font-size: 10px;
    font-weight: 700;
    color: #fff;
    background: var(--multi-input-off, #e5534b);
    flex-shrink: 0;
  }
  .tab-mark.chosen {
    background: var(--multi-input-on, #3fb950);
  }
</style>
