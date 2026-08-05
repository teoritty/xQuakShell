// Atomic plugin-dialog RPC wrappers (ADR-015 §2).
import { getGateway } from '../backend/context';

/**
 * Submits a form's values.
 *
 * Unlike the other wrappers here, this one lets the error escape: a submit can be refused by the
 * host's own validation, and the dialog must stay open showing why. Swallowing it would close the
 * modal on a value the user still has to fix.
 */
export async function submitPluginDialog(
  dialogId: string,
  values: Record<string, string>
): Promise<void> {
  const app = getGateway();
  if (!app || !app.SubmitPluginDialog) return;
  await app.SubmitPluginDialog(dialogId, values);
}

export async function cancelPluginDialog(dialogId: string): Promise<void> {
  const app = getGateway();
  if (!app || !app.CancelPluginDialog) return;
  try {
    await app.CancelPluginDialog(dialogId);
  } catch (e) {
    console.debug('[dialog cancel]', dialogId, e);
  }
}
