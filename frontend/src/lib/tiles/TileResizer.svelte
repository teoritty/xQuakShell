<!-- frontend/src/lib/tiles/TileResizer.svelte -->
<script lang="ts">
  import type { ResizerSpec } from './resizers';
  import { nextRatio } from './resizeMath';
  import { setDivider, tileLayout } from '../../stores/tileLayout';

  export let spec: ResizerSpec;
  export let container: HTMLElement | null;

  function onMouseDown(e: MouseEvent) {
    if (!container) return;
    e.preventDefault();
    const box = container.getBoundingClientRect();
    const horizontal = spec.axis === 'x';
    const start = horizontal ? e.clientX : e.clientY;
    const size = horizontal ? box.width : box.height;
    const startRatio = spec.divider === 'main' ? $tileLayout.dividers.main : $tileLayout.dividers.cross;

    function onMove(ev: MouseEvent) {
      const cur = horizontal ? ev.clientX : ev.clientY;
      setDivider(spec.divider, nextRatio(startRatio, cur - start, size));
    }
    function onUp() {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
      window.dispatchEvent(new Event('resize'));
    }
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }
</script>

<div
  class="tile-resizer"
  class:x={spec.axis === 'x'}
  class:y={spec.axis === 'y'}
  style="left: {spec.xPct}%; top: {spec.yPct}%; width: {spec.wPct}%; height: {spec.hPct}%;"
  on:mousedown={onMouseDown}
  role="separator"
  aria-orientation={spec.axis === 'x' ? 'vertical' : 'horizontal'}
></div>

<style>
  .tile-resizer {
    position: absolute;
    z-index: 5;
    background: transparent;
    transition: background 0.15s;
  }
  /* The rect is a zero-thickness line; expand the hit area around it. */
  .tile-resizer.x {
    transform: translateX(-3px);
    width: 6px !important;
    cursor: ew-resize;
  }
  .tile-resizer.y {
    transform: translateY(-3px);
    height: 6px !important;
    cursor: ns-resize;
  }
  .tile-resizer:hover { background: var(--accent); }
</style>
