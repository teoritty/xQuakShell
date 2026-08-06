<script lang="ts">
  // The property panel of a selected discovery node (ADR-015 §3).
  //
  // It sits where Connection Details sits, because it is the same idea applied to the other thing
  // a tree row can be. The host stores none of what it saves: values go back to the plugin, which
  // persists them wherever it keeps its own state.
  import { onDestroy } from 'svelte';
  import FieldSections from './fields/FieldSections.svelte';
  import { validateFieldValue } from './fields/validate';
  import { isFieldVisible } from './fields/layout';
  import {
    nodeDetailsTarget,
    nodeDetailsRevision,
    closeNodeDetails,
    type NodeDetails,
  } from '../stores/nodeDetailsState';
  import { describeDiscoveryNode, applyDiscoveryNodeDetails } from '../api/nodeDetails';
  import { X, Loader2 } from 'lucide-svelte';

  let details: NodeDetails | null = null;
  let values: Record<string, string> = {};
  let errors: Record<string, string> = {};
  let loading = false;
  let failure = '';
  let saving = false;
  let loadedKey = '';
  // The revision this panel has already read. A pushed refresh names the same node, so the key
  // alone cannot tell "a different node" from "the same node, but newer" (ADR-015 §3).
  let loadedRevision = 0;

  $: target = $nodeDetailsTarget;
  $: key = target ? `${target.connectionId}${target.pluginId}${target.nodeId}` : '';
  $: if (key !== loadedKey) {
    loadedKey = key;
    loadedRevision = $nodeDetailsRevision;
    if (target) void load(target.connectionId, target.pluginId, target.nodeId);
    else reset();
  } else if ($nodeDetailsRevision !== loadedRevision) {
    loadedRevision = $nodeDetailsRevision;
    // A push while the user is mid-edit is not applied: overwriting half-typed values with the
    // plugin's snapshot would throw away work nobody asked to discard. Saving re-reads anyway, so
    // the newer snapshot is never lost, only deferred.
    if (target && !dirty) void load(target.connectionId, target.pluginId, target.nodeId);
  }

  function reset() {
    details = null;
    values = {};
    errors = {};
    failure = '';
  }

  async function load(connectionId: string, pluginId: string, nodeId: string) {
    loading = true;
    failure = '';
    try {
      const loaded = await describeDiscoveryNode(connectionId, pluginId, nodeId);
      // The selection may have moved on while the plugin was answering; a stale reply must not
      // replace the panel the user is now looking at.
      if (`${connectionId}${pluginId}${nodeId}` !== loadedKey) return;
      details = loaded;
      values = { ...(loaded?.values ?? {}) };
      errors = {};
    } catch (e) {
      details = null;
      failure = String(e);
    } finally {
      loading = false;
    }
  }

  $: fields = (details?.sections ?? []).flatMap((s) => s.fields);
  $: dirty = !!details && fields.some((f) => (values[f.id] ?? '') !== (details?.values[f.id] ?? ''));
  $: invalid = fields.some(
    (f) => isFieldVisible(f, values) && validateFieldValue(f, values[f.id] ?? '') !== ''
  );

  function onChange(fieldId: string, value: string) {
    values = { ...values, [fieldId]: value };
    const field = fields.find((f) => f.id === fieldId);
    if (field) errors = { ...errors, [fieldId]: validateFieldValue(field, value) };
  }

  async function save() {
    if (!target || !details?.editable || saving || invalid || !dirty) return;
    saving = true;
    failure = '';
    try {
      await applyDiscoveryNodeDetails(target.connectionId, target.pluginId, target.nodeId, values);
      // Re-read rather than assume: the plugin may have normalised what it stored, and showing the
      // values it actually kept beats showing the ones we sent.
      await load(target.connectionId, target.pluginId, target.nodeId);
    } catch (e) {
      failure = String(e);
    } finally {
      saving = false;
    }
  }

  onDestroy(reset);
</script>

{#if target}
  <div class="node-details">
    <div class="header">
      <span class="title" title={target.label}>{target.label}</span>
      <button class="ghost" title="Close" on:click={closeNodeDetails}><X size={13} /></button>
    </div>

    <div class="body">
      {#if loading && !details}
        <div class="status"><Loader2 size={13} /> Loading…</div>
      {:else if failure}
        <div class="status error">{failure}</div>
      {:else if details}
        <FieldSections
          sections={details.sections}
          {values}
          {errors}
          readonly={!details.editable}
          {onChange}
        />
      {:else}
        <div class="status">This item has no details.</div>
      {/if}
    </div>

    {#if details?.editable}
      <div class="actions">
        <button class="primary" disabled={!dirty || invalid || saving} on:click={save}>
          {saving ? 'Saving…' : 'Save'}
        </button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .node-details {
    display: flex;
    flex-direction: column;
    border-top: 1px solid var(--border-color);
    max-height: 55%;
    flex-shrink: 0;
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    padding: 5px 8px;
    border-bottom: 1px solid var(--border-color);
  }

  .title {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-bright);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .body {
    overflow-y: auto;
    padding: 8px;
  }

  .status {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .status.error {
    color: var(--danger);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    padding: 6px 8px;
    border-top: 1px solid var(--border-color);
  }
</style>
