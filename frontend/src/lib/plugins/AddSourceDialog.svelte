<script lang="ts">
  // Registering a repository, with the trust decision taken at the same moment.
  //
  // Trust is answered here rather than defaulted and edited later because a repository that exists
  // in an undecided state gets read as trusted by whatever looks at it first. The backend's
  // AddGitHubRepositoryRequest carries both fields for the same reason.
  import { createEventDispatcher } from 'svelte';
  import { AlertTriangle } from 'lucide-svelte';
  import Modal from '../Modal.svelte';
  import { isSupportedRepositoryURL } from '../forgeRepo';

  export let show = false;
  export let busy = false;

  const dispatch = createEventDispatcher();

  let url = '';
  let trusted = false;

  $: if (!show) {
    url = '';
    trusted = false;
  }

  $: valid = isSupportedRepositoryURL(url);
  $: showError = url.trim().length > 0 && !valid;

  function submit() {
    if (!valid || busy) return;
    dispatch('add', { url: url.trim(), trusted });
  }
</script>

<Modal title="Add plugin repository" {show} on:close={() => dispatch('cancel')}>
  <div class="add-body">
    <label class="field">
      <span>Repository URL</span>
      <input
        type="text"
        bind:value={url}
        placeholder="https://github.com/owner/plugin"
        on:keydown={(e) => e.key === 'Enter' && submit()}
      />
    </label>
    {#if showError}
      <p class="error">Only github.com and gitlab.com repository URLs are supported.</p>
    {/if}

    <label class="checkbox-row">
      <input type="checkbox" bind:checked={trusted} />
      Trust this repository
    </label>

    {#if !trusted}
      <div class="banner">
        <AlertTriangle size={14} />
        <span>
          Plugins from an untrusted repository still install, and every signature and permission
          check still runs. You will be told the source is untrusted each time.
        </span>
      </div>
    {/if}
  </div>

  <div class="dialog-actions">
    <button class="secondary" disabled={busy} on:click={() => dispatch('cancel')}>Cancel</button>
    <button class="primary" disabled={!valid || busy} on:click={submit}>
      {busy ? 'Checking…' : 'Add'}
    </button>
  </div>
</Modal>

<style>
  .add-body {
    display: flex;
    flex-direction: column;
    gap: 10px;
    min-width: 42ch;
    max-width: 56ch;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
  }

  .error {
    margin: 0;
    font-size: 11px;
    color: var(--danger, #f85149);
  }

  .banner {
    display: flex;
    align-items: flex-start;
    gap: 7px;
    padding: 8px 10px;
    border-radius: 5px;
    background: rgba(210, 153, 34, 0.12);
    color: var(--warning, #d29922);
    font-size: 11.5px;
    line-height: 1.45;
  }
</style>
