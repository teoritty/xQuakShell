<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { UserPlus } from 'lucide-svelte';
  import AuthEntryCard from '../AuthEntryCard.svelte';
  import type { ConnectionUser, SSHIdentityMeta } from '../../stores/appState';
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
  }>();

  function addUser() {
    const id = 'u-' + Date.now();
    const next = [...users, { id, username: '', authMethod: 'key' as AuthMethod }];
    dispatch('userschange', next);
    if (next.length === 1) dispatch('defaultuserchange', id);
    dispatch('dirty');
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
      users.map((u) =>
        u.id === userId ? { ...u, authMethod: value as AuthMethod } : u,
      ),
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
      {identities}
      on:authmethodchange={(e) => updateAuthMethod(u.id, e.detail)}
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
  .default-radio {
    font-size: 10px; color: var(--text-secondary); display: flex; align-items: center; gap: 2px;
    cursor: pointer; white-space: nowrap;
    flex-shrink: 0;
  }
  .default-radio input { margin: 0; }
</style>
