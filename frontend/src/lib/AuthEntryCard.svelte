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

  /* wrap is the last-resort guard: if the row genuinely cannot fit even after the select has
     given up all the width it can, the meta slot moves to its own line instead of its content
     being painted over the delete button. */
  .auth-entry-toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px;
    margin-bottom: 4px;
    min-width: 0;
  }

  /* 88px is a preferred width, not a floor: this is the element that yields when the panel is
     narrow. It holds one short word ("Key", "Password", "Plugin") and stays legible when
     squeezed, whereas the meta slot next to it holds a label that must be read in full. */
  .auth-select {
    width: 88px;
    font-size: 11px;
    flex: 0 1 auto;
    min-width: 0;
  }

  /* flex-basis auto, not 0: the slot's starting width is its content, so the label it holds is
     never squeezed out. An earlier attempt clipped this slot instead, which did stop the overlap
     but hid the label entirely once the slot fell below the label's width — the row has to be
     made to fit, not have its overflow hidden. */
  .auth-entry-meta {
    display: flex;
    align-items: center;
    gap: 4px;
    flex: 1 1 auto;
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
