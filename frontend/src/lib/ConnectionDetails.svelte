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
  import KeyPickerDialog from './keys/KeyPickerDialog.svelte';
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
  import { followRecord, nextDraftSync, recordKey } from './connectionDetails/draftSync';
  import { adoptPersistedHopIds } from './connectionDetails/hopIds';
  import { adoptPersistedRuleIds } from './connectionDetails/forwardRuleIds';
  import {
    addIdentityToTarget,
    identitiesForTarget,
    removeIdentityFromHop,
    removeIdentityFromUser,
  } from './connectionDetails/authDraftMutations';
  import { saveDraftNow, withPassword } from './connectionDetails/draftHandoff';
  import {
    cancelPendingAutosave, commitFieldEditOnEnter,
    createAutosaveTimerState,
    isStaleAutosaveGeneration,
    scheduleAutosave,
    scheduleSavedIndicatorReset,
  } from './connectionDetails/autosave';
  import { importPasswordIfChanged } from './connectionDetails/authSecrets';
  import { linkPersonalCredential, linksPersonally } from '../actions/personalCredentialActions';
  import ConnectionVaultField from './connectionDetails/ConnectionVaultField.svelte';
  import { isViewOnly, teams } from '../stores/teamState';
  import type { ConnectionDetailsDraft, SaveStatus } from './connectionDetails/types';
  import type { Connection, ConnectionUser, ForwardRule, JumpHop } from '../stores/appState';

  let draft: ConnectionDetailsDraft = createDraftFromConnection(null, 22);
  let fieldErrors: Record<string, string> = {};
  let dirty = false;
  let saveStatus: SaveStatus = 'idle';
  let addingTag = false;
  let newTagValue = '';
  let boundCatalogKey = '', boundRecord: Connection | null = null;
  let formMode: ConnectionFormMode = 'none';
  let formProtocolDef: ConnectionProtocol | null = null;
  const autosaveState = createAutosaveTimerState();

  $: protocols = $connectionProtocols;
  $: protocolCatalogKey = connectionProtocolCatalogKey(protocols);
  $: connId = $detailsConnection?.id || '';
  $: formSectionKey = `${connId}:${formMode}:${protocolCatalogKey}`;
  // A viewer's shared fields are locked; what stays open are its own credentials for the connection.
  $: viewOnly = isViewOnly($teams, $detailsConnection);

  onMount(() => {
    void refreshConnectionProtocols();
  });

  // Why a change made anywhere else - a rename in the tree, an edit on another device - has to reach
  // an open panel, why the record is compared against itself rather than against the draft, and
  // how an unsaved edit survives the record moving under it, is in draftSync.ts.
  $: switch (nextDraftSync({ connId, draftId: draft.editingId, recordKey: recordKey($detailsConnection), boundRecordKey: recordKey(boundRecord), catalogKey: protocolCatalogKey, boundCatalogKey })) {
    case 'rebuild': if (dirty && draft.editingId) void saveDraftNow(draft, boundRecord, saveConnection); boundCatalogKey = protocolCatalogKey; boundRecord = $detailsConnection; syncDraftFromConnection(); break;
    case 'catalog': boundCatalogKey = protocolCatalogKey; resyncProtocolCatalog(); break;
    case 'follow-record': draft = followRecord(draft, boundRecord, $detailsConnection, protocols); boundRecord = $detailsConnection; updateFormMode(); break;
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
    if (!editingId || !dirty || !draft.name.trim()) return;
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
      if (viewOnly) syncDraftFromConnection(); // the backend kept the record; show it as it is
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
    // The stored-secret set belongs to the saved protocol; a different protocol's fields have no
    // secret in the vault under this connection, so every field is sent as typed.
    draft.storedSecretFields = [];
    draft.pluginFieldsTouched = {};
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

  function handleFieldChange(e: CustomEvent<{ fieldId: string; value: unknown }>) {
    const { fieldId, value } = e.detail;
    draft.pluginFields = { ...draft.pluginFields, [fieldId]: value };
    // Mark the field edited so a stored secret, once the user actually types into it, is sent in
    // the save payload (serializePluginFields omits stored secrets only while untouched).
    draft.pluginFieldsTouched = { ...draft.pluginFieldsTouched, [fieldId]: true };
    markDirty();
  }

  // Which user or hop asked for a key. The picker is a single dialog shared by all of them, so
  // the target has to be remembered while it is open rather than passed through it.
  let keyPickTarget = '';
  let showKeyPicker = false;

  function onKeyPick(id: string) {
    keyPickTarget = id;
    showKeyPicker = true;
  }

  function onKeyChosen(keyId: string) {
    showKeyPicker = false;
    // A team user's key is this member's own link, never a change to the record the team shares.
    if ($detailsConnection && linksPersonally($detailsConnection, keyPickTarget)) return void linkPersonalCredential($detailsConnection, keyPickTarget, { identity: keyId });
    const next = addIdentityToTarget({ users: draft.users, hops: draft.jumpHops }, keyPickTarget, keyId);
    draft.users = next.users;
    draft.jumpHops = next.hops;
    markDirty();
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

  async function onPasswordChange({ userId, hopId, value }: { userId?: string; hopId?: string; value: string }) {
    const [before, record] = [draft, boundRecord];
    const pwId = await importPasswordIfChanged(value, userId ? `user-${userId}` : `hop-${hopId}`);
    const moved = draft.editingId !== before.editingId;
    const next = pwId ? withPassword(moved ? before : draft, { userId, hopId }, pwId) : null;
    if (!next) return;
    if (moved) return void saveDraftNow(next, record, saveConnection); // the editor went on; the password stays where it was typed
    draft = next;
    markDirty();
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

  <!-- Enter ends the edit of whichever field has focus, plugin fields included. One handler for the
       whole form rather than one per component: see commitFieldEditOnEnter in autosave.ts. -->
  <div class="details-body" role="group" on:keydown={(e) => commitFieldEditOnEnter(e, autosaveState, runAutosave)}>
    <ConnectionVaultField />

    <fieldset class="shared-fields" disabled={viewOnly}>
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
    </fieldset>

    {#key formSectionKey}
      <ConnectionProtocolForm
        mode={formMode}
        protocolDef={formProtocolDef}
        users={draft.users}
        defaultUserId={draft.defaultUserId}
        jumpHops={draft.jumpHops}
        forwardRules={draft.forwardRules}
        agentForwarding={draft.agentForwarding}
        identities={$identities}
        pluginFields={draft.pluginFields}
        storedSecretFields={draft.storedSecretFields}
        {fieldErrors}
        on:dirty={markDirty}
        on:userschange={(e) => setDraftUsers(e.detail)}
        on:defaultuserchange={(e) => { draft.defaultUserId = e.detail; }}
        on:hopschange={(e) => setDraftHops(e.detail)}
        on:forwardruleschange={(e) => setDraftForwardRules(e.detail)}
        on:agentforwardingchange={(e) => { draft.agentForwarding = e.detail; }}
        on:keypick={(e) => onKeyPick(e.detail)}
        on:keyremove={(e) => onKeyRemove(e.detail)}
        on:passwordchange={(e) => void onPasswordChange(e.detail)}
        on:fieldchange={handleFieldChange}
      />
    {/key}
  </div>
</div>

<KeyPickerDialog
  bind:show={showKeyPicker}
  alreadyChosen={identitiesForTarget({ users: draft.users, hops: draft.jumpHops }, keyPickTarget)}
  on:pick={(e) => onKeyChosen(e.detail)}
  on:close={() => (showKeyPicker = false)}
/>
{/if}

<style>
  .connection-details {
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    max-height: var(--sidebar-bottom-max, 55vh);
    border-top: 1px solid var(--border-color);
  }

  .details-body {
    padding: 8px 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    overflow-y: auto;
  }

  /* Only groups the shared fields so a viewer's can be disabled at once; it draws nothing. */
  .shared-fields {
    display: contents;
  }
</style>
