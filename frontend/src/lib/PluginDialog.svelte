<script lang="ts">
  // The modal a plugin opens (ADR-015 §2).
  //
  // A form has submit and cancel and returns values; a detail has only a close button and never
  // submits, because submitting one would hand a plugin an answer to a question it never asked.
  import Modal from './Modal.svelte';
  import FieldSections from './fields/FieldSections.svelte';
  import { validateFieldValue } from './fields/validate';
  import { isFieldVisible } from './fields/layout';
  import { activeDialog, dialogError } from '../stores/dialogState';
  import { submitPluginDialog, cancelPluginDialog } from '../api/dialogs';

  let values: Record<string, string> = {};
  let localErrors: Record<string, string> = {};
  let submitting = false;
  let currentId = '';

  // Rebuilt whenever a different dialog opens. Keyed on the id rather than on the object so a
  // re-emitted event for the SAME dialog does not wipe what the user has typed.
  $: if ($activeDialog && $activeDialog.dialogId !== currentId) {
    currentId = $activeDialog.dialogId;
    values = { ...($activeDialog.values ?? {}) };
    localErrors = {};
    submitting = false;
  }

  $: fields = ($activeDialog?.sections ?? []).flatMap((s) => s.fields);
  // Hidden fields are not validated, matching the host: a dependency that is off means the field
  // is not part of this answer at all.
  $: invalid = fields.some(
    (f) => isFieldVisible(f, values) && validateFieldValue(f, values[f.id] ?? '') !== ''
  );
  $: mergedErrors = { ...localErrors, ...($dialogError?.fieldErrors ?? {}) };

  function onChange(fieldId: string, value: string) {
    values = { ...values, [fieldId]: value };
    const field = fields.find((f) => f.id === fieldId);
    if (!field) return;
    const message = validateFieldValue(field, value);
    localErrors = { ...localErrors, [fieldId]: message };
  }

  async function submit() {
    const dialog = $activeDialog;
    if (!dialog || dialog.kind !== 'form' || submitting || invalid) return;
    submitting = true;
    try {
      await submitPluginDialog(dialog.dialogId, values);
      // The dialog is closed by the host's PluginDialogClosed event, not here: the host decides
      // when an answer has been accepted, and closing early would hide a refusal.
    } catch (e) {
      dialogError.set({ message: String(e), fieldErrors: {} });
    } finally {
      submitting = false;
    }
  }

  function cancel() {
    const dialog = $activeDialog;
    if (!dialog) return;
    void cancelPluginDialog(dialog.dialogId);
  }
</script>

{#if $activeDialog}
  <Modal show title={$activeDialog.title} on:close={cancel}>
    <div class="dialog-body">
      {#if $dialogError?.message}
        <div class="dialog-error">{$dialogError.message}</div>
      {/if}
      <FieldSections
        sections={$activeDialog.sections}
        {values}
        errors={mergedErrors}
        readonly={$activeDialog.kind === 'detail'}
        {onChange}
      />
    </div>
    <div class="dialog-actions">
      {#if $activeDialog.kind === 'form'}
        <button class="ghost" on:click={cancel}>Cancel</button>
        <button class="primary" disabled={invalid || submitting} on:click={submit}>
          {$activeDialog.submitLabel || 'Submit'}
        </button>
      {:else}
        <button class="primary" on:click={cancel}>Close</button>
      {/if}
    </div>
  </Modal>
{/if}

<style>
  .dialog-body {
    max-height: 60vh;
    overflow-y: auto;
    padding-right: 4px;
  }

  .dialog-error {
    background: var(--bg-tertiary);
    border-left: 3px solid var(--danger);
    color: var(--danger);
    padding: 6px 8px;
    font-size: 12px;
    margin-bottom: 10px;
  }

  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 12px;
  }
</style>
