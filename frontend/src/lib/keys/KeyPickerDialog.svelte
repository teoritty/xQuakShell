<script lang="ts">
  import { t } from '../../i18n/messages';
  import { createEventDispatcher } from 'svelte';
  import Modal from '../Modal.svelte';
  import NewKeyDialog from './NewKeyDialog.svelte';
  import { addKeyFromFile, createKey, refreshKeys, storedKeys } from '../../actions/keyActions';
  import type { StoredKey } from '../../api/keys';

  export let show = false;
  // Keys the connection already uses. They stay visible but cannot be picked twice, which is
  // clearer than hiding them and leaving the user wondering where a key they know they have went.
  export let alreadyChosen: string[] = [];

  const dispatch = createEventDispatcher<{ pick: string; close: void }>();

  let filter = '';
  let showNew = false;

  $: if (show) void refreshKeys();

  $: visible = $storedKeys.filter((key) => {
    const needle = filter.trim().toLowerCase();
    if (!needle) return true;
    return (
      key.comment.toLowerCase().includes(needle) ||
      key.keyType.toLowerCase().includes(needle) ||
      (key.fingerprint || '').toLowerCase().includes(needle)
    );
  });

  function describe(key: StoredKey): string {
    if (key.keyType === 'openssh' || key.keyType === 'unknown') return 'type unknown';
    return key.bits ? `${key.keyType} ${key.bits}` : key.keyType;
  }

  function shortFingerprint(key: StoredKey): string {
    const raw = (key.fingerprint || '').replace(/^SHA256:/, '');
    return raw ? `…${raw.slice(-12)}` : '';
  }

  // A key created from here is picked straight away: opening this dialog is already the statement
  // that the connection needs a key, so making the user find the new one in the list is a step
  // that carries no decision.
  async function onCreate(event: CustomEvent) {
    const d = event.detail;
    const ok = d.mode === 'generate'
      ? await createKey(d.algorithm, d.bits, d.comment, d.passphrase, d.options)
      : await addKeyFromFile(d.pemBase64, d.passphrase, d.comment, d.options);
    if (!ok) return;
    showNew = false;
    const newest = $storedKeys.find((key) => key.comment === d.comment);
    if (newest) dispatch('pick', newest.id);
  }
</script>

<Modal title={$t('keys.picker.title')} {show} on:close={() => dispatch('close')}>
  <p class="intro">
    {$t('keys.picker.intro')}
  </p>

  <input class="search" placeholder={$t('keys.search')} bind:value={filter} />

  <ul class="picker">
    {#each visible as key (key.id)}
      {@const used = alreadyChosen.includes(key.id)}
      <li>
        <button class="row" disabled={used} on:click={() => dispatch('pick', key.id)}>
          <span class="top">
            <span class="name">{key.comment || key.id}</span>
            {#if used}<span class="tag">{$t('keys.picker.alreadyAdded')}</span>{/if}
            {#if key.migrationPending}<span class="tag warn">{$t('keys.badge.unfinished')}</span>{/if}
          </span>
          <span class="bottom">
            <span>{describe(key)}</span>
            {#if shortFingerprint(key)}<span class="fingerprint">{shortFingerprint(key)}</span>{/if}
          </span>
        </button>
      </li>
    {:else}
      <li class="empty">
        {filter ? $t('keys.empty.filtered') : $t('keys.picker.empty')}
      </li>
    {/each}
  </ul>

  <div class="dialog-actions">
    <button class="secondary" on:click={() => (showNew = true)}>{$t('keys.picker.add')}</button>
    <button on:click={() => dispatch('close')}>{$t('common.cancel')}</button>
  </div>
</Modal>

<NewKeyDialog bind:show={showNew} on:submit={onCreate} on:close={() => (showNew = false)} />

<style>
  .intro {
    margin: 0 0 10px;
    font-size: 12px;
    line-height: 1.5;
    color: var(--text-secondary);
  }

  .search {
    width: 100%;
    margin-bottom: 8px;
  }

  .picker {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 320px;
    overflow-y: auto;
    border: 1px solid var(--border-color);
    border-radius: 3px;
  }

  .row {
    display: flex;
    flex-direction: column;
    gap: 3px;
    width: 100%;
    padding: 8px 10px;
    background: transparent;
    border: none;
    border-radius: 0;
    border-bottom: 1px solid var(--border-color);
    color: var(--text-primary);
    text-align: left;
  }

  li:last-child .row {
    border-bottom: none;
  }

  .row:hover:not(:disabled) {
    background: var(--bg-hover);
    border-color: transparent;
    border-bottom-color: var(--border-color);
  }

  .top {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  .name {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    color: var(--text-bright);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .bottom {
    display: flex;
    gap: 8px;
    align-items: baseline;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .fingerprint {
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .tag {
    flex-shrink: 0;
    padding: 1px 5px;
    border-radius: 2px;
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    font-size: 10px;
  }

  .tag.warn {
    background: rgba(196, 144, 64, 0.16);
    color: var(--warning);
  }

  .empty {
    padding: 16px 12px;
    color: var(--text-secondary);
    font-size: 12px;
    line-height: 1.5;
  }

  .dialog-actions {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin-top: 14px;
  }
</style>
