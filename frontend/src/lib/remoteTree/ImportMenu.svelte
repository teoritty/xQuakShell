<script lang="ts">
  /**
   * Anchored dropdown listing the import sources. Purely presentational: it
   * renders items and reports which one was chosen, while the parent owns
   * both the anchor rect and the dialogs the choice opens.
   */
  import { createEventDispatcher, tick } from 'svelte';
  import { KeyRound, FileCode } from 'lucide-svelte';
  import { clampMenuPosition, type MenuAnchorRect } from '../clampMenuPosition';

  export let show = false;
  export let anchor: MenuAnchorRect | null = null;

  const dispatch = createEventDispatcher<{ select: 'putty' | 'sshConfig'; close: void }>();

  const items = [
    {
      id: 'sshConfig' as const,
      label: 'From SSH config…',
      hint: '~/.ssh/config — hosts, keys and jump chains',
      icon: FileCode
    },
    {
      id: 'putty' as const,
      label: 'From PuTTY…',
      hint: '.ppk private key or .reg session export',
      icon: KeyRound
    }
  ];

  let menuEl: HTMLDivElement | null = null;
  let position = { left: 0, top: 0 };
  let focusedIndex = 0;

  // The menu is measured after it renders, so its real height feeds the clamp
  // instead of a guess: a menu that opens near the bottom edge has to flip
  // above the button, and guessing wrong makes it hang off-screen.
  $: if (show && anchor) void place(anchor);

  async function place(rect: MenuAnchorRect) {
    focusedIndex = 0;
    await tick();
    const width = menuEl?.offsetWidth ?? 240;
    const height = menuEl?.offsetHeight ?? items.length * 44;
    position = clampMenuPosition(rect, width, height);
    menuEl?.focus();
  }

  function choose(id: 'putty' | 'sshConfig') {
    dispatch('select', id);
    dispatch('close');
  }

  function handleKeydown(e: KeyboardEvent) {
    switch (e.key) {
      case 'Escape':
        e.preventDefault();
        dispatch('close');
        break;
      case 'ArrowDown':
        e.preventDefault();
        focusedIndex = (focusedIndex + 1) % items.length;
        break;
      case 'ArrowUp':
        e.preventDefault();
        focusedIndex = (focusedIndex - 1 + items.length) % items.length;
        break;
      case 'Home':
        e.preventDefault();
        focusedIndex = 0;
        break;
      case 'End':
        e.preventDefault();
        focusedIndex = items.length - 1;
        break;
      case 'Enter':
      case ' ':
        e.preventDefault();
        choose(items[focusedIndex].id);
        break;
    }
  }
</script>

{#if show}
  <div
    class="import-menu"
    style="left: {position.left}px; top: {position.top}px"
    role="menu"
    tabindex="-1"
    aria-label="Import connections from"
    bind:this={menuEl}
    on:keydown={handleKeydown}
    on:click|stopPropagation
  >
    {#each items as item, i (item.id)}
      <button
        class="menu-item"
        class:focused={i === focusedIndex}
        role="menuitem"
        on:click={() => choose(item.id)}
        on:mouseenter={() => (focusedIndex = i)}
      >
        <svelte:component this={item.icon} size={14} />
        <span class="labels">
          <span class="label">{item.label}</span>
          <span class="hint">{item.hint}</span>
        </span>
      </button>
    {/each}
  </div>
{/if}

<style>
  .import-menu {
    position: fixed;
    z-index: 1000;
    min-width: 240px;
    padding: 4px 0;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    outline: none;
  }

  .menu-item {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    width: 100%;
    padding: 7px 12px;
    border: none;
    background: transparent;
    color: var(--text-primary);
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }

  .menu-item:hover,
  .menu-item.focused {
    background: var(--bg-hover);
  }

  .labels {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }

  .label {
    font-size: 12px;
  }

  .hint {
    font-size: 10px;
    color: var(--text-secondary);
  }
</style>
