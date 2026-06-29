<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { KeyRound, Plus, X, Trash2 } from 'lucide-svelte';
  import type { KeyAuthConfig, PassAuthConfig, SSHIdentityMeta } from '../stores/appState';

  export let authMethod: 'key' | 'password';
  export let keyAuth: KeyAuthConfig | undefined = undefined;
  export let passAuth: PassAuthConfig | undefined = undefined;
  export let identities: SSHIdentityMeta[] = [];

  const dispatch = createEventDispatcher<{
    authmethodchange: string;
    passwordchange: string;
    keyimport: void;
    keyremove: string;
    remove: void;
  }>();
</script>

<div class="auth-entry-card">
  <div class="auth-entry-identity">
    <slot name="primary" />
  </div>
  <div class="auth-entry-toolbar">
    <select
      value={authMethod}
      on:change={(e) => dispatch('authmethodchange', e.currentTarget.value)}
      class="auth-select"
    >
      <option value="key">Key</option>
      <option value="password">Password</option>
    </select>
    <div class="auth-entry-meta">
      <slot name="meta" />
    </div>
    <button class="ghost micro-btn danger toolbar-remove" on:click={() => dispatch('remove')} title="Remove">
      <Trash2 size={12} />
    </button>
  </div>
  {#if authMethod === 'password'}
    <div class="pass-block">
      <input
        type="password"
        placeholder="Enter password"
        value={passAuth?.passwordId ? '********' : ''}
        on:change={(e) => dispatch('passwordchange', e.currentTarget.value)}
        class="pass-input"
      />
    </div>
  {:else if authMethod === 'key'}
    <div class="keys-list">
      {#each (keyAuth?.identityIds || []) as keyId}
        {@const meta = identities.find(i => i.id === keyId)}
        <div class="key-item">
          <KeyRound size={11} />
          <span class="key-name">{meta?.comment || keyId.slice(0, 8)}</span>
          <button class="ghost key-remove" on:click={() => dispatch('keyremove', keyId)}>
            <X size={10} />
          </button>
        </div>
      {/each}
      <button class="secondary tiny-btn" on:click={() => dispatch('keyimport')}>
        <Plus size={11} /> Import Key
      </button>
    </div>
  {/if}
</div>

<style>
  .auth-entry-card {
    padding: 6px;
    background: var(--bg-tertiary);
    border-radius: 2px;
    margin-bottom: 4px;
  }

  .auth-entry-identity {
    display: flex;
    align-items: center;
    gap: 4px;
    margin-bottom: 4px;
    min-width: 0;
  }

  .auth-entry-identity :global(*) {
    min-width: 0;
  }

  .auth-entry-toolbar {
    display: flex;
    align-items: center;
    gap: 4px;
    margin-bottom: 4px;
    min-width: 0;
  }

  .auth-select {
    width: 80px;
    font-size: 11px;
    flex-shrink: 0;
  }

  .auth-entry-meta {
    display: flex;
    align-items: center;
    gap: 4px;
    flex: 1;
    min-width: 0;
    justify-content: flex-end;
  }

  .toolbar-remove {
    flex-shrink: 0;
  }

  .keys-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .key-item {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 10px;
    padding: 2px 4px;
    background: var(--bg-secondary);
    border-radius: 2px;
    color: var(--text-secondary);
  }

  .key-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .key-remove {
    padding: 0 2px;
    display: inline-flex;
    align-items: center;
  }

  .pass-block {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .pass-input {
    flex: 1;
    font-size: 11px;
  }

  .tiny-btn {
    font-size: 10px;
    padding: 2px 6px;
    display: inline-flex;
    align-items: center;
    gap: 3px;
  }

  .micro-btn {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 11px;
    padding: 1px 6px;
  }
</style>
