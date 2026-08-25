<script lang="ts">
  // One local shell tab. Thinner than SessionView and SurfaceView because there is genuinely less
  // to it: no file panels, no split resizing, and no state banner - a local shell has no
  // connecting phase to report and no remote end that can refuse.
  import Terminal from './Terminal.svelte';
  import { localTerminalIO } from '../terminal/localTerminalIO';
  import type { LocalTerminal } from '../stores/localTerminalState';

  export let terminal: LocalTerminal;
  export let active: boolean = false;

  $: io = localTerminalIO(terminal.id);
</script>

<div class="local-terminal-view" class:visible={active}>
  {#key terminal.id}
    <Terminal {io} {active} />
  {/key}
</div>

<style>
  /* Hidden by default, shown only when this is the tile's active tab. TileGroup renders every
     tab of a tile at once and unmounts none of them, so a view that is always displayed shares
     the tile with its siblings and the result reads as a broken split. */
  .local-terminal-view {
    display: none;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    background: var(--bg-primary);
  }

  .local-terminal-view.visible {
    display: flex;
  }
</style>
