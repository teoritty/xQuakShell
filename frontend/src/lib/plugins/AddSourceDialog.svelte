<script lang="ts">
  import { t } from '../../i18n/messages';
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

<Modal title={$t('plugins.sources.addTitle')} {show} on:close={() => dispatch('cancel')}>
  <div class="add-body">
    <label class="field">
      <span>{$t('plugins.sources.urlLabel')}</span>
      <input
        type="text"
        bind:value={url}
        placeholder="https://github.com/owner/plugin"
        on:keydown={(e) => e.key === 'Enter' && submit()}
      />
    </label>
    {#if showError}
      <p class="error">{$t('plugins.sources.urlInvalid')}</p>
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
    <button class="secondary" disabled={busy} on:click={() => dispatch('cancel')}>{$t('common.cancel')}</button>
    <button class="primary" disabled={!valid || busy} on:click={submit}>
      {busy ? $t('plugins.sources.checking') : $t('common.add')}
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
    color: var(--danger);
  }

  .banner {
    display: flex;
    align-items: flex-start;
    gap: 7px;
    padding: 8px 10px;
    border-radius: 5px;
    background: rgba(196, 144, 64, 0.14);
    color: var(--warning);
    font-size: 11.5px;
    line-height: 1.45;
  }
</style>
