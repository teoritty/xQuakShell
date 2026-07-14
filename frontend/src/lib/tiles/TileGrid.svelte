<!-- frontend/src/lib/tiles/TileGrid.svelte -->
<script lang="ts">
  import { tileLayout } from '../../stores/tileLayout';
  import { computeGrid } from './geometry';
  import { computeResizers } from './resizers';
  import TileGroupView from './TileGroup.svelte';
  import TileResizer from './TileResizer.svelte';

  let container: HTMLElement;

  $: n = $tileLayout.tiles.length;
  $: grid = computeGrid(n, $tileLayout.orientation, $tileLayout.dividers);
  $: resizers = computeResizers(n, $tileLayout.orientation, $tileLayout.dividers);
</script>

<div class="tile-grid-wrap" bind:this={container}>
  <div
    class="tile-grid"
    style="grid-template-columns: {grid.columns}; grid-template-rows: {grid.rows};"
  >
    {#each $tileLayout.tiles as tile, i (tile.id)}
      <div class="tile-slot" style="grid-area: {grid.areas[i]};">
        <TileGroupView {tile} />
      </div>
    {/each}
  </div>
  {#each resizers as spec (spec.divider)}
    <TileResizer {spec} {container} />
  {/each}
</div>

<style>
  .tile-grid-wrap {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
  .tile-grid {
    display: grid;
    width: 100%;
    height: 100%;
    gap: 0;
  }
  .tile-slot {
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    display: flex;
  }
  .tile-slot > :global(.tile-group) {
    flex: 1;
  }
</style>
