<script lang="ts">
  import RemoteTree from './RemoteTree.svelte';
  import ConnectionDetails from './ConnectionDetails.svelte';
  import ContributionHost from './ContributionHost.svelte';
  import DiscoveryNodeDetails from './DiscoveryNodeDetails.svelte';
  import { clampSidebarWidth, MIN_SIDEBAR_WIDTH } from './sidebarWidth';

  let width = MIN_SIDEBAR_WIDTH;
  let isDragging = false;

  // The same shape as the session view's file-column resizer: listeners on the window rather than
  // the handle, so a fast drag that outruns the pointer does not drop the gesture on the way out.
  function startResize(e: MouseEvent) {
    isDragging = true;
    const startX = e.clientX;
    const startWidth = width;

    function onMouseMove(ev: MouseEvent) {
      width = clampSidebarWidth(startWidth + (ev.clientX - startX), window.innerWidth);
    }

    function onMouseUp() {
      isDragging = false;
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
      // The terminal sizes itself off its container and only remeasures on this event, so without
      // it the columns move and the terminal keeps rendering at the width it had before.
      window.dispatchEvent(new Event('resize'));
    }

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }

  // A width in pixels does not follow the window the way the flex-ratio splits do, so shrinking the
  // window would otherwise leave a sidebar wider than the ceiling that was in force when it was set.
  function clampToWindow() {
    width = clampSidebarWidth(width, window.innerWidth);
  }
</script>

<svelte:window on:resize={clampToWindow} />

<div class="sidebar" class:no-select={isDragging} style="width: {width}px">
  <RemoteTree />
  <ConnectionDetails />
  <!-- The other kind of tree row's details. Only one of the two is ever open: selecting a node
       clears the connection selection and the other way round. -->
  <DiscoveryNodeDetails />
  <ContributionHost />
</div>
<div
  class="sidebar-handle"
  on:mousedown={startResize}
  role="separator"
  aria-orientation="vertical"
></div>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    min-width: 280px;
    background: var(--bg-secondary);
    border-right: 1px solid var(--border-color);
    overflow: hidden;
    flex-shrink: 0;
  }

  .sidebar.no-select {
    user-select: none;
  }

  .sidebar-handle {
    width: 4px;
    background: var(--border-color);
    cursor: ew-resize;
    flex-shrink: 0;
    transition: background 0.15s;
  }

  .sidebar-handle:hover { background: var(--accent); }
</style>
