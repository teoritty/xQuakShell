<script lang="ts">
  import { onMount } from 'svelte';
  import { detailsConnection, detailsConnectionId, identities, type ConnectionUser, type JumpHop } from '../stores/appState';
  import { saveConnection, importIdentity, importPassword, getPluginConnectionProtocols, type ConnectionProtocol } from '../stores/api';
  import { UserPlus, Plus, X, ChevronUp, ChevronDown } from 'lucide-svelte';
  import AuthEntryCard from './AuthEntryCard.svelte';

  let editingId = '';
  let name = '';
  let protocol = 'ssh';
  let host = '';
  let port = 22;
  let protocols: ConnectionProtocol[] = [{ id: 'ssh', label: 'SSH', defaultPort: 22, icon: 'terminal' }];
  let tags: string[] = [];
  let users: ConnectionUser[] = [];
  let defaultUserId = '';
  let jumpHops: JumpHop[] = [];
  let dirty = false;
  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  let saveStatus: 'idle' | 'saving' | 'saved' = 'idle';
  let addingTag = false;
  let newTagValue = '';

  $: connId = $detailsConnection?.id || '';
  $: isSSH = protocol === 'ssh';
  $: tagTooLong = newTagValue.trim().length > 30;

  onMount(async () => {
    protocols = await getPluginConnectionProtocols();
  });

  $: if (connId !== editingId) {
    loadFromStore();
  }

  function ensureHopId(hop: JumpHop, index: number): JumpHop {
    const withAuth = {
      ...hop,
      authMethod: hop.authMethod || 'key',
    };
    if (withAuth.id) return withAuth;
    return { ...withAuth, id: `h-legacy-${Date.now()}-${index}` };
  }

  async function loadFromStore() {
    const c = $detailsConnection;
    editingId = c?.id || '';
    name = c?.name || '';
    protocol = c?.protocol || 'ssh';
    host = c?.host || '';
    port = c?.port || protocols.find(p => p.id === (c?.protocol || 'ssh'))?.defaultPort || 22;
    tags = [...(c?.tags || [])];
    users = (c?.users || []).map(u => ({...u}));
    defaultUserId = c?.defaultUserId || '';
    jumpHops = (c?.jumpChain || []).map((h, i) => ensureHopId({ ...h }, i));
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
      protocol,
      host: host.trim(),
      port,
      folderId: $detailsConnection?.folderId || '',
      tags,
      users: filteredUsers,
      defaultUserId,
      jumpChain: filteredHops,
      order: $detailsConnection?.order ?? 0,
    };
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
    if (t.length > 30) return;
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

  // --- Jump Hops ---
  function addHop() {
    const id = 'h-' + Date.now();
    jumpHops = [...jumpHops, { id, host: '', port: 22, username: '', authMethod: 'key' }];
    markDirty();
  }

  function removeHop(hopId: string) {
    jumpHops = jumpHops.filter(h => h.id !== hopId);
    markDirty();
  }

  function updateHopField(hopId: string, field: string, value: unknown) {
    jumpHops = jumpHops.map(h => h.id === hopId ? { ...h, [field]: value } : h);
    markDirty();
  }

  function moveHop(hopId: string, direction: -1 | 1) {
    const idx = jumpHops.findIndex(h => h.id === hopId);
    if (idx < 0) return;
    const newIdx = idx + direction;
    if (newIdx < 0 || newIdx >= jumpHops.length) return;
    const next = [...jumpHops];
    const [item] = next.splice(idx, 1);
    next.splice(newIdx, 0, item);
    jumpHops = next;
    markDirty();
  }

  async function handleKeyImportForHop(hopId: string) {
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
        jumpHops = jumpHops.map(h => {
          if (h.id !== hopId) return h;
          const ids = h.keyAuth?.identityIds || [];
          return { ...h, keyAuth: { identityIds: [...ids, kid] } };
        });
        markDirty();
      }
    };
    input.click();
  }

  function removeKeyFromHop(hopId: string, keyId: string) {
    jumpHops = jumpHops.map(h => {
      if (h.id !== hopId) return h;
      const ids = (h.keyAuth?.identityIds || []).filter(i => i !== keyId);
      return { ...h, keyAuth: { identityIds: ids } };
    });
    markDirty();
  }

  async function handlePasswordChangeForHop(hopId: string, value: string) {
    if (!value || value === '********') return;
    const pwId = await importPassword(value, `hop-${hopId}`);
    if (pwId) {
      jumpHops = jumpHops.map(h => {
        if (h.id !== hopId) return h;
        return { ...h, passAuth: { passwordId: pwId } };
      });
      markDirty();
    }
  }

  function onProtocolChange(e: Event) {
    const next = (e.currentTarget as HTMLSelectElement).value;
    protocol = next;
    const def = protocols.find(p => p.id === next);
    if (def?.defaultPort) {
      port = def.defaultPort;
    }
    markDirty();
  }
