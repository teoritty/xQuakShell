<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { getPluginContributions, executePluginCommand, type PluginCommand } from '../stores/api';
  import { Search, X } from 'lucide-svelte';

  export let open = false;

  let query = '';
  let commands: PluginCommand[] = [];
  let filtered: PluginCommand[] = [];
  let selected = 0;
  let inputEl: HTMLInputElement;
  let status = '';
  let running = false;

  $: filtered = commands.filter(c => {
    const q = query.trim().toLowerCase();
    if (!q) return true;
    return (
      c.title.toLowerCase().includes(q) ||
      c.category?.toLowerCase().includes(q) ||
      c.fullId.toLowerCase().includes(q)
    );
  });

  $: if (selected >= filtered.length) selected = Math.max(0, filtered.length - 1);

  export async function refresh() {
    const data = await getPluginContributions();
    commands = data.commands || [];
  }

  export function openPalette() {
    open = true;
    query = '';
    selected = 0;
    status = '';
    void refresh().then(async () => {
      await tick();
      inputEl?.focus();
    });
  }

  export function closePalette() {
    open = false;
    query = '';
    status = '';
  }

  async function runCommand(cmd: PluginCommand) {
    if (running) return;
    running = true;
    status = `Running ${cmd.title}...`;
    try {
      const result = await executePluginCommand(cmd.pluginId, cmd.id);
      const msg = result?.message || 'Done';
      status = msg;
      setTimeout(closePalette, 600);
    } catch (e: any) {
      status = e?.message || 'Command failed';
    } finally {
      running = false;
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      closePalette();
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selected = Math.min(selected + 1, Math.max(0, filtered.length - 1));
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      selected = Math.max(selected - 1, 0);
      return;
    }
    if (e.key === 'Enter' && filtered[selected]) {
      e.preventDefault();
      void runCommand(filtered[selected]);
    }
  }

  onMount(() => {
    void refresh();
  });
</script>

<svelte:window on:keydown={onKeydown} />

{#if open}
  <div class="palette-backdrop" on:click={closePalette} role="presentation">
    <div class="palette" on:click|stopPropagation role="dialog" aria-label="Command palette">
      <div class="palette-search">
        <Search size={14} />
        <input
          bind:this={inputEl}
          type="text"
          placeholder="Type a command..."
          bind:value={query}
        />
        <button class="ghost icon-btn" on:click={closePalette} title="Close"><X size={14} /></button>
      </div>
      <div class="palette-list">
        {#if filtered.length === 0}
          <div class="palette-empty">No commands found</div>
        {:else}
          {#each filtered as cmd, idx}
            <button
              class="palette-item"
              class:selected={idx === selected}
              on:click={() => runCommand(cmd)}
              on:mouseenter={() => (selected = idx)}
            >
              <span class="cmd-title">{cmd.title}</span>
              {#if cmd.category}
                <span class="cmd-category">{cmd.category}</span>
              {/if}
            </button>
          {/each}
        {/if}
      </div>
      {#if status}
        <div class="palette-status">{status}</div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .palette-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    z-index: 9000;
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 12vh;
  }

  .palette {
    width: min(520px, 92vw);
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.35);
    overflow: hidden;
  }

  .palette-search {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border-color);
    color: var(--text-secondary);
  }

  .palette-search input {
    flex: 1;
    border: none;
    background: transparent;
    color: var(--text-primary);
    font-size: 13px;
    outline: none;
  }

  .icon-btn {
    padding: 2px;
    display: inline-flex;
  }

  .palette-list {
    max-height: 320px;
    overflow-y: auto;
  }

  .palette-item {
    width: 100%;
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border: none;
    background: transparent;
    color: var(--text-primary);
    text-align: left;
    cursor: pointer;
    font-size: 12px;
  }

  .palette-item:hover,
  .palette-item.selected {
    background: var(--bg-tertiary);
  }

  .cmd-title {
    flex: 1;
  }

  .cmd-category {
    font-size: 10px;
    color: var(--text-secondary);
  }

  .palette-empty,
  .palette-status {
    padding: 10px 12px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .palette-status {
    border-top: 1px solid var(--border-color);
    color: var(--text-primary);
  }
</style>
