<script lang="ts">
  import { selectedConnection, identities, selectedConnectionId, type ConnectionUser, type JumpHop, type ProxyConfig } from '../stores/appState';
  import { saveConnection, importIdentity, importPassword } from '../stores/api';
  import { UserPlus, Trash2, KeyRound, Plus, X } from 'lucide-svelte';

  let editingId = '';
  let name = '';
  let host = '';
  let port = 22;
  let tags: string[] = [];
  let users: ConnectionUser[] = [];
  let defaultUserId = '';
  let jumpHops: JumpHop[] = [];
  let proxyEnabled = false;
  let proxyHost = '';
  let proxyPort = 1080;
  let proxyUsername = '';
  let proxyPasswordId = '';
  let dirty = false;
  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  let saveStatus: 'idle' | 'saving' | 'saved' = 'idle';
  let addingTag = false;
  let newTagValue = '';

  $: connId = $selectedConnection?.id || '';

  $: if (connId !== editingId) {
    loadFromStore();
  }

  async function loadFromStore() {
    const c = $selectedConnection;
    editingId = c?.id || '';
    name = c?.name || '';
    host = c?.host || '';
    port = c?.port || 22;
    tags = [...(c?.tags || [])];
    users = (c?.users || []).map(u => ({...u}));
    defaultUserId = c?.defaultUserId || '';
    jumpHops = (c?.jumpChain || []).map(h => ({...h}));
    proxyEnabled = !!(c?.proxy && (c.proxy.host || c.proxy.port));
    proxyHost = c?.proxy?.host || '';
    proxyPort = c?.proxy?.port || 1080;
    proxyUsername = c?.proxy?.username || '';
    proxyPasswordId = c?.proxy?.passwordId || '';
    dirty = false;
    saveStatus = 'idle';
    addingTag = false;
    newTagValue = '';
    if (saveTimer) { clearTimeout(saveTimer); saveTimer = null; }
  }

  function markDirty() {
    dirty = true;
    saveStatus = 'idle';
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => autoSave(), 600);
  }

  async function autoSave() {
    if (!editingId || !dirty) return;
    saveStatus = 'saving';
    const filteredHops = jumpHops.filter(h => h.host.trim() !== '');
    // Keep draft users: do not drop a row just because username is still empty while password/keys are set.
    let filteredUsers = users.filter(
      u =>
        u.username.trim() !== '' ||
        (u.authMethod === 'password' && u.passAuth?.passwordId) ||
        (u.keyAuth?.identityIds && u.keyAuth.identityIds.length > 0)
    );
    if (filteredUsers.length === 0 && users.length > 0) {
      filteredUsers = [...users];
    }
    const conn: any = {
      id: editingId,
      name: name.trim() || 'New connection',
      protocol: 'ssh',
      host: host.trim(),
      port,
      folderId: $selectedConnection?.folderId || '',
      tags,
      users: filteredUsers,
      defaultUserId,
      jumpChain: filteredHops,
      order: $selectedConnection?.order ?? 0,
    };
    if (proxyEnabled && proxyHost.trim()) {
      conn.proxy = { type: 'socks5', host: proxyHost.trim(), port: proxyPort, username: proxyUsername.trim() || undefined };
      if (proxyPasswordId) conn.proxy.passwordId = proxyPasswordId;
    }
    try {
      await saveConnection(conn);
      dirty = false;
      saveStatus = 'saved';
    } catch (e) {
      console.error('autoSave', e);
      saveStatus = 'idle';
    }
    setTimeout(() => { if (saveStatus === 'saved') saveStatus = 'idle'; }, 1500);
  }

  // --- Tags ---
  function startAddTag() { addingTag = true; newTagValue = ''; }
  function confirmTag() {
    const t = newTagValue.trim();
    if (t && !tags.includes(t)) { tags = [...tags, t]; markDirty(); }
    addingTag = false;
    newTagValue = '';
  }
  function cancelTag() { addingTag = false; newTagValue = ''; }
  function removeTag(t: string) { tags = tags.filter(x => x !== t); markDirty(); }
  function tagColor(tag: string): string {
    let hash = 0;
    for (let i = 0; i < tag.length; i++) hash = tag.charCodeAt(i) + ((hash << 5) - hash);
    return `hsl(${Math.abs(hash) % 360}, 50%, 40%)`;
  }

  // --- Users ---
  function addUser() {
    const id = 'u-' + Date.now();
    users = [...users, { id, username: '', authMethod: 'key' }];
    if (users.length === 1) defaultUserId = id;
    markDirty();
  }

  function removeUser(id: string) {
    users = users.filter(u => u.id !== id);
    if (defaultUserId === id) defaultUserId = users[0]?.id || '';
    markDirty();
  }

  function updateUsername(userId: string, value: string) {
    users = users.map(u => u.id === userId ? { ...u, username: value } : u);
    markDirty();
  }

  function updateAuthMethod(userId: string, value: string) {
    users = users.map(u => u.id === userId ? { ...u, authMethod: value as 'key' | 'password' } : u);
    markDirty();
  }

  function setDefaultUser(userId: string) {
    defaultUserId = userId;
    markDirty();
  }

  async function handleKeyImportForUser(userId: string) {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.pem,.key,.id_rsa,.id_ecdsa,.id_ed25519,*';
    input.onchange = async () => {
      const file = input.files?.[0];
      if (!file) return;
      const text = await file.text();
      const base64 = btoa(text);
      const kid = await importIdentity(base64, file.name);
      if (kid) {
        users = users.map(u => {
          if (u.id !== userId) return u;
          const ids = u.keyAuth?.identityIds || [];
          return { ...u, keyAuth: { identityIds: [...ids, kid] } };
        });
        markDirty();
      }
    };
    input.click();
  }

  function removeKeyFromUser(userId: string, keyId: string) {
    users = users.map(u => {
      if (u.id !== userId) return u;
      const ids = (u.keyAuth?.identityIds || []).filter(i => i !== keyId);
      return { ...u, keyAuth: { identityIds: ids } };
    });
    markDirty();
  }

  async function handlePasswordChange(userId: string, value: string) {
    if (!value || value === '********') return;
    const pwId = await importPassword(value, `user-${userId}`);
    if (pwId) {
      users = users.map(u => {
        if (u.id !== userId) return u;
        return { ...u, passAuth: { passwordId: pwId } };
      });
      markDirty();
    }
  }

  async function handleProxyPasswordChange(value: string) {
    if (!value || value === '********') return;
    const pwId = await importPassword(value, 'proxy');
    if (pwId) {
      proxyPasswordId = pwId;
      markDirty();
    }
  }

  // --- Jump Hops ---
  function addHop() {
    jumpHops = [...jumpHops, { host: '', port: 22, username: '', authMethod: 'key' }];
    markDirty();
  }

  function removeHop(idx: number) {
    jumpHops = jumpHops.filter((_, i) => i !== idx);
    markDirty();
  }

  function updateHopField(idx: number, field: string, value: any) {
    jumpHops = jumpHops.map((h, i) => i === idx ? { ...h, [field]: value } : h);
    markDirty();
  }
