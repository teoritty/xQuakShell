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

<div class="local-terminal-view">
  {#key terminal.id}
    <Terminal {io} {active} />
  {/key}
</div>

<style>
  .local-terminal-view {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    background: var(--bg-primary);
  }
</style>
