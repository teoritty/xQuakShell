<!-- frontend/src/lib/tiles/TileDropOverlay.svelte -->
<script lang="ts">
  import type { Zone } from './types';
  export let zone: Zone | null;
</script>

{#if zone}
  <div class="drop-overlay {zone}" aria-hidden="true"></div>
{/if}

<style>
  .drop-overlay {
    position: absolute;
    z-index: 10;
    pointer-events: none;
    background: color-mix(in srgb, var(--accent) 30%, transparent);
    border: 1px solid var(--accent);
    transition: all 0.08s ease;
  }
  .center { inset: 0; }
  .left { left: 0; top: 0; width: 50%; height: 100%; }
  .right { right: 0; top: 0; width: 50%; height: 100%; }
  .top { left: 0; top: 0; width: 100%; height: 50%; }
  .bottom { left: 0; bottom: 0; width: 100%; height: 50%; }
</style>
