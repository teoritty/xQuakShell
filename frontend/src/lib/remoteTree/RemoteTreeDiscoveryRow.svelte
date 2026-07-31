<script lang="ts">
  // A row inside a plugin-drawn subtree (ADR-014), plus the host's own service
  // lines (loading / error / truncated / empty).
  //
  // It reuses the tree's existing classes on purpose — .folder-arrow, .conn-icon,
  // .node-name, .ping-dot — so the subtree reads as part of the tree rather than
  // as a panel someone embedded in it. No parallel indentation system: the
  // wrapper in RemoteTreeNode.svelte applies the same --ui-scale padding and the
  // same indent guides every other row gets.
  //
  // NO {@html} ANYWHERE. Labels come from a plugin and are interpolated as text;
  // icons go through PluginIcon.svelte, which explains why they are <img>.
  import { createEventDispatcher } from 'svelte';
  import { AlertTriangle, ChevronDown, ChevronRight, Loader2 } from 'lucide-svelte';
  import PluginIcon from './PluginIcon.svelte';
  import StatusDot from './StatusDot.svelte';
  import type { StatusDot as StatusDotSpec } from './statusDot';
  import { discoveryKey, type TreeNode } from './types';

  export let node: TreeNode;
  /** Keyed by discoveryKey(pluginId, iconId) — iconIds are plugin-scoped. */
  export let icons: Map<string, string> = new Map();

  const dispatch = createEventDispatcher();

  $: row = node.discovery;
  $: notice = node.notice;
  // A missing status renders NOTHING. A present status with tone 'neutral'
  // renders a grey dot. Collapsing those two would tell the user the plugin has
  // an opinion about a resource when it has not offered one.
  $: status = (row?.status ?? null) as StatusDotSpec | null;
  $: iconSrc = row ? (icons.get(discoveryKey(row.pluginId, row.iconId)) ?? '') : '';
</script>

{#if notice}
  <span class="discovery-notice" class:error={notice.kind === 'error'}>
    {#if notice.kind === 'loading'}
      <span class="spinning"><Loader2 size={10} /></span>
    {:else if notice.kind === 'error'}
      <AlertTriangle size={10} />
    {/if}
    <span class="discovery-notice-text" title={notice.text}>{notice.text}</span>
  </span>
{:else if row}
  {#if row.kind === 'group'}
    <span
      class="folder-arrow"
      role="button"
      tabindex="-1"
      on:click|stopPropagation={() => dispatch('toggleDiscoveryNode', { row })}
      on:keydown|stopPropagation={(e) => e.key === 'Enter' && dispatch('toggleDiscoveryNode', { row })}
    >
      {#if row.expanded}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}
    </span>
  {:else}
    <span class="folder-arrow"></span>
  {/if}
  <StatusDot {status} />
  <PluginIcon src={iconSrc} label={row.label} />
  <span class="node-name" title={row.label}>{row.label}</span>
  {#if row.branchState === 'stale'}
    <span class="discovery-flag" title="The session that reported this handed over — refreshing">stale</span>
  {/if}
{/if}

<style>
  .discovery-notice {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding-left: 18px;
    font-size: 11px;
    color: var(--text-secondary);
    min-width: 0;
  }

  .discovery-notice.error {
    color: var(--danger, #f44747);
  }

  .discovery-notice-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .discovery-notice .spinning {
    display: inline-flex;
    animation: remote-tree-spin 1s linear infinite;
  }

  .discovery-flag {
    font-size: 8px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--text-secondary);
    flex-shrink: 0;
  }
</style>
