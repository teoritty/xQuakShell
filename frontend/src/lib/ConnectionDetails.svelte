<script lang="ts">
  import { onMount } from 'svelte';
  import { detailsConnection, detailsConnectionId, identities } from '../stores/appState';
  import { saveConnection, getPluginConnectionProtocols, type ConnectionProtocol } from '../stores/api';
  import ConnectionDetailsHeader from './connectionDetails/ConnectionDetailsHeader.svelte';
  import ConnectionBaseFields from './connectionDetails/ConnectionBaseFields.svelte';
  import ConnectionTags from './connectionDetails/ConnectionTags.svelte';
  import ConnectionUsers from './connectionDetails/ConnectionUsers.svelte';
  import JumpHosts from './connectionDetails/JumpHosts.svelte';
  import {
    createDraftFromConnection,
    resolveDefaultPort,
  } from './connectionDetails/connectionDraft';
  import { buildConnectionSavePayload } from './connectionDetails/savePayload';
  import {
    addIdentityToHop,
    addIdentityToUser,
    removeIdentityFromHop,
    removeIdentityFromUser,
    setHopPassword,
    setUserPassword,
  } from './connectionDetails/authDraftMutations';
  import {
    cancelPendingAutosave,
    createAutosaveTimerState,
    isStaleAutosaveGeneration,
    scheduleAutosave,
    scheduleSavedIndicatorReset,
  } from './connectionDetails/autosave';
  import { pickAndImportIdentity, importPasswordIfChanged } from './connectionDetails/authSecrets';
  import type { ConnectionDetailsDraft, SaveStatus } from './connectionDetails/types';
  import type { Connection, ConnectionUser, JumpHop } from '../stores/appState';

  let draft: ConnectionDetailsDraft = {
    editingId: '',
    name: '',
    protocol: 'ssh',
    host: '',
    port: 22,
    tags: [],
    users: [],
    defaultUserId: '',
    jumpHops: [],
  };
  let protocols: ConnectionProtocol[] = [{ id: 'ssh', label: 'SSH', defaultPort: 22, icon: 'terminal' }];
  let dirty = false;
  let saveStatus: SaveStatus = 'idle';
  let addingTag = false;
  let newTagValue = '';
  const autosaveState = createAutosaveTimerState();

  $: connId = $detailsConnection?.id || '';
  $: isSSH = draft.protocol === 'ssh';

  onMount(async () => {
    protocols = await getPluginConnectionProtocols();
  });

  $: if (connId !== draft.editingId) {
    loadFromStore();
  }

  function loadFromStore() {
    const c = $detailsConnection;
    const defaultPort = resolveDefaultPort(c?.protocol || 'ssh', protocols, c?.port);
    draft = createDraftFromConnection(c, defaultPort);
    dirty = false;
    saveStatus = 'idle';
    addingTag = false;
    newTagValue = '';
    cancelPendingAutosave(autosaveState, { invalidate: true });
  }

  function reconcileSavedConnection(saved: Connection, generation: number) {
    if (isStaleAutosaveGeneration(autosaveState, generation) || dirty) return;
    const defaultPort = resolveDefaultPort(saved.protocol || 'ssh', protocols, saved.port);
    draft = createDraftFromConnection(saved, defaultPort);
  }

  function markDirty() {
    dirty = true;
    saveStatus = 'idle';
    scheduleAutosave(autosaveState, runAutosave);
  }

  async function runAutosave(generation: number) {
    const editingId = draft.editingId;
    if (!editingId || !dirty) return;
    if (isStaleAutosaveGeneration(autosaveState, generation)) return;

    saveStatus = 'saving';
    const payload = buildConnectionSavePayload(draft, {
      folderId: $detailsConnection?.folderId || '',
      order: $detailsConnection?.order ?? 0,
    });

    try {
      const saved = await saveConnection(payload);
      if (isStaleAutosaveGeneration(autosaveState, generation)) return;
      if (draft.editingId !== editingId) return;
      if (!saved) {
        saveStatus = 'idle';
        return;
      }

      dirty = false;
      reconcileSavedConnection(saved, generation);
      saveStatus = 'saved';
      scheduleSavedIndicatorReset(
        autosaveState,
        generation,
        () => saveStatus,
        (s) => { saveStatus = s; },
      );
    } catch (e) {
      console.error('autoSave', e);
      if (!isStaleAutosaveGeneration(autosaveState, generation)) {
        saveStatus = 'idle';
      }
    }
  }

  function onProtocolChange(e: CustomEvent<{ protocol: string; defaultPort?: number }>) {
    draft.protocol = e.detail.protocol;
    if (e.detail.defaultPort) draft.port = e.detail.defaultPort;
    markDirty();
  }

  async function onUserKeyImport(userId: string) {
    const editingId = draft.editingId;
    const kid = await pickAndImportIdentity();
    if (!kid || draft.editingId !== editingId) return;
    if (!draft.users.some((u) => u.id === userId)) return;
    draft.users = addIdentityToUser(draft.users, userId, kid);
    markDirty();
  }

  function onUserKeyRemove(e: CustomEvent<{ userId: string; keyId: string }>) {
    const { userId, keyId } = e.detail;
    draft.users = removeIdentityFromUser(draft.users, userId, keyId);
    markDirty();
  }

  async function onUserPasswordChange(e: CustomEvent<{ userId: string; value: string }>) {
    const { userId, value } = e.detail;
    const editingId = draft.editingId;
    const pwId = await importPasswordIfChanged(value, `user-${userId}`);
    if (!pwId || draft.editingId !== editingId) return;
    if (!draft.users.some((u) => u.id === userId)) return;
    draft.users = setUserPassword(draft.users, userId, pwId);
    markDirty();
  }

  async function onHopKeyImport(hopId: string) {
    const editingId = draft.editingId;
    const kid = await pickAndImportIdentity();
    if (!kid || draft.editingId !== editingId) return;
    if (!draft.jumpHops.some((h) => h.id === hopId)) return;
    draft.jumpHops = addIdentityToHop(draft.jumpHops, hopId, kid);
    markDirty();
  }

  function onHopKeyRemove(e: CustomEvent<{ hopId: string; keyId: string }>) {
    const { hopId, keyId } = e.detail;
    draft.jumpHops = removeIdentityFromHop(draft.jumpHops, hopId, keyId);
    markDirty();
  }

  async function onHopPasswordChange(e: CustomEvent<{ hopId: string; value: string }>) {
    const { hopId, value } = e.detail;
    const editingId = draft.editingId;
    const pwId = await importPasswordIfChanged(value, `hop-${hopId}`);
    if (!pwId || draft.editingId !== editingId) return;
    if (!draft.jumpHops.some((h) => h.id === hopId)) return;
    draft.jumpHops = setHopPassword(draft.jumpHops, hopId, pwId);
    markDirty();
  }

  function setDraftUsers(users: ConnectionUser[]) {
    draft.users = users;
  }

  function setDraftHops(hops: JumpHop[]) {
    draft.jumpHops = hops;
  }
