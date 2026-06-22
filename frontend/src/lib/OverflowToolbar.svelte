<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import { MoreHorizontal, Check } from 'lucide-svelte';
  import { clampMenuPosition } from './clampMenuPosition';
  import type { ToolbarItem } from './filePanelToolbar';

  export let items: ToolbarItem[] = [];

  const GAP = 2;

  let containerEl: HTMLDivElement;
  let measureEl: HTMLDivElement;
  let headerEl: HTMLElement | null = null;
  let triggerEl: HTMLButtonElement | null = null;
  let menuEl: HTMLDivElement | null = null;

  let overflowCount = 0;
  let menuOpen = false;
  let menuLeft = 0;
  let menuTop = 0;
  let menuPositioned = false;

  let itemWidths: number[] = [];
  let ellipsisWidth = 28;
  let rafId: number | null = null;
  let resizeObserver: ResizeObserver | null = null;

  $: overflowItems = items.slice(0, overflowCount);
  $: visibleItems = items.slice(overflowCount);
  $: if (overflowCount === 0) menuOpen = false;

  function scheduleLayout() {
    if (rafId != null) cancelAnimationFrame(rafId);
    rafId = requestAnimationFrame(() => {
      rafId = null;
      void runLayout();
    });
  }

  async function runLayout() {
    await tick();
    await measure();
    computeOverflow();
    if (menuOpen) await positionMenu();
  }

  async function measure() {
    if (!measureEl || items.length === 0) return;
    const buttons = measureEl.querySelectorAll<HTMLElement>('[data-measure-item]');
    if (buttons.length !== items.length) return;
    itemWidths = Array.from(buttons).map((el) => el.getBoundingClientRect().width);
    const ellipsisEl = measureEl.querySelector<HTMLElement>('[data-measure-ellipsis]');
    if (ellipsisEl) ellipsisWidth = ellipsisEl.getBoundingClientRect().width;
  }

  function getAvailableWidth(): number {
    if (!containerEl) return 0;
    // Prefer flex-assigned width; fall back to header minus title when layout is not ready.
    const width = containerEl.clientWidth;
    if (width > 0) return width;
    if (!headerEl) return 0;
    const titleEl = headerEl.querySelector(':scope > span:first-child') as HTMLElement | null;
    if (!titleEl) return 0;
    const gap = parseFloat(getComputedStyle(headerEl).columnGap || getComputedStyle(headerEl).gap || '8') || 8;
    return Math.max(0, headerEl.clientWidth - titleEl.offsetWidth - gap);
  }

  function computeOverflow() {
    if (!containerEl || itemWidths.length !== items.length) return;
    const available = getAvailableWidth();
    let count = 0;
    while (count < items.length) {
      let total = 0;
      for (let i = count; i < items.length; i++) {
        total += itemWidths[i];
        if (i > count) total += GAP;
      }
      if (count > 0) total += ellipsisWidth + GAP;
      if (total <= available) break;
      count++;
    }
    overflowCount = count;
  }

  async function positionMenu() {
    await tick();
    if (!menuOpen || !triggerEl || !menuEl) return;
    const anchor = triggerEl.getBoundingClientRect();
    const menuRect = menuEl.getBoundingClientRect();
    const pos = clampMenuPosition(anchor, menuRect.width, menuRect.height);
    menuLeft = pos.left;
    menuTop = pos.top;
    menuPositioned = true;
  }

  async function toggleMenu() {
    if (menuOpen) {
      menuOpen = false;
      menuPositioned = false;
      return;
    }
    menuOpen = true;
    menuPositioned = false;
    await tick();
    await positionMenu();
    await tick();
    await positionMenu();
  }

  function closeMenu() {
    menuOpen = false;
    menuPositioned = false;
  }

  function handleItemClick(item: ToolbarItem) {
    if (item.disabled) return;
    item.onClick();
    closeMenu();
  }

  function handleTriggerKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      void toggleMenu();
    } else if (e.key === 'Escape') {
      closeMenu();
    }
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && menuOpen) closeMenu();
  }

  $: if (items) scheduleLayout();

  onMount(() => {
    headerEl = containerEl?.closest('.panel-header') ?? null;
    resizeObserver = new ResizeObserver(scheduleLayout);
    if (containerEl) resizeObserver.observe(containerEl);
    if (headerEl) resizeObserver.observe(headerEl);
    window.addEventListener('resize', scheduleLayout);
    scheduleLayout();
  });

  onDestroy(() => {
    if (rafId != null) cancelAnimationFrame(rafId);
    resizeObserver?.disconnect();
    window.removeEventListener('resize', scheduleLayout);
  });
</script>

<svelte:window on:click={closeMenu} on:keydown={handleWindowKeydown} />

<div class="actions overflow-toolbar" bind:this={containerEl}>
  <div class="measure-row" bind:this={measureEl} aria-hidden="true">
    {#each items as item (item.id)}
      <button
        type="button"
        data-measure-item
        class={item.buttonClass}
        class:active={item.active}
        disabled={item.disabled}
      >
        <svelte:component this={item.icon} size={12} />
        {#if item.inlineSuffix}
          <span>{item.inlineSuffix}</span>
        {/if}
      </button>
    {/each}
    <button type="button" data-measure-ellipsis class="overflow-trigger">
      <MoreHorizontal size={12} />
    </button>
  </div>

  {#if overflowCount > 0}
    <button
      bind:this={triggerEl}
      type="button"
      class="overflow-trigger"
      title="More actions"
      aria-haspopup="menu"
      aria-expanded={menuOpen}
      tabindex="0"
      on:click|stopPropagation={toggleMenu}
      on:keydown|stopPropagation={handleTriggerKeydown}
    >
      <MoreHorizontal size={12} />
    </button>
  {/if}

  {#each visibleItems as item (item.id)}
    <button
      type="button"
      class={item.buttonClass}
      class:active={item.active}
      title={item.label}
      disabled={item.disabled}
      on:click={() => item.onClick()}
    >
      <svelte:component this={item.icon} size={12} />
      {#if item.inlineSuffix}
        <span>{item.inlineSuffix}</span>
      {/if}
    </button>
  {/each}

  {#if menuOpen && overflowCount > 0}
    <div
      bind:this={menuEl}
      class="overflow-menu"
      style="left: {menuLeft}px; top: {menuTop}px; visibility: {menuPositioned ? 'visible' : 'hidden'}"
      role="menu"
      tabindex="-1"
      on:click|stopPropagation
      on:keydown|stopPropagation={(e) => e.key === 'Escape' && closeMenu()}
    >
      {#each overflowItems as item (item.id)}
        <button
          type="button"
          class="overflow-menu-item"
          class:active={item.active}
          role="menuitem"
          disabled={item.disabled}
          on:click={() => handleItemClick(item)}
        >
          <svelte:component this={item.icon} size={12} />
          <span class="overflow-menu-label">{item.label}{item.menuSuffix || ''}</span>
          {#if item.showCheck && item.active}
            <span class="overflow-menu-check"><Check size={12} /></span>
          {/if}
        </button>
      {/each}
    </div>
  {/if}
</div>
