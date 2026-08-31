<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { X } from 'lucide-svelte';
  export let title: string = '';
  export let show: boolean = false;
  export let contentClass: string = '';
  const dispatch = createEventDispatcher();

  function close() {
    dispatch('close');
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }
</script>

{#if show}
  <div class="modal-backdrop" on:click={close} on:keydown={handleKeydown}>
    <div class="modal-content {contentClass}" on:click|stopPropagation on:keydown|stopPropagation>
      <div class="modal-header" class:has-header-center={$$slots['header-center']}>
        <span class="modal-title">{title}</span>
        {#if $$slots['header-center']}
          <div class="modal-header-center">
            <slot name="header-center" />
          </div>
        {/if}
        <button class="modal-close" on:click={close}><X size={14} /></button>
      </div>
      <div class="modal-body">
        <slot />
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal-content {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    min-width: 360px;
    max-width: 560px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px rgba(0,0,0,0.5);
  }

  .modal-content.scripts-modal {
    width: 60vw;
    max-width: 60vw;
  }

  /* The key manager is a list beside a details pane. At the default 560px the two columns fight
     for the same gutter and every row truncates, which is what a browsing surface must not do. */
  /* height as well as max-height: the ceiling alone let a dialog with one key in it open half the
     size of the same dialog with ten. max-height has to stay, or the base rule's 80vh clamps this
     back down and the pane loses the six percent it was given on purpose. */
  .modal-content:global(.key-manager) {
    width: min(1040px, 92vw);
    max-width: min(1040px, 92vw);
    height: 86vh;
    max-height: 86vh;
  }

  /* Both panes own their own padding so the divider between them reaches the dialog border. */
  .modal-content:global(.key-manager) .modal-body {
    padding: 0;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
  }

  /* Known hosts and trusted peers are the same shape: a list that is very often empty. Sized to
     their content, the dialog jumped between a full-height list and a two-line box, and the empty
     state read as a rendering fault rather than as "there is nothing here yet". The height matches
     the audit log's, so the three list dialogs open as one size instead of three. */
  .modal-content:global(.list-manager) {
    width: 560px;
    max-width: 560px;
    height: 62vh;
  }

  /* The list inside has to be the part that scrolls, which it can only do if the body is a column
     that may shrink below its content. */
  .modal-content:global(.list-manager) .modal-body {
    display: flex;
    flex-direction: column;
    /* flex so the body fills the fixed height instead of ending where its content does, min-height
       so it may shrink below that content, and overflow hidden so the list inside owns the only
       scrollbar - the base rule's overflow-y:auto would otherwise put a second one around it. */
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .modal-content.settings-modal {
    width: 680px;
    max-width: 680px;
  }

  /* Settings draws its own tab sidebar and footer, both of which need their dividers to reach the
     dialog border. A padded body would stop them one gutter short; each region inside pays for its
     own inset instead. */
  .modal-content.settings-modal .modal-body {
    padding: 0;
    overflow: hidden;
  }

  /* The plugins screen is a browsing surface, like the key manager: a rail beside a list of cards
     whose rows carry a name, a version, a source and two controls. At the default 560px every one
     of those rows truncates.

     The height is fixed rather than capped: the sections differ in length, and a dialog that sizes
     to its content resizes under the cursor on every rail click, which moves the rail button the
     user is still pointing at. */
  .modal-content:global(.plugins-modal) {
    width: min(1080px, 92vw);
    max-width: min(1080px, 92vw);
    height: 86vh;
    max-height: 86vh;
  }

  /* The scrolling belongs to the section pane inside, not to the dialog body: the toolbar and the
     rail must stay put while a long list moves. */
  .modal-content:global(.plugins-modal) .modal-body {
    overflow: hidden;
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
  }

  /* Opens on top of the plugins screen, so it is deliberately narrower than it: a details pane
     that covered the list behind it would lose the context the user opened it from. */
  .modal-content:global(.plugin-details-modal) {
    width: min(760px, 88vw);
    max-width: min(760px, 88vw);
    max-height: 82vh;
  }

  .modal-content:global(.plugin-details-modal) .modal-body {
    overflow: hidden;
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
  }

  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border-color);
    gap: 8px;
  }

  .modal-header.has-header-center {
    display: grid;
    grid-template-columns: 1fr minmax(200px, 350px) 1fr;
    align-items: center;
  }

  .modal-header.has-header-center .modal-title {
    justify-self: start;
  }

  .modal-header-center {
    justify-self: center;
    width: 100%;
  }

  .modal-header.has-header-center .modal-close {
    justify-self: end;
  }

  .modal-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-bright);
  }

  .modal-close {
    background: transparent;
    color: var(--text-secondary);
    padding: 2px 6px;
    font-size: 14px;
  }

  .modal-close:hover {
    color: var(--text-bright);
    background: var(--bg-hover);
  }

  .modal-body {
    padding: 16px;
    overflow-y: auto;
  }
</style>