</script>

{#if $selectedConnection}
<div class="connection-details">
  <div class="panel-header">
    <span>Connection</span>
    <span class="save-indicator">
      {#if saveStatus === 'saving'}Saving...{:else if saveStatus === 'saved'}Saved{/if}
    </span>
  </div>

  <div class="details-body">
    <label class="field">
      <span class="field-label">Name</span>
      <input type="text" bind:value={name} on:input={markDirty} placeholder="My Server" />
    </label>

    <div class="field-row">
      <label class="field" style="flex:1">
        <span class="field-label">Host</span>
        <input type="text" bind:value={host} on:input={markDirty} placeholder="192.168.1.1" />
      </label>
      <label class="field" style="width: calc(60px * var(--ui-scale))">
        <span class="field-label">Port</span>
        <input type="number" bind:value={port} on:input={markDirty} min="1" max="65535" />
      </label>
    </div>

    <!-- Tags -->
    <div class="field">
      <div class="section-header">
        <span class="field-label">Tags</span>
        <button class="ghost micro-btn" on:click={startAddTag}><Plus size={12} /> Tag</button>
      </div>
      <div class="tags-row">
        {#each tags as tag}
          <span class="tag-chip" style="background: {tagColor(tag)}">
            {tag}
            <button class="tag-remove" on:click={() => removeTag(tag)}><X size={9} /></button>
          </span>
        {/each}
        {#if addingTag}
          <input
            class="tag-inline-input"
            placeholder="tag name..."
            bind:value={newTagValue}
            on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); confirmTag(); } if (e.key === 'Escape') cancelTag(); }}
            on:blur={confirmTag}
          />
        {/if}
        {#if tags.length === 0 && !addingTag}
          <span class="no-items">No tags</span>
        {/if}
      </div>
    </div>

    <!-- Users -->
    <div class="field">
      <div class="section-header">
        <span class="field-label">Users</span>
        <button class="ghost micro-btn" on:click={addUser}><UserPlus size={12} /> Add</button>
      </div>
      {#each users as u (u.id)}
        <div class="user-block">
          <div class="user-header">
            <input
              type="text"
              value={u.username}
              on:input={(e) => updateUsername(u.id, e.currentTarget.value)}
              placeholder="username"
              class="user-input"
            />
            <select
              value={u.authMethod}
              on:change={(e) => updateAuthMethod(u.id, e.currentTarget.value)}
              class="auth-select"
            >
              <option value="key">Key</option>
              <option value="password">Password</option>
            </select>
            <label class="default-radio" title="Set as default">
              <input
                type="radio"
                name="defaultUser"
                checked={defaultUserId === u.id}
                on:change={() => setDefaultUser(u.id)}
              />
              Default
            </label>
            <button class="ghost micro-btn danger" on:click={() => removeUser(u.id)} title="Remove user"><Trash2 size={12} /></button>
          </div>
          {#if u.authMethod === 'password'}
            <div class="pass-block">
              <input
                type="password"
                placeholder="Enter password"
                value={u.passAuth?.passwordId ? '********' : ''}
                on:change={(e) => handlePasswordChange(u.id, e.currentTarget.value)}
                class="pass-input"
              />
            </div>
          {:else if u.authMethod === 'key'}
            <div class="keys-list">
              {#each (u.keyAuth?.identityIds || []) as keyId}
                {@const meta = $identities.find(i => i.id === keyId)}
                <div class="key-item">
                  <KeyRound size={11} />
                  <span class="key-name">{meta?.comment || keyId.slice(0, 8)}</span>
                  <button class="ghost key-remove" on:click={() => removeKeyFromUser(u.id, keyId)}><X size={10} /></button>
                </div>
              {/each}
              <button class="secondary tiny-btn" on:click={() => handleKeyImportForUser(u.id)}>
                <Plus size={11} /> Import Key
              </button>
            </div>
          {/if}
        </div>
      {/each}
      {#if users.length === 0}
        <div class="no-items">No users configured</div>
      {/if}
    </div>

    <!-- Jump Chain -->
    <div class="field">
      <div class="section-header">
        <span class="field-label">Jump Hosts (Bastion)</span>
        <button class="ghost micro-btn" on:click={addHop}><Plus size={12} /> Hop</button>
      </div>
      {#each jumpHops as hop, idx}
        <div class="hop-block">
          <div class="hop-row">
            <input
              type="text"
              value={hop.host}
              on:input={(e) => updateHopField(idx, 'host', e.currentTarget.value)}
              placeholder="bastion-host"
              class="hop-input"
            />
            <input
              type="number"
              value={hop.port}
              on:input={(e) => updateHopField(idx, 'port', parseInt(e.currentTarget.value) || 22)}
              min="1" max="65535" class="hop-port"
            />
            <input
              type="text"
              value={hop.username}
              on:input={(e) => updateHopField(idx, 'username', e.currentTarget.value)}
              placeholder="user"
              class="hop-input"
            />
            <select
              value={hop.authMethod}
              on:change={(e) => updateHopField(idx, 'authMethod', e.currentTarget.value)}
              class="hop-select"
            >
              <option value="key">Key</option>
              <option value="password">Pass</option>
            </select>
            <button class="ghost micro-btn danger" on:click={() => removeHop(idx)}><X size={12} /></button>
          </div>
        </div>
      {/each}
    </div>

    <!-- Proxy (SOCKS5) -->
    <div class="field">
      <div class="section-header">
        <span class="field-label">SOCKS Proxy</span>
        <button class="ghost micro-btn" on:click={() => { proxyEnabled = !proxyEnabled; markDirty(); }}>
          {proxyEnabled ? 'Disable' : 'Enable'}
        </button>
      </div>
      {#if proxyEnabled}
        <div class="proxy-block">
          <div class="field-row">
            <label class="field" style="flex:1">
              <span class="field-label">Host</span>
              <input type="text" bind:value={proxyHost} on:input={markDirty} placeholder="proxy.example.com" />
            </label>
            <label class="field" style="width: calc(70px * var(--ui-scale))">
              <span class="field-label">Port</span>
              <input type="number" bind:value={proxyPort} on:input={markDirty} min="1" max="65535" />
            </label>
          </div>
          <label class="field">
            <span class="field-label">Username (optional)</span>
            <input type="text" bind:value={proxyUsername} on:input={markDirty} placeholder="proxy user" />
          </label>
          <label class="field">
            <span class="field-label">Password (optional)</span>
            <input
              type="password"
              placeholder={proxyPasswordId ? '********' : 'Enter password'}
              value={proxyPasswordId ? '' : ''}
              on:change={(e) => handleProxyPasswordChange(e.currentTarget.value)}
              class="pass-input"
            />
          </label>
        </div>
      {/if}
    </div>

  </div>
</div>
{/if}

<style>
  .connection-details {
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    max-height: 55vh;
    border-top: 1px solid var(--border-color);
  }

  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .save-indicator {
    font-size: 10px;
    color: var(--text-secondary);
    font-style: italic;
  }

  .details-body {
    padding: 8px 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    overflow-y: auto;
  }

  .field { display: flex; flex-direction: column; gap: 2px; }
  .field-label { font-size: 11px; color: var(--text-secondary); font-weight: 500; }
  .field input, .field select { width: 100%; }
  .field-row { display: flex; gap: 8px; }

  .section-header {
    display: flex; justify-content: space-between; align-items: center;
    margin-bottom: 4px;
  }

  .micro-btn {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 11px;
    padding: 1px 6px;
  }

  .no-items {
    font-size: 11px;
    color: var(--text-secondary);
    padding: 4px 0;
  }

  /* Tags */
  .tags-row {
    display: flex; flex-wrap: wrap; gap: 4px; align-items: center;
    min-height: 24px;
  }
  .tag-chip {
    display: inline-flex; align-items: center; gap: 3px;
    font-size: 10px; padding: 1px 6px; border-radius: 2px; color: #fff;
  }
  .tag-remove {
    background: none; border: none; color: rgba(255,255,255,0.7);
    cursor: pointer; padding: 0 1px; display: inline-flex; align-items: center;
  }
  .tag-remove:hover { color: #fff; }
  .tag-inline-input {
    width: 80px; font-size: 11px; padding: 2px 4px;
    background: var(--bg-input); border: 1px solid var(--border-focus);
    color: var(--text-primary); border-radius: 2px; outline: none;
  }

  /* Users */
  .user-block {
    padding: 6px; background: var(--bg-tertiary); border-radius: 2px;
    margin-bottom: 4px;
  }
  .user-header {
    display: flex; align-items: center; gap: 4px; margin-bottom: 4px;
  }
  .user-input { flex: 1; font-size: 11px; min-width: 60px; }
  .auth-select { width: 80px; font-size: 11px; }
  .default-radio {
    font-size: 10px; color: var(--text-secondary); display: flex; align-items: center; gap: 2px;
    cursor: pointer; white-space: nowrap;
  }
  .default-radio input { margin: 0; }

  .keys-list { display: flex; flex-direction: column; gap: 2px; }
  .key-item {
    display: flex; align-items: center; gap: 4px; font-size: 10px;
    padding: 2px 4px; background: var(--bg-secondary); border-radius: 2px;
    color: var(--text-secondary);
  }
  .key-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .key-remove {
    padding: 0 2px; display: inline-flex; align-items: center;
  }

  .pass-block {
    display: flex; align-items: center; gap: 6px;
  }
  .pass-input {
    flex: 1; font-size: 11px;
  }
  .tiny-btn {
    font-size: 10px; padding: 2px 6px;
    display: inline-flex; align-items: center; gap: 3px;
  }

  /* Jump hops */
  .hop-block { margin-bottom: 4px; }
  .hop-row {
    display: flex; align-items: center; gap: 4px;
    padding: 4px; background: var(--bg-tertiary); border-radius: 2px;
  }
  .hop-input { flex: 1; font-size: 11px; min-width: 60px; }
  .hop-port { width: 55px; font-size: 11px; }
  .hop-select { width: 55px; font-size: 11px; }

  /* Proxy */
  .proxy-block { padding: 4px; background: var(--bg-tertiary); border-radius: 2px; }

</style>
