<script lang="ts">
  import RemoteTree from './RemoteTree.svelte';
  import ConnectionDetails from './ConnectionDetails.svelte';
  import ContributionHost from './ContributionHost.svelte';
  import DiscoveryNodeDetails from './DiscoveryNodeDetails.svelte';
  import { clampSidebarWidth, MIN_SIDEBAR_WIDTH } from './sidebarWidth';
  import { clampPanelHeight } from './sidebarPanelHeight';

  let width = MIN_SIDEBAR_WIDTH;
  let isDragging = false;

  // null until the user drags, and that is the whole of the default: with the variable unset the
  // panels fall back to the 55vh they have always had, including its habit of following the window.
  // Storing a number up front would freeze that behaviour at whatever the window happened to be.
  let panelHeight: number | null = null;
  let bottomEl: HTMLDivElement;

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

  // Dragging the panel's top edge upward makes it taller, so the delta is subtracted. The starting
  // point is the panel's measured height rather than `panelHeight`, which is null until the first
  // drag - without measuring, that first drag would jump from the 55vh default to wherever the
  // pointer happened to be.
  function startPanelResize(e: MouseEvent) {
    isDragging = true;
    const startY = e.clientY;
    const startHeight = bottomEl?.getBoundingClientRect().height ?? 0;

    function onMouseMove(ev: MouseEvent) {
      panelHeight = clampPanelHeight(startHeight - (ev.clientY - startY), window.innerHeight);
    }

    function onMouseUp() {
      isDragging = false;
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    }

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }

  // A width in pixels does not follow the window the way the flex-ratio splits do, so shrinking the
  // window would otherwise leave a sidebar wider than the ceiling that was in force when it was set.
  // The panel ceiling has the same problem once it has been dragged, and none before that, which is
  // why the null is preserved rather than resolved to a number here.
  function clampToWindow() {
    width = clampSidebarWidth(width, window.innerWidth);
    if (panelHeight !== null) {
      panelHeight = clampPanelHeight(panelHeight, window.innerHeight);
    }
  }
</script>

<svelte:window on:resize={clampToWindow} />

<div
  class="sidebar"
  class:no-select={isDragging}
  style="width: {width}px{panelHeight === null ? '' : `; --sidebar-bottom-max: ${panelHeight}px`}"
>
  <RemoteTree />
  <!-- One height for whatever is showing down here, rather than one per panel. Only one of these is
       ever open - selecting a discovery node clears the connection selection and the other way
       round - so a per-panel height would make the boundary jump as the selection changed. -->
  <div class="sidebar-bottom" bind:this={bottomEl}>
    <div
      class="panel-resizer"
      on:mousedown={startPanelResize}
      role="separator"
      aria-orientation="horizontal"
    ></div>
    <ConnectionDetails />
    <DiscoveryNodeDetails />
    <ContributionHost />
  </div>
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

  /* Holds whichever detail panel is open. It has no size of its own: the panels are content-sized
     and cap themselves at --sidebar-bottom-max, so an empty block is genuinely zero tall and the
     tree keeps the whole sidebar. */
  .sidebar-bottom {
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    min-height: 0;
  }

  .panel-resizer {
    height: 4px;
    background: var(--border-color);
    cursor: ns-resize;
    flex-shrink: 0;
    transition: background 0.15s;
  }

  .panel-resizer:hover { background: var(--accent); }

  /* Asks whether a panel actually rendered rather than repeating the three conditions that decide
     it - three copies of that rule in a fourth component would drift, and the drifted one would
     leave a handle that resizes nothing. On a WebKitGTK older than 2.40 :has() does not match and
     the handle simply stays visible; it still drags a ceiling, so nothing breaks. */
  .sidebar-bottom:not(
      :has(:global(.connection-details), :global(.node-details), :global(.plugin-webview-panel))
    )
    > .panel-resizer {
    display: none;
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