</script>

{#if $detailsConnection}
<div class="connection-details">
  <div class="panel-header">
    <div class="panel-header-left">
      <span>Connection</span>
      <span class="save-indicator">
        {#if saveStatus === 'saving'}Saving...{:else if saveStatus === 'saved'}Saved{/if}
      </span>
    </div>
    <button
      type="button"
      class="panel-close-btn"
      title="Close"
      on:click={() => detailsConnectionId.set('')}
    >
      <X size={14} />
    </button>
  </div>

  <div class="details-body">
    <label class="field">
      <span class="field-label">Name</span>
      <input type="text" bind:value={name} on:input={markDirty} placeholder="My Server" />
    </label>

    <label class="field">
      <span class="field-label">Protocol</span>
      <select value={protocol} on:change={onProtocolChange}>
        {#each protocols as p}
          <option value={p.id}>{p.label}</option>
        {/each}
      </select>
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
            <span class="tag-label">{tag}</span>
            <button class="tag-remove" on:click={() => removeTag(tag)}><X size={9} /></button>
          </span>
        {/each}
        {#if addingTag}
          <div class="tag-input-wrap">
            <input
              class="tag-inline-input"
              class:invalid={tagTooLong}
              placeholder="tag name..."
              bind:value={newTagValue}
              on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); confirmTag(); } if (e.key === 'Escape') cancelTag(); }}
              on:blur={confirmTag}
            />
            {#if tagTooLong}
              <span class="tag-error">Maximum 30 characters</span>
            {/if}
          </div>
        {/if}
        {#if tags.length === 0 && !addingTag}
          <span class="no-items">No tags</span>
        {/if}
      </div>
    </div>

    <!-- Users (SSH only) -->
    {#if isSSH}
    <div class="field">
      <div class="section-header">
        <span class="field-label">Users</span>
        <button class="ghost micro-btn" on:click={addUser}><UserPlus size={12} /> Add</button>
      </div>
      {#each users as u (u.id)}
        <AuthEntryCard
          authMethod={u.authMethod}
          keyAuth={u.keyAuth}
          passAuth={u.passAuth}
          identities={$identities}
          on:authmethodchange={(e) => updateAuthMethod(u.id, e.detail)}
          on:passwordchange={(e) => handlePasswordChange(u.id, e.detail)}
          on:keyimport={() => handleKeyImportForUser(u.id)}
          on:keyremove={(e) => removeKeyFromUser(u.id, e.detail)}
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
        <div class="no-items">No users configured</div>
      {/if}
    </div>
    {/if}

    <!-- Jump Chain (SSH only) -->
    {#if isSSH}
    <div class="field">
      <div class="section-header">
        <span class="field-label">Jump Hosts (Bastion)</span>
        <button class="ghost micro-btn" on:click={addHop}><Plus size={12} /> Hop</button>
      </div>
      {#each jumpHops as hop, idx (hop.id)}
        <AuthEntryCard
          authMethod={hop.authMethod}
          keyAuth={hop.keyAuth}
          passAuth={hop.passAuth}
          identities={$identities}
          on:authmethodchange={(e) => updateHopField(hop.id, 'authMethod', e.detail)}
          on:passwordchange={(e) => handlePasswordChangeForHop(hop.id, e.detail)}
          on:keyimport={() => handleKeyImportForHop(hop.id)}
          on:keyremove={(e) => removeKeyFromHop(hop.id, e.detail)}
          on:remove={() => removeHop(hop.id)}
        >
          <svelte:fragment slot="primary">
            <div class="hop-fields">
              <div class="hop-field-row">
                <input
                  type="text"
                  value={hop.host}
                  on:input={(e) => updateHopField(hop.id, 'host', e.currentTarget.value)}
                  placeholder="host"
                  class="hop-host"
                />
                <input
                  type="number"
                  value={hop.port}
                  on:input={(e) => updateHopField(hop.id, 'port', parseInt(e.currentTarget.value) || 22)}
                  min="1" max="65535"
                  placeholder="port"
                  class="hop-port"
                />
              </div>
              <input
                type="text"
                value={hop.username}
                on:input={(e) => updateHopField(hop.id, 'username', e.currentTarget.value)}
                placeholder="username"
                class="hop-username"
              />
            </div>
          </svelte:fragment>
          <svelte:fragment slot="meta">
            <div class="hop-reorder-stack">
              <button
                class="ghost hop-reorder"
                title="Move up"
                disabled={idx === 0}
                on:click={() => moveHop(hop.id, -1)}
              >
                <ChevronUp size={12} />
              </button>
              <span class="hop-badge" title="Hop order in chain">{idx + 1}</span>
              <button
                class="ghost hop-reorder"
                title="Move down"
                disabled={idx === jumpHops.length - 1}
                on:click={() => moveHop(hop.id, 1)}
              >
                <ChevronDown size={12} />
              </button>
            </div>
          </svelte:fragment>
        </AuthEntryCard>
      {/each}
      {#if jumpHops.length === 0}
        <div class="no-items">No jump hosts configured</div>
      {/if}
    </div>
    {/if}

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

  .panel-header-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .panel-close-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    padding: 2px 4px;
    cursor: pointer;
    color: var(--text-secondary);
    border-radius: 2px;
    flex-shrink: 0;
  }

  .panel-close-btn:hover {
    color: var(--danger);
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
    max-width: 100%;
    min-width: 0;
  }
  .tag-label {
    min-width: 0;
    overflow-wrap: anywhere;
    word-break: break-word;
  }
  .tag-remove {
    background: none; border: none; color: rgba(255,255,255,0.7);
    cursor: pointer; padding: 0 1px; display: inline-flex; align-items: center;
    flex-shrink: 0;
  }
  .tag-remove:hover { color: #fff; }
  .tag-input-wrap {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    flex: 1 1 100%;
    max-width: 100%;
  }
  .tag-inline-input {
    width: 80px; font-size: 11px; padding: 2px 4px;
    background: var(--bg-input); border: 1px solid var(--border-focus);
    color: var(--text-primary); border-radius: 2px; outline: none;
  }
  .tag-inline-input.invalid {
    border-color: var(--danger);
  }
  .tag-error {
    font-size: 10px;
    color: var(--danger);
  }

  /* Users */
  .user-input { flex: 1; font-size: 11px; min-width: 0; width: 100%; }
  .default-radio {
    font-size: 10px; color: var(--text-secondary); display: flex; align-items: center; gap: 2px;
    cursor: pointer; white-space: nowrap;
    flex-shrink: 0;
  }
  .default-radio input { margin: 0; }

  /* Jump hops */
  .hop-fields { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; width: 100%; }
  .hop-field-row { display: flex; gap: 8px; width: 100%; min-width: 0; }
  .field .hop-field-row .hop-host {
    width: auto;
    flex: 1 1 auto;
    font-size: 11px;
    min-width: 0;
  }
  .field .hop-username {
    width: 100%;
    font-size: 11px;
    min-width: 0;
  }
  .field .hop-field-row .hop-port {
    width: calc(60px * var(--ui-scale));
    flex: 0 0 calc(60px * var(--ui-scale));
    font-size: 11px;
  }
  .hop-reorder-stack {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1px;
    flex-shrink: 0;
  }
  .hop-badge {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-secondary);
    background: var(--bg-secondary);
    padding: 0 5px;
    border-radius: 2px;
    white-space: nowrap;
    line-height: 1.4;
  }
  .hop-reorder {
    padding: 0 3px;
    line-height: 1;
    display: inline-flex;
    align-items: center;
  }
  .hop-reorder:disabled {
    opacity: 0.35;
    cursor: default;
  }
</style>
