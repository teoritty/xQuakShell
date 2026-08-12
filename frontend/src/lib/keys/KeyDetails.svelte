<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { KeyUsage, StoredKey } from '../../api/keys';

  export let key: StoredKey | null = null;
  export let usages: KeyUsage[] = [];
  export let copied = false;

  const dispatch = createEventDispatcher();

  function cachePolicyLabel(k: StoredKey): string {
    if (k.policy !== 'passphrase') return 'Not applicable — the vault opens this key';
    switch (k.cachePolicy) {
      case 'never':
        return 'Ask every time';
      case 'duration':
        return `Remember for ${Math.round((k.cacheTtlSeconds || 900) / 60)} min`;
      default:
        return 'Remember until the vault locks';
    }
  }
</script>

{#if !key}
  <div class="empty">Select a key to see its details.</div>
{:else}
  <div class="details">
    <header>
      <h3>{key.comment || key.id}</h3>
      <p class="subtitle">{key.bits ? `${key.keyType} ${key.bits}` : key.keyType}</p>
    </header>

    {#if key.migrationPending}
      <p class="warning">
        This key still holds its pre-upgrade form because its passphrase was skipped. It works for
        connecting, but it cannot be exported or published until you finish the upgrade by changing
        its passphrase below.
      </p>
    {/if}

    <dl>
      <dt>Fingerprint</dt>
      <dd class="mono">{key.fingerprint || '—'}</dd>

      <dt>Protection</dt>
      <dd>{key.policy === 'passphrase' ? 'Passphrase required — the vault alone does not open it' : 'Opened by the vault'}</dd>

      <dt>Passphrase memory</dt>
      <dd>{cachePolicyLabel(key)}</dd>

      <dt>Plugins</dt>
      <dd>{key.allowPlugins ? 'May read this key' : 'Cannot read this key'}</dd>

      <dt>Export</dt>
      <dd>{key.nonExportable ? 'Sealed — this key can never leave the vault' : 'Allowed with the master password'}</dd>

      <dt>Used by</dt>
      <dd>
        {#if usages.length === 0}
          Nothing yet
        {:else}
          <ul class="usages">
            {#each usages as usage}
              <li>{usage.connectionName} · {usage.username}{usage.hop ? ` (jump via ${usage.hop})` : ''}</li>
            {/each}
          </ul>
        {/if}
      </dd>
    </dl>

    {#if key.publicKey}
      <label class="pub-label" for="public-key">Public key</label>
      <textarea id="public-key" class="mono" readonly rows="3" value={key.publicKey}></textarea>
    {/if}

    <div class="actions">
      <button on:click={() => dispatch('copy')}>{copied ? 'Copied' : 'Copy public key'}</button>
      <button on:click={() => dispatch('deploy')} disabled={!key.publicKey}>Publish to server…</button>
      <button on:click={() => dispatch('rename')}>Rename…</button>
      <button on:click={() => dispatch('passphrase')}>Change passphrase…</button>
      <button on:click={() => dispatch('policy')}>Settings…</button>
      <button on:click={() => dispatch('export')} disabled={key.nonExportable || key.migrationPending}>Export…</button>
      <button class="danger" on:click={() => dispatch('delete')}>Delete…</button>
    </div>
  </div>
{/if}

<style>
  .details {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 14px 16px;
    overflow-y: auto;
  }

  h3 {
    margin: 0;
    font-size: 15px;
  }

  .subtitle {
    margin: 2px 0 0;
    font-size: 12px;
    color: var(--text-secondary, #888);
  }

  .warning {
    margin: 0;
    padding: 8px 10px;
    border-radius: 4px;
    background: var(--warn-bg, rgba(255, 176, 0, 0.14));
    color: var(--warn-fg, #ffb000);
    font-size: 12px;
    line-height: 1.45;
  }

  dl {
    display: grid;
    grid-template-columns: minmax(120px, auto) 1fr;
    gap: 6px 14px;
    margin: 0;
    font-size: 12px;
  }

  dt {
    color: var(--text-secondary, #888);
  }

  dd {
    margin: 0;
  }

  .mono {
    font-family: var(--font-mono, monospace);
    font-size: 11px;
    word-break: break-all;
  }

  .usages {
    margin: 0;
    padding-left: 16px;
  }

  .pub-label {
    font-size: 12px;
    color: var(--text-secondary, #888);
  }

  textarea {
    width: 100%;
    resize: vertical;
    background: var(--bg-input, rgba(0, 0, 0, 0.25));
    color: var(--text-primary, #ddd);
    border: 1px solid var(--border, rgba(255, 255, 255, 0.12));
    border-radius: 4px;
    padding: 6px 8px;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 4px;
  }

  button {
    padding: 5px 10px;
    font-size: 12px;
    border-radius: 4px;
    border: 1px solid var(--border, rgba(255, 255, 255, 0.14));
    background: var(--bg-button, rgba(255, 255, 255, 0.06));
    color: var(--text-primary, #ddd);
    cursor: pointer;
  }

  button:hover:not(:disabled) {
    background: var(--bg-hover, rgba(255, 255, 255, 0.12));
  }

  button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  button.danger {
    color: var(--danger, #ff6b6b);
    border-color: var(--danger-border, rgba(255, 107, 107, 0.4));
  }

  .empty {
    padding: 20px;
    color: var(--text-secondary, #888);
    font-size: 12px;
  }
</style>
