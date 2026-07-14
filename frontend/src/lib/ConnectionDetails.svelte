<script lang="ts">
  import { onMount } from 'svelte';
  import { detailsConnection, detailsConnectionId, identities } from '../stores/appState';
  import { saveConnection } from '../actions/connectionActions';
  import {
    connectionProtocols,
    connectionProtocolCatalogKey,
    refreshConnectionProtocols,
  } from '../actions/protocolActions';
  import ConnectionDetailsHeader from './connectionDetails/ConnectionDetailsHeader.svelte';
  import ConnectionBaseFields from './connectionDetails/ConnectionBaseFields.svelte';
  import ConnectionTags from './connectionDetails/ConnectionTags.svelte';
  import ConnectionProtocolForm from './connectionDetails/ConnectionProtocolForm.svelte';
  import { connectionDraftStore } from '../stores/connectionDraft';
  import { get } from 'svelte/store';
  import {
    createDraftFromConnection,
    resolveDefaultPort,
  } from './connectionDetails/connectionDraft';
  import {
    applyProtocolFieldDefaults,
    refreshFormModeFromDraft,
    type ConnectionFormMode,
  } from './connectionDetails/connectionFormMode';
  import type { ConnectionProtocol } from '../actions/protocolActions';
  import { buildConnectionSavePayload } from './connectionDetails/savePayload';
  import { adoptPersistedHopIds } from './connectionDetails/hopIds';
  import { adoptPersistedRuleIds } from './connectionDetails/forwardRuleIds';
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
  import type { Connection, ConnectionUser, ForwardRule, JumpHop } from '../stores/appState';

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
    forwardRules: [],
    pluginFields: {},
  };
  let fieldErrors: Record<string, string> = {};
  let dirty = false;
  let saveStatus: SaveStatus = 'idle';
  let addingTag = false;
  let newTagValue = '';
  let boundCatalogKey = '';
  let formMode: ConnectionFormMode = 'none';
  let formProtocolDef: ConnectionProtocol | null = null;
  const autosaveState = createAutosaveTimerState();

  $: protocols = $connectionProtocols;
  $: protocolCatalogKey = connectionProtocolCatalogKey(protocols);
  $: connId = $detailsConnection?.id || '';
  $: formSectionKey = `${connId}:${formMode}:${protocolCatalogKey}`;

  onMount(() => {
    void refreshConnectionProtocols();
  });

  $: if (connId && connId !== draft.editingId) {
    syncDraftFromConnection();
    boundCatalogKey = protocolCatalogKey;
  } else if (connId && protocolCatalogKey !== boundCatalogKey) {
    boundCatalogKey = protocolCatalogKey;
    resyncProtocolCatalog();
  }

  function updateFormMode() {
    const next = refreshFormModeFromDraft(draft, protocols);
    formMode = next.mode;
    formProtocolDef = next.protocolDef;
  }

  function syncDraftFromConnection() {
    const c = $detailsConnection;
    if (!c) return;

    const defaultPort = resolveDefaultPort(c.protocol || 'ssh', protocols, c.port);
    draft = createDraftFromConnection(c, defaultPort);
    applyProtocolFieldDefaults(draft, draft.protocol, protocols);
    updateFormMode();
    dirty = false;
    saveStatus = 'idle';
    addingTag = false;
    newTagValue = '';
    fieldErrors = {};
    cancelPendingAutosave(autosaveState, { invalidate: true });
  }

  function reconcileSavedConnection(saved: Connection, generation: number) {
    if (isStaleAutosaveGeneration(autosaveState, generation) || dirty) return;
    // Intentionally avoid rebuilding the full draft from `saved`: backend save
    // responses contain only persisted rows, while the editor may still contain
    // local in-progress rows that must remain visible to the user.
    draft = {
      ...draft,
      jumpHops: adoptPersistedHopIds(draft.jumpHops, saved.jumpChain ?? []),
      forwardRules: adoptPersistedRuleIds(draft.forwardRules, saved.forwardRules ?? []),
    };
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
    const previousProtocol = draft.protocol;
    connectionDraftStore.setProtocolFields(previousProtocol, { ...draft.pluginFields });
    draft.protocol = e.detail.protocol;
    if (e.detail.defaultPort) draft.port = e.detail.defaultPort;
    draft.pluginFields = { ...(get(connectionDraftStore).protocolFieldHistory[e.detail.protocol] ?? {}) };
    applyProtocolFieldDefaults(draft, draft.protocol, protocols);
    updateFormMode();
    fieldErrors = {};
    markDirty();
  }

  function resyncProtocolCatalog() {
    const c = $detailsConnection;
    if (c && !c.port) {
      draft.port = resolveDefaultPort(c.protocol || 'ssh', protocols, c.port);
    }
    applyProtocolFieldDefaults(draft, draft.protocol, protocols);
    updateFormMode();
    draft = { ...draft };
  }

  function handleFieldChange() {
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

  async function onUserPasswordChange(userId: string, value: string) {
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

  async function onHopPasswordChange(hopId: string, value: string) {
    const editingId = draft.editingId;
    const pwId = await importPasswordIfChanged(value, `hop-${hopId}`);
    if (!pwId || draft.editingId !== editingId) return;
    if (!draft.jumpHops.some((h) => h.id === hopId)) return;
    draft.jumpHops = setHopPassword(draft.jumpHops, hopId, pwId);
    markDirty();
  }

  function onKeyImport(id: string) {
    if (draft.users.some((u) => u.id === id)) {
      void onUserKeyImport(id);
      return;
    }
    void onHopKeyImport(id);
  }

  function onKeyRemove(detail: { userId?: string; hopId?: string; keyId: string }) {
    if (detail.userId) {
      draft.users = removeIdentityFromUser(draft.users, detail.userId, detail.keyId);
      markDirty();
      return;
    }
    if (detail.hopId) {
      draft.jumpHops = removeIdentityFromHop(draft.jumpHops, detail.hopId, detail.keyId);
      markDirty();
    }
  }

  function onPasswordChange(detail: { userId?: string; hopId?: string; value: string }) {
    if (detail.userId) {
      void onUserPasswordChange(detail.userId, detail.value);
      return;
    }
    if (detail.hopId) {
      void onHopPasswordChange(detail.hopId, detail.value);
    }
  }

  function setDraftUsers(users: ConnectionUser[]) {
    draft.users = users;
  }

  function setDraftHops(hops: JumpHop[]) {
    draft.jumpHops = hops;
  }

  function setDraftForwardRules(rules: ForwardRule[]) {
    draft.forwardRules = rules;
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

    {#key formSectionKey}
      <ConnectionProtocolForm
        mode={formMode}
        protocolDef={formProtocolDef}
        users={draft.users}
        defaultUserId={draft.defaultUserId}
        jumpHops={draft.jumpHops}
        forwardRules={draft.forwardRules}
        identities={$identities}
        pluginFields={draft.pluginFields}
        {fieldErrors}
        on:dirty={markDirty}
        on:userschange={(e) => setDraftUsers(e.detail)}
        on:defaultuserchange={(e) => { draft.defaultUserId = e.detail; }}
        on:hopschange={(e) => setDraftHops(e.detail)}
        on:forwardruleschange={(e) => setDraftForwardRules(e.detail)}
        on:keyimport={(e) => onKeyImport(e.detail)}
        on:keyremove={(e) => onKeyRemove(e.detail)}
        on:passwordchange={(e) => onPasswordChange(e.detail)}
        on:fieldchange={handleFieldChange}
      />
    {/key}
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
