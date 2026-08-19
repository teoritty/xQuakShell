<script lang="ts">
  // One plugin row, in both lists. It renders and dispatches; every decision about what an action
  // means belongs to the section that owns the data.
  import { createEventDispatcher } from 'svelte';
  import { ShieldCheck, ShieldAlert, Info } from 'lucide-svelte';

  export let name: string;
  export let version = '';
  export let description = '';
  /** Where it came from, shown under the name: a repository URL or a source display name. */
  export let origin = '';
  export let signed: boolean | null = null;
  /** Right-hand status text, e.g. "Installed 1.2.0" or "Not installed". */
  export let status = '';
  export let statusKind: 'installed' | 'not-installed' | 'warning' = 'not-installed';
  export let disabled = false;
  export let showDetails = false;

  const dispatch = createEventDispatcher();
</script>

<div class="plugin-card" class:disabled>
  <div class="card-main">
    <div class="card-head">
      <span class="card-name">{name}</span>
      {#if version}<span class="card-version">{version}</span>{/if}
      {#if signed !== null}
        <span class="card-signed" class:unsigned={!signed} title={signed ? 'Signed by a trusted publisher' : 'Not signed'}>
          {#if signed}<ShieldCheck size={12} />{:else}<ShieldAlert size={12} />{/if}
        </span>
      {/if}
    </div>
    {#if description}<div class="card-desc">{description}</div>{/if}
    {#if origin}<div class="card-origin">{origin}</div>{/if}
  </div>

  <div class="card-side">
    {#if status}
      <span class="card-status" class:installed={statusKind === 'installed'} class:warning={statusKind === 'warning'}>
        {status}
      </span>
    {/if}
    <div class="card-actions">
      {#if showDetails}
        <button class="ghost icon-btn" title="Details" on:click={() => dispatch('details')}>
          <Info size={13} />
        </button>
      {/if}
      <slot name="actions" />
    </div>
  </div>
</div>

<style>
  .plugin-card {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-secondary);
  }

  .plugin-card.disabled {
    opacity: 0.55;
  }

  .card-main {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .card-head {
    display: flex;
    align-items: center;
    gap: 7px;
  }

  .card-name {
    font-weight: 600;
    font-size: 13px;
  }

  .card-version {
    font-size: 11px;
    color: var(--text-secondary);
  }

  .card-signed {
    display: inline-flex;
    color: var(--success, #3fb950);
  }

  .card-signed.unsigned {
    color: var(--warning, #d29922);
  }

  .card-desc {
    font-size: 12px;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .card-origin {
    font-size: 11px;
    color: var(--text-tertiary, var(--text-secondary));
    overflow-wrap: anywhere;
  }

  .card-side {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 6px;
    flex-shrink: 0;
  }

  .card-status {
    font-size: 11px;
    color: var(--text-secondary);
    white-space: nowrap;
  }

  .card-status.installed {
    color: var(--success, #3fb950);
  }

  .card-status.warning {
    color: var(--warning, #d29922);
  }

  .card-actions {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    padding: 3px;
  }
</style>
