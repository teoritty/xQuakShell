<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Activity, Trash2 } from 'lucide-svelte';
  import PluginCard from './PluginCard.svelte';
  import { filterInstalled } from './pluginsView';
  import type { PluginInfo } from '../../api/plugins';

  export let plugins: PluginInfo[] = [];
  export let query = '';
  export let busyPluginId = '';

  const dispatch = createEventDispatcher();

  $: visible = filterInstalled(plugins, query);

  // 'enforced-partial' is a real boundary with a dimension missing (a Linux kernel below 6.7
  // confines the filesystem and not the network), so it gets its own wording. Rounding it up to
  // "sandboxed" would claim containment the user does not have.
  function sandboxLabel(mode: string | undefined): string {
    switch (mode) {
      case 'enforced':
        return 'Sandboxed';
      case 'enforced-partial':
        return 'Partially sandboxed';
      case 'unavailable':
        return 'Sandbox unavailable';
      case 'disabled':
        return 'Sandbox disabled';
      default:
        return '';
    }
  }

  function statusFor(plugin: PluginInfo): { text: string; kind: 'installed' | 'warning' } {
    if (!plugin.enabled) return { text: 'Disabled', kind: 'warning' };
    const sandbox = sandboxLabel(plugin.sandboxMode);
    const running = plugin.state === 'running';
    if (sandbox && sandbox !== 'Sandboxed') {
      return { text: sandbox, kind: 'warning' };
    }
    return { text: running ? sandbox || 'Running' : plugin.state || 'Stopped', kind: 'installed' };
  }
</script>

{#if plugins.length === 0}
  <p class="empty">No plugins are installed yet. Browse a source to add one.</p>
{:else if visible.length === 0}
  <p class="empty">No installed plugin matches “{query}”.</p>
{:else}
  <div class="card-list">
    {#each visible as plugin (plugin.id)}
      {@const status = statusFor(plugin)}
      <PluginCard
        name={plugin.name}
        version={plugin.version}
        description={plugin.description}
        origin={plugin.source}
        signed={plugin.signed}
        status={status.text}
        statusKind={status.kind}
        disabled={!plugin.enabled}
      >
        <svelte:fragment slot="actions">
          <label class="toggle" title={plugin.enabled ? 'Disable' : 'Enable'}>
            <input
              type="checkbox"
              checked={plugin.enabled}
              disabled={busyPluginId === plugin.id}
              on:change={(e) =>
                dispatch('toggle', { plugin, enabled: e.currentTarget.checked })}
            />
          </label>
          <button
            class="ghost icon-btn"
            title="Ping"
            disabled={busyPluginId === plugin.id || !plugin.enabled}
            on:click={() => dispatch('ping', { plugin })}
          >
            <Activity size={13} />
          </button>
          <button
            class="ghost icon-btn danger"
            title="Uninstall"
            disabled={busyPluginId === plugin.id}
            on:click={() => dispatch('uninstall', { plugin })}
          >
            <Trash2 size={13} />
          </button>
        </svelte:fragment>
      </PluginCard>
    {/each}
  </div>
{/if}

<style>
  .card-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .empty {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .toggle {
    display: inline-flex;
    align-items: center;
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    padding: 3px;
  }

  .icon-btn.danger:hover:not(:disabled) {
    color: var(--danger, #f85149);
  }
</style>
