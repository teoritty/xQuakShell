<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import { KeyRound, Plus, X, Trash2 } from 'lucide-svelte';
  import type { KeyAuthConfig, PassAuthConfig, PluginAuthConfig } from '../stores/appState';
  import type { PluginAuthMethodContribution } from '../api/pluginRuntime';
  import { getPluginContributions } from '../api/pluginRuntime';
  import PluginConnectionFields from './connectionDetails/PluginConnectionFields.svelte';

  export let authMethod: 'key' | 'password' | 'plugin';
  export let keyAuth: KeyAuthConfig | undefined = undefined;
  export let passAuth: PassAuthConfig | undefined = undefined;
  export let pluginAuth: PluginAuthConfig | undefined = undefined;
  export let identities: { id: string; comment: string; keyType: string }[] = [];

  const dispatch = createEventDispatcher<{
    authmethodchange: string;
    passwordchange: string;
    keyimport: void;
    keyremove: string;
    pluginauthchange: PluginAuthConfig;
    remove: void;
  }>();

  let authMethods: PluginAuthMethodContribution[] = [];

  onMount(async () => {
    const contrib = await getPluginContributions();
    authMethods = contrib.authMethods || [];
  });

  $: pluginIds = [...new Set(authMethods.map((m) => m.pluginId))];
  $: methodsForPlugin = authMethods.filter((m) => m.pluginId === (pluginAuth?.pluginId || ''));
  $: selectedMethod = authMethods.find(
    (m) => m.pluginId === pluginAuth?.pluginId && m.id === pluginAuth?.authMethodId,
  );

  function onPluginChange(pluginId: string) {
    const next: PluginAuthConfig = { pluginId, authMethodId: '', fields: {} };
    dispatch('pluginauthchange', next);
  }

  function onAuthMethodChange(authMethodId: string) {
    const method = authMethods.find((m) => m.pluginId === pluginAuth?.pluginId && m.id === authMethodId);
    const fields: Record<string, string> = {};
    for (const group of method?.fields || []) {
      for (const field of group.fields || []) {
        if (field.default !== undefined && field.default !== null) {
          fields[field.id] = String(field.default);
        }
      }
    }
    dispatch('pluginauthchange', {
      pluginId: pluginAuth?.pluginId || '',
      authMethodId,
      fields,
    });
  }

  function onFieldChange(fieldId: string, value: unknown) {
    if (!pluginAuth) return;
    const fields = { ...(pluginAuth.fields || {}), [fieldId]: String(value ?? '') };
    dispatch('pluginauthchange', { ...pluginAuth, fields });
  }
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
      <option value="plugin">Plugin</option>
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
  {:else if authMethod === 'plugin'}
    <div class="plugin-auth-block">
      <select
        value={pluginAuth?.pluginId || ''}
        on:change={(e) => onPluginChange(e.currentTarget.value)}
        class="plugin-select"
      >
        <option value="">Select plugin…</option>
        {#each pluginIds as pid}
          <option value={pid}>{pid}</option>
        {/each}
      </select>
      <select
        value={pluginAuth?.authMethodId || ''}
        on:change={(e) => onAuthMethodChange(e.currentTarget.value)}
        class="plugin-select"
        disabled={!pluginAuth?.pluginId}
      >
        <option value="">Auth method…</option>
        {#each methodsForPlugin as method}
          <option value={method.id}>{method.label || method.id}</option>
        {/each}
      </select>
      <p class="consent-hint">Plugin auth requires install-time auth provider consent in plugin settings.</p>
      {#if selectedMethod?.fields?.length}
        <PluginConnectionFields
          groups={selectedMethod.fields}
          values={pluginAuth?.fields || {}}
          errors={{}}
          on:fieldchange={(e) => onFieldChange(e.detail.fieldId, e.detail.value)}
        />
      {/if}
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
    width: 88px;
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

  .plugin-auth-block {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .plugin-select {
    font-size: 11px;
  }

  .consent-hint {
    font-size: 9px;
    color: var(--text-secondary);
    margin: 0;
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
