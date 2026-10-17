<script lang="ts">
  import { t } from '../i18n/messages';
  import { createEventDispatcher } from 'svelte';
  import { get } from 'svelte/store';
  import { ArrowRightLeft, CopyPlus, FolderPlus, MonitorDot, Pencil, Star, Trash2 } from 'lucide-svelte';
  import { connections, selectedConnectionIds, selectedConnectionId } from '../stores/appState';
  import { canWrite, requestVaultMove, teams } from '../stores/teamState';
  import { duplicateConnections } from '../actions/connectionActions';
  import { closeActiveContextMenu } from './contextMenuManager';
  import type { DiscoveryMenu, DiscoveryMenuItem } from './remoteTree/discoveryActions';

  export let x = 0;
  export let y = 0;
  export let show = false;
  export let isFolder = false;
  export let isConnection = false;
  export let isFavorite = false;
  /**
   * Discovery rows replace the whole static menu rather than adding to it: the
   * core's items (edit, delete, favourite) act on local config and mean nothing
   * for a remote resource a plugin drew. Already computed by
   * discoveryActions.ts — this component only draws it.
   */
  export let discoveryMenu: DiscoveryMenu | null = null;

  const dispatch = createEventDispatcher<{
    newConnection: void;
    newFolder: void;
    edit: void;
    delete: void;
    toggleFavorite: void;
    invokeAction: DiscoveryMenuItem;
  }>();

  // Acts on the selection, as Delete does. A right-click on an unselected row selects it alone
  // before the menu opens, so the selection is always what the user right-clicked.
  function selection(): string[] {
    const ids = [...get(selectedConnectionIds)];
    return ids.length > 0 ? ids : [get(selectedConnectionId)].filter(Boolean);
  }

  function duplicate() {
    const target = selection();
    closeActiveContextMenu();
    void duplicateConnections(target, {
      first: $t('tree.duplicateName'),
      numbered: $t('tree.duplicateNameNumbered'),
    });
  }

  // "Move to": every vault the selected connections could go to - the personal one and each team this
  // account may write to - except the one they are all already in. Only once there is somewhere else
  // to go: without teams the personal vault is the only vault, and the menu stays as it always was.
  $: selected = show ? selection() : [];
  $: here = new Set($connections.filter((c) => selected.includes(c.id)).map((c) => c.vaultId ?? ''));
  $: moveTargets = [{ vault: '', name: $t('tree.vault.personal') },
    ...$teams.filter(canWrite).map((x) => ({ vault: x.vault, name: x.name || $t('sync.teams.unnamed') }))]
    .filter((d) => !(here.size === 1 && here.has(d.vault)));
  // A move takes the connection out of where it is, so a viewer's team items offer none: the backend
  // would refuse it, and a menu item that only ever fails is worse than none.
  $: movable = [...here].every((v) => v === '' || canWrite($teams.find((x) => x.vault === v)));
  $: offerMove = ($teams.length > 0 || [...here].some(Boolean)) && moveTargets.length > 0 && movable;

  function moveTo(vault: string) {
    const connectionIds = selection();
    closeActiveContextMenu();
    requestVaultMove({ connectionIds, vault, folder: '' });
  }
</script>

{#if show}
  <div class="context-menu" style="left: {x}px; top: {y}px" role="menu" on:click|stopPropagation>
    {#if discoveryMenu}
      {#each discoveryMenu.items as item (item.id)}
        <button
          class="menu-item"
          class:danger={item.danger}
          disabled={item.disabled}
          on:click={() => !item.disabled && dispatch('invokeAction', item)}
          role="menuitem"
        >
          <span>{item.label}</span>
        </button>
      {/each}
      <!-- An empty menu still says why. A menu that simply is not there reads as
           a bug, and "no action applies to all 4 selected items" is an answer. -->
      {#if discoveryMenu.notice}
        <div class="menu-notice">{discoveryMenu.notice}</div>
      {/if}
    {:else}
    {#if isFolder}
      <button class="menu-item" on:click={() => dispatch('newConnection')} role="menuitem">
        <MonitorDot size={12} />
        <span>{$t('tree.action.newConnection')}</span>
      </button>
      <button class="menu-item" on:click={() => dispatch('newFolder')} role="menuitem">
        <FolderPlus size={12} />
        <span>{$t('tree.action.newFolder')}</span>
      </button>
      <button class="menu-item" on:click={() => dispatch('edit')} role="menuitem">
        <Pencil size={12} />
        <span>{$t('common.edit')}</span>
      </button>
    {/if}
    {#if isConnection}
      <button class="menu-item" on:click={() => dispatch('edit')} role="menuitem">
        <Pencil size={12} />
        <span>{$t('common.edit')}</span>
      </button>
      <button class="menu-item" on:click={duplicate} role="menuitem">
        <CopyPlus size={12} />
        <span>{$t('tree.action.duplicate')}</span>
      </button>
      <button class="menu-item" on:click={() => dispatch('toggleFavorite')} role="menuitem">
        <span class="star-icon" class:filled={isFavorite}><Star size={12} /></span>
        <span>{isFavorite ? $t('tree.favorites.remove') : $t('tree.favorites.add')}</span>
      </button>
      {#if offerMove}
        <div class="menu-label" role="presentation"><ArrowRightLeft size={12} /><span>{$t('tree.action.moveTo')}</span></div>
        {#each moveTargets as d (d.vault)}
          <button class="menu-item menu-sub" on:click={() => moveTo(d.vault)} role="menuitem"><span>{d.name}</span></button>
        {/each}
      {/if}
    {/if}
    <button class="menu-item danger" on:click={() => dispatch('delete')} role="menuitem">
      <Trash2 size={12} />
      <span>{$t('common.delete')}</span>
    </button>
    {/if}
  </div>
{/if}

<style>
  .context-menu {
    position: fixed;
    z-index: 1000;
    min-width: 180px;
    padding: 4px 0;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }

  .menu-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 6px 12px;
    border: none;
    background: transparent;
    color: var(--text-primary);
    font-size: 12px;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }

  .menu-item:hover {
    background: var(--bg-hover);
  }

  .menu-item.danger:hover {
    background: rgba(211, 47, 47, 0.2);
    color: var(--danger);
  }

  .menu-item .star-icon.filled :global(svg) {
    fill: currentColor;
  }

  .menu-item:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .menu-item:disabled:hover {
    background: transparent;
  }

  .menu-label {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 12px 2px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .menu-sub {
    padding-left: 32px;
  }

  .menu-notice {
    padding: 6px 12px;
    max-width: 260px;
    font-size: 11px;
    color: var(--text-secondary);
    white-space: normal;
  }
</style>
