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
  /* An outlined label rather than a filled chip, so the tab bar does not light up. */
  .tab-mark {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 15px;
    height: 15px;
    padding: 0 3px;
    box-sizing: border-box;
    border-radius: 3px;
    font-size: 10px;
    font-weight: 600;
    color: rgba(240, 130, 122, 0.9);
    border: 1px solid rgba(229, 83, 75, 0.5);
    flex-shrink: 0;
  }
  .tab-mark.chosen {
    color: rgba(126, 214, 142, 0.9);
    border-color: rgba(86, 196, 108, 0.6);
  }
</style>
