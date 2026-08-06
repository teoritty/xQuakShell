// The one plugin dialog that can be open at a time (ADR-015 §2).
//
// A single store rather than a list, matching the host's limit: two modals from one plugin would
// stack, and the user would answer a question without seeing which one they were answering. The
// host refuses a second open, so the frontend never has one to hold.
import { writable } from 'svelte/store';
import type { ValidatableField } from '../lib/fields/validate';

export type DialogKind = 'form' | 'detail';

export interface DialogField extends ValidatableField {
  placeholder?: string;
  description?: string;
  width?: 'full' | 'half' | 'third';
  order?: number;
  dependsOn?: string;
}

export interface DialogSection {
  id: string;
  label: string;
  order?: number;
  fields: DialogField[];
}

export interface PluginDialog {
  dialogId: string;
  pluginId: string;
  kind: DialogKind;
  title: string;
  submitLabel?: string;
  sections: DialogSection[];
  values: Record<string, string>;
}

export const activeDialog = writable<PluginDialog | null>(null);

/** A plugin-reported failure shown on the open dialog, which stays open so the user can correct it. */
export const dialogError = writable<{ message: string; fieldErrors: Record<string, string> } | null>(
  null
);

export function openDialog(dialog: PluginDialog): void {
  activeDialog.set(dialog);
  dialogError.set(null);
}

/**
 * Closes the dialog if the id matches.
 *
 * The id check matters: a close for a dialog that was already replaced would otherwise dismiss the
 * new one — a real race, since the host closes and opens on the same event stream.
 */
export function closeDialog(dialogId: string): void {
  activeDialog.update((current) => (current && current.dialogId !== dialogId ? current : null));
  dialogError.set(null);
}

export function setDialogError(
  dialogId: string,
  message: string,
  fieldErrors: Record<string, string>
): void {
  activeDialog.update((current) => {
    if (current && current.dialogId === dialogId) {
      dialogError.set({ message, fieldErrors: fieldErrors ?? {} });
    }
    return current;
  });
}
