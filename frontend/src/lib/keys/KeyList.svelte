<script lang="ts">
  import { t } from '../../i18n/messages';
  import type { StoredKey } from '../../api/keys';

  export let keys: StoredKey[] = [];
  export let selectedId = '';
  export let filter = '';

  // The fingerprint is searchable alongside the label because it is what a server shows you, and
  // matching a key to a log line is the main reason anyone searches this list.
  $: visible = keys.filter((key) => {
    const needle = filter.trim().toLowerCase();
    if (!needle) return true;
    return (
      key.comment.toLowerCase().includes(needle) ||
      key.keyType.toLowerCase().includes(needle) ||
      (key.fingerprint || '').toLowerCase().includes(needle)
    );
  });

  function algorithm(key: StoredKey): string {
    if (key.keyType === 'openssh' || key.keyType === 'unknown') return 'type unknown';
    return key.bits ? `${key.keyType} ${key.bits}` : key.keyType;
  }

  // The tail of a fingerprint is what distinguishes two keys at a glance; the SHA256: prefix is
  // the same on every one of them and only costs width in a narrow rail.
  function shortFingerprint(key: StoredKey): string {
    const raw = (key.fingerprint || '').replace(/^SHA256:/, '');
    return raw ? `…${raw.slice(-12)}` : 'no fingerprint';
  }
</script>

<ul class="key-list" role="listbox" aria-label="SSH keys">
  {#each visible as key (key.id)}
    <li>
      <button
        class="key-row"
        class:selected={key.id === selectedId}
        role="option"
        aria-selected={key.id === selectedId}
        on:click={() => (selectedId = key.id)}
      >
        <span class="line-top">
          <span class="key-name">{key.comment || key.id}</span>
          {#if key.migrationPending}
            <span class="badge warn" title={$t('keys.badge.unfinished.title')}>{$t('keys.badge.unfinished')}</span>
          {:else if key.policy === 'passphrase'}
            <span class="badge" title={$t('keys.badge.passphrase.title')}>{$t('keys.badge.passphrase')}</span>
          {/if}
          {#if key.nonExportable}<span class="badge" title={$t('security.keys.badge.sealed.title')}>{$t('security.keys.badge.sealed')}</span>{/if}
        </span>
        <span class="line-bottom">
          <span class="algorithm">{algorithm(key)}</span>
          <span class="fingerprint">{shortFingerprint(key)}</span>
        </span>
      </button>
    </li>
  {:else}
    <li class="empty">{filter ? $t('keys.empty.filtered') : $t('keys.empty')}</li>
  {/each}
</ul>

<style>
  .key-list {
    list-style: none;
    margin: 0;
    padding: 0;
    overflow-y: auto;
    flex: 1;
    min-height: 0;
  }

  .key-row {
    display: flex;
    flex-direction: column;
    gap: 4px;
    width: 100%;
    padding: 9px 12px 9px 10px;
    background: transparent;
    border: none;
    border-left: 2px solid transparent;
    border-radius: 0;
    border-bottom: 1px solid var(--border-color);
    color: var(--text-primary);
    text-align: left;
  }

  .key-row:hover {
    background: var(--bg-hover);
    border-color: transparent;
    border-bottom-color: var(--border-color);
  }

  .key-row.selected {
    background: var(--bg-active);
    border-left-color: var(--accent);
  }

  .line-top {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  .key-name {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    color: var(--text-bright);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .line-bottom {
    display: flex;
    align-items: baseline;
    gap: 8px;
    font-size: 11px;
    color: var(--text-secondary);
    min-width: 0;
  }

  .algorithm {
    flex-shrink: 0;
  }

  .fingerprint {
    font-family: var(--font-mono);
    font-size: 10px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .badge {
    flex-shrink: 0;
    padding: 1px 5px;
    border-radius: 2px;
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    font-size: 10px;
  }

  .badge.warn {
    background: rgba(196, 144, 64, 0.16);
    color: var(--warning);
  }

  .empty {
    padding: 16px 12px;
    color: var(--text-secondary);
    font-size: 12px;
    line-height: 1.5;
  }
</style>
