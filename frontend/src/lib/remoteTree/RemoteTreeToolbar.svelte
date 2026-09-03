<script lang="ts">
  import { t } from '../../i18n/messages';
  import { ChevronsDownUp, ChevronsUpDown, Download, FolderPlus, MonitorDot, SquareTerminal } from 'lucide-svelte';
  import { openLocalTerminal } from '../../actions/localTerminalActions';
  import type { MenuAnchorRect } from '../clampMenuPosition';
  import './remoteTreeShared.css';

  // openLocalTerminal is imported rather than taken as a prop, unlike every other action here.
  // It needs no tree context - no target folder, no selection - so threading it through
  // RemoteTree.svelte would add wiring to a file that is already over its size baseline and may
  // not grow, in exchange for nothing this button would use.
  export let onNewConnection: () => void;
  export let onNewFolder: () => void;
  /** Receives the button's rect so the menu can anchor to it. */
  export let onImport: (anchor: MenuAnchorRect) => void;
  export let onExpandAll: () => void;
  export let onCollapseAll: () => void;
  /** Reflected into aria-expanded so the trigger announces the menu state. */
  export let importMenuOpen = false;

  function handleImport(e: MouseEvent) {
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    onImport({ left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom });
  }
</script>

<div class="tree-toolbar">
  <button class="toolbar-btn" on:click={onNewConnection} title={$t('tree.action.newConnection')}>
    <MonitorDot size={14} />
  </button>
  <button class="toolbar-btn" on:click={onNewFolder} title={$t('tree.action.newFolder')}>
    <FolderPlus size={14} />
  </button>
  <button
    class="toolbar-btn"
    on:click={() => void openLocalTerminal()}
    title={$t('tree.action.newLocalTerminal')}
  >
    <SquareTerminal size={14} />
  </button>
  <button
    class="toolbar-btn"
    on:click|stopPropagation={handleImport}
    title={$t('tree.action.import')}
    aria-haspopup="menu"
    aria-expanded={importMenuOpen}
  >
    <Download size={14} />
  </button>
  <div class="toolbar-spacer"></div>
  <button class="toolbar-btn" on:click={onExpandAll} title={$t('tree.action.expandAll')}>
    <ChevronsUpDown size={14} />
  </button>
  <button class="toolbar-btn" on:click={onCollapseAll} title={$t('tree.action.collapseAll')}>
    <ChevronsDownUp size={14} />
  </button>
</div>
