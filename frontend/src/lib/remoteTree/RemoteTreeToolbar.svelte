<script lang="ts">
  import { ChevronsDownUp, ChevronsUpDown, Download, FolderPlus, MonitorDot } from 'lucide-svelte';
  import './remoteTreeShared.css';

  export let onNewConnection: () => void;
  export let onNewFolder: () => void;
  export let onImport: () => void;
  export let onExpandAll: () => void;
  export let onCollapseAll: () => void;
</script>

<div class="tree-toolbar">
  <button class="toolbar-btn" on:click={onNewConnection} title="New Connection">
    <MonitorDot size={14} />
  </button>
  <button class="toolbar-btn" on:click={onNewFolder} title="New Folder">
    <FolderPlus size={14} />
  </button>
  <button class="toolbar-btn" on:click={onImport} title="Import from PuTTY (.ppk, .reg)">
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
