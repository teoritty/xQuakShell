<!-- frontend/src/lib/tiles/TileDropOverlay.svelte -->
<script lang="ts">
  import type { Zone } from './types';

  export let zone: Zone | null;
  export let intent: 'split' | 'reorient' | 'swap' | null = null;

  // Only a `split` shows the half-tile slice where the new tile will appear.
  // reorient / swap fill the whole tile (no confusing "quarter" preview):
  // reorient shows a large arrow pointing the way the layout will flip, swap
  // shows a swap glyph. (Merge is drawn on the tab bar by TileGroup, not here.)
  $: half = intent === 'split' && zone && zone !== 'center';
  $: glyph =
    intent === 'reorient' && zone && zone !== 'center'
      ? ({ left: '←', right: '→', top: '↑', bottom: '↓' } as const)[zone]
      : intent === 'swap'
        ? '⇄'
        : '';
</script>

{#if zone && intent}
  {#if half}
    <div class="drop-overlay {zone}" aria-hidden="true"></div>
  {:else}
    <div class="drop-overlay full {intent}" aria-hidden="true">
      {#if glyph}<span class="glyph">{glyph}</span>{/if}
    </div>
  {/if}
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
  .full {
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .full.reorient,
  .full.swap {
    background: color-mix(in srgb, var(--accent) 18%, transparent);
    border: 2px dashed var(--accent);
  }
  .glyph {
    font-size: clamp(56px, 9vw, 150px);
    font-weight: 700;
    line-height: 1;
    color: var(--accent);
    opacity: 0.95;
    text-shadow: 0 2px 10px rgba(0, 0, 0, 0.35);
  }
  .left { left: 0; top: 0; width: 50%; height: 100%; }
  .right { right: 0; top: 0; width: 50%; height: 100%; }
  .top { left: 0; top: 0; width: 100%; height: 50%; }
  .bottom { left: 0; bottom: 0; width: 100%; height: 50%; }
</style>
