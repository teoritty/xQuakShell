<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Copy, Check, Upload } from 'lucide-svelte';
  import type { KeyUsage, StoredKey } from '../../api/keys';

  export let key: StoredKey | null = null;
  export let usages: KeyUsage[] = [];
  export let copied = false;

  const dispatch = createEventDispatcher();

  // schema 3 recorded "openssh" or "unknown" for a key it could not see inside. That is the
  // absence of an answer rather than an algorithm, and naming it as one would suggest ed25519 and
  // "openssh" are alternatives to choose between.
  function algorithm(k: StoredKey): string {
    if (k.keyType === 'openssh' || k.keyType === 'unknown') {
      return k.migrationPending ? 'Type unknown until the upgrade is finished' : 'Type unknown';
    }
    return k.bits ? `${k.keyType} · ${k.bits} bits` : k.keyType;
  }

  function protection(k: StoredKey): string {
    return k.policy === 'passphrase'
      ? 'Passphrase required — the vault alone does not open it'
      : 'Opened by the vault';
  }

  function memory(k: StoredKey): string {
    if (k.policy !== 'passphrase') return 'Nothing to remember';
    switch (k.cachePolicy) {
      case 'never':
        return 'Asked every time';
      case 'duration':
        return `Remembered for ${Math.round((k.cacheTtlSeconds || 900) / 60)} minutes`;
      default:
        return 'Remembered until the vault locks';
    }
  }
</script>

{#if !key}
  <div class="placeholder">Select a key to see its details.</div>
{:else}
  <div class="details">
    <header>
      <div class="titles">
        <h3>{key.comment || key.id}</h3>
        <p class="subtitle">{algorithm(key)}</p>
      </div>
      <div class="lead-actions">
        <button class="secondary" on:click={() => dispatch('copy')} disabled={!key.publicKey}>
          {#if copied}<Check size={12} />{:else}<Copy size={12} />{/if}
          {copied ? 'Copied' : 'Copy public key'}
        </button>
        <button class="primary" on:click={() => dispatch('deploy')} disabled={!key.publicKey}>
          <Upload size={12} /> Publish to server
        </button>
      </div>
    </header>

    {#if key.migrationPending}
      <p class="warning">
        This key still holds its pre-upgrade form because its passphrase was skipped. It works for
        connecting, but it cannot be exported or published until you finish the upgrade — change its
        passphrase below to do that.
      </p>
    {/if}

    <section>
      <h4>Identity</h4>
      <dl>
        <dt>Fingerprint</dt>
        <dd class="mono">{key.fingerprint || 'Not derived yet'}</dd>
        <dt>Added</dt>
        <dd>{key.createdAt ? key.createdAt.slice(0, 10) : 'Unknown'}{key.source ? ` · ${key.source}` : ''}</dd>
      </dl>
      {#if key.publicKey}
        <span class="pub-label">Public key</span>
        <pre class="public-key">{key.publicKey}</pre>
      {/if}
    </section>

    <section>
      <h4>Protection</h4>
      <dl>
        <dt>Opened by</dt>
        <dd>{protection(key)}</dd>
        <dt>Passphrase</dt>
        <dd>{memory(key)}</dd>
        <dt>Plugins</dt>
        <dd>{key.allowPlugins ? 'May read this key' : 'Cannot read this key'}</dd>
        <dt>Export</dt>
        <dd>{key.nonExportable ? 'Sealed — this key can never leave the vault' : 'Allowed with the master password'}</dd>
      </dl>
    </section>

    <section>
      <h4>Used by</h4>
      {#if usages.length === 0}
        <p class="muted">No connection uses this key yet. Pick it in a connection's key authentication.</p>
      {:else}
        <ul class="usages">
          {#each usages as usage}
            <li>
              <span class="usage-name">{usage.connectionName}</span>
              <span class="muted">{usage.username}{usage.hop ? ` · jump via ${usage.hop}` : ''}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  </div>

  <footer class="tail-actions">
    <div class="group">
      <button class="secondary" on:click={() => dispatch('rename')}>Rename</button>
      <button class="secondary" on:click={() => dispatch('passphrase')}>Change passphrase</button>
      <button class="secondary" on:click={() => dispatch('policy')}>Settings</button>
      <button class="secondary" on:click={() => dispatch('export')} disabled={key.nonExportable || key.migrationPending}>
        Export
      </button>
    </div>
    <button class="danger" on:click={() => dispatch('delete')}>Delete</button>
  </footer>
{/if}

<style>
  .details {
    display: flex;
    flex-direction: column;
    gap: 20px;
    padding: 16px 18px;
    overflow-y: auto;
    flex: 1;
    min-height: 0;
  }

  header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    flex-wrap: wrap;
  }

  .titles {
    min-width: 0;
  }

  h3 {
    margin: 0;
    font-size: 16px;
    font-weight: 500;
    color: var(--text-bright);
    word-break: break-word;
  }

  .subtitle {
    margin: 3px 0 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .lead-actions {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
  }

  .lead-actions button {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 4px 10px;
  }

  .warning {
    margin: 0;
    padding: 9px 11px;
    border-radius: 3px;
    background: rgba(196, 144, 64, 0.16);
    color: var(--warning);
    font-size: 12px;
    line-height: 1.5;
  }

  section {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  h4 {
    margin: 0;
    font-size: 10px;
    font-weight: 600;
    letter-spacing: 0.8px;
    text-transform: uppercase;
    color: var(--text-secondary);
    padding-bottom: 6px;
    border-bottom: 1px solid var(--border-color);
  }

  dl {
    display: grid;
    grid-template-columns: 116px 1fr;
    gap: 8px 16px;
    margin: 0;
    font-size: 12px;
  }

  dt {
    color: var(--text-secondary);
  }

  dd {
    margin: 0;
    color: var(--text-primary);
    line-height: 1.45;
  }

  .mono {
    font-family: var(--font-mono);
    font-size: 11px;
    word-break: break-all;
  }

  .pub-label {
    margin-top: 4px;
    font-size: 12px;
    color: var(--text-secondary);
  }

  /* Read-only, so it is presented as a value and not as a field: a textarea invites editing that
     does nothing, and its resize handle promises a control this is not. */
  .public-key {
    margin: 0;
    padding: 8px 10px;
    border: 1px solid var(--border-color);
    border-radius: 3px;
    background: var(--bg-primary);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 11px;
    line-height: 1.55;
    white-space: pre-wrap;
    word-break: break-all;
    user-select: all;
  }

  .usages {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 12px;
  }

  .usages li {
    display: flex;
    gap: 10px;
    align-items: baseline;
  }

  .usage-name {
    color: var(--text-primary);
  }

  .muted {
    margin: 0;
    color: var(--text-secondary);
    font-size: 12px;
    line-height: 1.5;
  }

  .tail-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-shrink: 0;
    padding: 10px 18px;
    border-top: 1px solid var(--border-color);
    background: var(--bg-primary);
  }

  .group {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .placeholder {
    padding: 28px 20px;
    color: var(--text-secondary);
    font-size: 12px;
  }
</style>
