import type { SaveStatus } from './types';

export const AUTOSAVE_DEBOUNCE_MS = 600;
export const SAVED_INDICATOR_MS = 1500;

export interface AutosaveTimerState {
  saveTimer: ReturnType<typeof setTimeout> | null;
  savedIndicatorTimer: ReturnType<typeof setTimeout> | null;
  saveGeneration: number;
}

export interface CancelAutosaveOptions {
  invalidate?: boolean;
}

export function createAutosaveTimerState(): AutosaveTimerState {
  return { saveTimer: null, savedIndicatorTimer: null, saveGeneration: 0 };
}

export function cancelPendingAutosave(
  state: AutosaveTimerState,
  options: CancelAutosaveOptions = {},
): void {
  if (state.saveTimer) {
    clearTimeout(state.saveTimer);
    state.saveTimer = null;
  }
  if (state.savedIndicatorTimer) {
    clearTimeout(state.savedIndicatorTimer);
    state.savedIndicatorTimer = null;
  }
  if (options.invalidate) {
    bumpAutosaveGeneration(state);
  }
}

export function bumpAutosaveGeneration(state: AutosaveTimerState): number {
  state.saveGeneration += 1;
  return state.saveGeneration;
}

export function scheduleAutosave(
  state: AutosaveTimerState,
  onSave: (generation: number) => Promise<void>,
): void {
  cancelPendingAutosave(state);
  const generation = bumpAutosaveGeneration(state);
  state.saveTimer = setTimeout(async () => {
    state.saveTimer = null;
    await onSave(generation);
  }, AUTOSAVE_DEBOUNCE_MS);
}

/** Runs the pending save now instead of waiting out the debounce. The generation moves with it, so
 *  a save already in flight lands stale and its completion is ignored. */
export function flushAutosave(
  state: AutosaveTimerState,
  onSave: (generation: number) => Promise<void>,
): void {
  cancelPendingAutosave(state);
  void onSave(bumpAutosaveGeneration(state));
}

/** The shape this needs from a keydown, so the decision can be tested without a DOM. */
export interface FieldKeyEvent {
  key: string;
  target: unknown;
  preventDefault(): void;
}

/**
 * Enter ends the edit of a connection field, wherever the field came from.
 *
 * It takes focus off the control and persists at once rather than leaving the value to the debounce,
 * because Enter is the one moment the user has said they are done - waiting another 600ms after that
 * reads as lag rather than as batching. One handler on the form covers every field it contains,
 * plugin-declared ones included: they are the same controls in the same box, and a rule that had to
 * be repeated per component is a rule the next field would be added without.
 *
 * A textarea is left alone: there Enter is part of the value, not the end of it. The tag input stops
 * the key before it reaches here, because there Enter already means "commit this tag".
 *
 * Returns whether it acted, so a test can see the decision rather than infer it.
 */
export function commitFieldEditOnEnter(
  event: FieldKeyEvent,
  state: AutosaveTimerState,
  onSave: (generation: number) => Promise<void>,
): boolean {
  const field = event.target as { tagName?: string; blur?: () => void } | null;
  const tag = field?.tagName ?? '';
  if (event.key !== 'Enter' || (tag !== 'INPUT' && tag !== 'SELECT')) return false;
  event.preventDefault();
  field?.blur?.();
  flushAutosave(state, onSave);
  return true;
}

export function isStaleAutosaveGeneration(
  state: AutosaveTimerState,
  generation: number,
): boolean {
  return generation !== state.saveGeneration;
}

export function scheduleSavedIndicatorReset(
  state: AutosaveTimerState,
  generation: number,
  getStatus: () => SaveStatus,
  setStatus: (status: SaveStatus) => void,
): void {
  if (state.savedIndicatorTimer) {
    clearTimeout(state.savedIndicatorTimer);
    state.savedIndicatorTimer = null;
  }
  state.savedIndicatorTimer = setTimeout(() => {
    state.savedIndicatorTimer = null;
    if (isStaleAutosaveGeneration(state, generation)) return;
    if (getStatus() === 'saved') setStatus('idle');
  }, SAVED_INDICATOR_MS);
}
