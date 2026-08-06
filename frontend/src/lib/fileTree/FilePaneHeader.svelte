<script lang="ts">
  // Title bar, toolbar, path bar and status strip of a file pane.
  //
  // Both panes had this markup inline and near-identical; the only real
  // differences are the title, the placeholder, and that the remote pane has a
  // "connecting" state while SFTP comes up. It owns the path input outright —
  // the panes used to carry pathInput, pathInputEl and a reactive block to keep
  // them in step, which is bookkeeping for an element they did not otherwise
  // touch.
  import { createEventDispatcher } from 'svelte';
  import OverflowToolbar from '../OverflowToolbar.svelte';
  import type { ToolbarItem } from '../filePanelToolbar';
  import { Loader2, X } from 'lucide-svelte';
  import './fileTreeShared.css';

  export let title: string;
  export let toolbarItems: ToolbarItem[];
  export let currentPath: string;
  export let placeholder = '/';
  export let error = '';
  /** False hides the path bar and shows connectingLabel instead (remote pane, SFTP starting). */
  export let ready = true;
  export let connectingLabel = '';

  const dispatch = createEventDispatcher<{ navigate: string; dismissError: void }>();

  let pathInput = '';
  let pathInputEl: HTMLInputElement | null = null;

  // Follow the pane while the user is not typing. Overwriting a half-typed path
  // because a background refresh landed is the bug this guard exists for.
  $: if (ready && (!pathInputEl || document.activeElement !== pathInputEl)) {
    pathInput = currentPath;
  }

  /**
   * Put the box back in step with the pane.
   *
   * The pane calls this after a rejected navigation: the input still has focus
   * at that point, so the reactive sync above deliberately will not fire, and
   * the box would otherwise keep showing a path the pane never went to.
   */
  export function resetInput() {
    pathInput = currentPath;
  }

  function submit() {
    const trimmed = pathInput.trim();
    if (trimmed) dispatch('navigate', trimmed);
  }
</script>

<div class="panel-header">
  <span>{title}</span>
  <OverflowToolbar items={toolbarItems} />
</div>

{#if ready}
  <div class="path-bar">
    <input
      bind:this={pathInputEl}
      bind:value={pathInput}
      on:keydown={(e) => e.key === 'Enter' && submit()}
      on:blur={() => (pathInput = currentPath)}
      {placeholder}
    />
  </div>
{/if}

{#if !ready && connectingLabel}
  <div class="tree-loading"><Loader2 size={14} /> {connectingLabel}</div>
{:else if error}
  <div class="tree-error">
    <span class="tree-error-msg">{error}</span>
    <button class="tree-error-close" title="Dismiss" on:click={() => dispatch('dismissError')}><X size={12} /></button>
  </div>
{/if}
