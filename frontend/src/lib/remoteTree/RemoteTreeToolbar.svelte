<script lang="ts">
  import { ChevronsDownUp, ChevronsUpDown, Download, FolderPlus, MonitorDot } from 'lucide-svelte';
  import type { MenuAnchorRect } from '../clampMenuPosition';
  import './remoteTreeShared.css';

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
  <button class="toolbar-btn" on:click={onNewConnection} title="New Connection">
    <MonitorDot size={14} />
  </button>
  <button class="toolbar-btn" on:click={onNewFolder} title="New Folder">
    <FolderPlus size={14} />
  </button>
  <button
    class="toolbar-btn"
    on:click|stopPropagation={handleImport}
    title="Import connections"
    aria-haspopup="menu"
    aria-expanded={importMenuOpen}
  >
    <Download size={14} />
  </button>
  <div class="toolbar-spacer"></div>
  <button class="toolbar-btn" on:click={onExpandAll} title="Expand All">
    <ChevronsUpDown size={14} />
  </button>
  <button class="toolbar-btn" on:click={onCollapseAll} title="Collapse All">
    <ChevronsDownUp size={14} />
  </button>
</div>