</script>

{#if $detailsConnection}
<div class="connection-details">
  <ConnectionDetailsHeader {saveStatus} on:close={() => detailsConnectionId.set('')} />

  <div class="details-body">
    <ConnectionBaseFields
      bind:name={draft.name}
      bind:protocol={draft.protocol}
      bind:host={draft.host}
      bind:port={draft.port}
      {protocols}
      on:dirty={markDirty}
      on:protocolchange={onProtocolChange}
    />

    <ConnectionTags
      tags={draft.tags}
      {addingTag}
      {newTagValue}
      on:dirty={markDirty}
      on:tagschange={(e) => { draft.tags = e.detail; }}
      on:addingtagchange={(e) => { addingTag = e.detail; }}
      on:newtagvaluechange={(e) => { newTagValue = e.detail; }}
    />

    {#if isSSH}
      <ConnectionUsers
        users={draft.users}
        defaultUserId={draft.defaultUserId}
        identities={$identities}
        on:dirty={markDirty}
        on:userschange={(e) => setDraftUsers(e.detail)}
        on:defaultuserchange={(e) => { draft.defaultUserId = e.detail; }}
        on:keyimport={(e) => onUserKeyImport(e.detail)}
        on:keyremove={onUserKeyRemove}
        on:passwordchange={onUserPasswordChange}
      />

      <JumpHosts
        jumpHops={draft.jumpHops}
        identities={$identities}
        on:dirty={markDirty}
        on:hopschange={(e) => setDraftHops(e.detail)}
        on:keyimport={(e) => onHopKeyImport(e.detail)}
        on:keyremove={onHopKeyRemove}
        on:passwordchange={onHopPasswordChange}
      />
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

  .details-body {
    padding: 8px 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    overflow-y: auto;
  }
</style>
