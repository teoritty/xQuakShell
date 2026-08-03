<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { UserPlus } from 'lucide-svelte';
  import AuthEntryCard from '../AuthEntryCard.svelte';
  import type { ConnectionUser, PluginAuthConfig, SSHIdentityMeta } from '../../stores/appState';
  import type { AuthMethod } from './types';
  import './connectionDetailsShared.css';

  export let users: ConnectionUser[] = [];
  export let defaultUserId = '';
  export let identities: SSHIdentityMeta[] = [];

  const dispatch = createEventDispatcher<{
    dirty: void;
    userschange: ConnectionUser[];
    defaultuserchange: string;
    keyimport: string;
    keyremove: { userId: string; keyId: string };
    passwordchange: { userId: string; value: string };
    pluginauthchange: { userId: string; value: PluginAuthConfig };
  }>();

  function addUser() {
    const id = 'u-' + Date.now();
    const next = [...users, { id, username: '', authMethod: 'key' as AuthMethod }];
    // Empty user rows are local-only until the user enters a username; do not mark dirty.
    dispatch('userschange', next);
    if (next.length === 1) dispatch('defaultuserchange', id);
  }

  function removeUser(id: string) {
    const next = users.filter((u) => u.id !== id);
    dispatch('userschange', next);
    if (defaultUserId === id) dispatch('defaultuserchange', next[0]?.id || '');
    dispatch('dirty');
  }

  function updateUsername(userId: string, value: string) {
    dispatch('userschange', users.map((u) => (u.id === userId ? { ...u, username: value } : u)));
    dispatch('dirty');
  }

  function updateAuthMethod(userId: string, value: string) {
    dispatch(
      'userschange',
      users.map((u) => {
        if (u.id !== userId) return u;
        const next = { ...u, authMethod: value as AuthMethod };
        if (value === 'plugin' && !next.pluginAuth) {
          next.pluginAuth = { pluginId: '', authMethodId: '', fields: {} };
        }
        return next;
      }),
    );
    dispatch('dirty');
  }

  function updatePluginAuth(userId: string, value: PluginAuthConfig) {
    dispatch(
      'userschange',
      users.map((u) => (u.id === userId ? { ...u, pluginAuth: value } : u)),
    );
    dispatch('dirty');
  }

  function setDefaultUser(userId: string) {
    dispatch('defaultuserchange', userId);
    dispatch('dirty');
  }
</script>

<div class="connection-detail-field">
  <div class="connection-detail-section-header">
    <span class="connection-detail-field-label">Users</span>
    <button class="ghost connection-detail-micro-btn" on:click={addUser}><UserPlus size={12} /> Add</button>
  </div>
  {#each users as u (u.id)}
    <AuthEntryCard
      authMethod={u.authMethod}
      keyAuth={u.keyAuth}
      passAuth={u.passAuth}
      pluginAuth={u.pluginAuth}
      {identities}
      on:authmethodchange={(e) => updateAuthMethod(u.id, e.detail)}
      on:pluginauthchange={(e) => updatePluginAuth(u.id, e.detail)}
      on:passwordchange={(e) => dispatch('passwordchange', { userId: u.id, value: e.detail })}
      on:keyimport={() => dispatch('keyimport', u.id)}
      on:keyremove={(e) => dispatch('keyremove', { userId: u.id, keyId: e.detail })}
      on:remove={() => removeUser(u.id)}
    >
      <svelte:fragment slot="primary">
        <input
          type="text"
          value={u.username}
          on:input={(e) => updateUsername(u.id, e.currentTarget.value)}
          placeholder="username"
          class="user-input"
        />
      </svelte:fragment>
      <svelte:fragment slot="meta">
        <label class="default-radio" title="Set as default">
          <input
            type="radio"
            name="defaultUser"
            checked={defaultUserId === u.id}
            on:change={() => setDefaultUser(u.id)}
          />
          Default
        </label>
      </svelte:fragment>
    </AuthEntryCard>
  {/each}
  {#if users.length === 0}
    <div class="connection-detail-empty-state">No users configured</div>
  {/if}
</div>

<style>
  .user-input { flex: 1; font-size: 11px; min-width: 0; width: 100%; }
  /* Deliberately not shrinkable: the label must read "Default" in full or the control is
     meaningless. What is left of the row after the label is taken care of in AuthEntryCard —
     the method select yields its width first, and the toolbar wraps as a last resort — so this
     label is never squeezed into painting its text over the delete button. */
  .default-radio {
    font-size: 10px; color: var(--text-secondary); display: flex; align-items: center; gap: 2px;
    cursor: pointer; white-space: nowrap;
    flex-shrink: 0;
  }
</style>
