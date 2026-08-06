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

<!-- The state is a banner over the viewer, never a replacement for it. Nothing in the protocol
     obliges a plugin to report `ready`, so a viewer that waited for it left a plugin that simply
     opened a tab and started writing with a spinner and a discarded stream; and unmounting on a
     momentary `error` threw away the log the user had accumulated. The viewer's lifetime is the
     surface's (ADR-015 §1). -->
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
  {/if}

  {#key surface.surfaceId}
    {#if surface.kind === 'terminal'}
      <Terminal {io} {active} />
    {:else}
      <LogSurfaceView surfaceId={surface.surfaceId} title={surface.title} />
    {/if}
  {/key}
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
    padding: 6px 12px;
    color: var(--text-secondary);
    font-size: 12px;
    border-bottom: 1px solid var(--border-color);
    background: var(--bg-tertiary);
    flex-shrink: 0;
  }

  .surface-status.error {
    color: var(--danger);
  }
</style>
