<script lang="ts">
  // One plugin-owned tab (ADR-015). Dispatches on kind and owns nothing else: a terminal surface
  // is the existing renderer over a different stream, and a log surface is its own viewer because
  // search, stdout/stderr and export are not things a terminal emulator can offer.
  import Terminal from './Terminal.svelte';
  import LogSurfaceView from './LogSurfaceView.svelte';
  import { surfaceTerminalIO } from '../terminal/surfaceTerminalIO';
  import type { Surface } from '../stores/surfaceState';
  import { Loader2, XCircle } from 'lucide-svelte';

  export let surface: Surface;
  export let active: boolean = false;

  $: io = surfaceTerminalIO(surface.surfaceId);
</script>

<div class="surface-view">
  {#if surface.state === 'error'}
    <div class="surface-status error">
      <XCircle size={16} />
      <span>{surface.errorMessage || 'The plugin reported an error'}</span>
    </div>
  {:else if surface.state === 'connecting'}
    <div class="surface-status">
      <Loader2 size={16} />
      <span>Starting…</span>
    </div>
  {:else if surface.kind === 'terminal'}
    {#key surface.surfaceId}
      <Terminal {io} {active} />
    {/key}
  {:else}
    {#key surface.surfaceId}
      <LogSurfaceView surfaceId={surface.surfaceId} title={surface.title} />
    {/key}
  {/if}
</div>

<style>
  .surface-view {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    background: var(--bg-primary);
  }

  .surface-status {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 12px;
    color: var(--text-secondary);
    font-size: 13px;
  }

  .surface-status.error {
    color: var(--danger);
  }
</style>
