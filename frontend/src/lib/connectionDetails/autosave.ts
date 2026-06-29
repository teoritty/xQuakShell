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
