// Reactive state for the tools that work across terminals: which terminals exist and what they are
// numbered and called, and the multi-input group. The rules live in terminal/multiInputState.ts
// and terminal/terminalOrder.ts; this file only keeps them in stores and in step with the tabs.
import { derived, get, writable } from 'svelte/store';
import { sessions, activeTabId } from './appState';
import { surfaces, resolveTabIn, tabTitle } from './surfaceState';
import { localTerminals } from './localTerminalState';
import { tileLayout } from './tileLayout';
import { liveTerminalIds, liveTerminal } from '../terminal/terminalRegistry';
import { numberTerminals, type NumberedTerminal } from '../terminal/terminalOrder';
import {
  MULTI_INPUT_OFF,
  cancelPicker,
  confirmPicker,
  mirrorTargets,
  openPicker,
  pruneClosed,
  setDraft,
  toggleMember,
  type MultiInputState,
} from '../terminal/multiInputState';

export interface TerminalEntry extends NumberedTerminal {
  title: string;
}

/** Every mounted terminal in reading order, with its number and its tab's title. */
export const terminalEntries = derived(
  [tileLayout, liveTerminalIds, sessions, surfaces, localTerminals],
  ([$layout, $live, $sessions, $surfaces, $locals]): TerminalEntry[] =>
    numberTerminals($layout.tiles, new Set($live)).map((entry) => {
      const tab = resolveTabIn($sessions, $surfaces, $locals, entry.id);
      return { ...entry, title: tab ? tabTitle(tab) : entry.id };
    })
);

export const multiInput = writable<MultiInputState>(MULTI_INPUT_OFF);

// A terminal that closes leaves the group on its own; nobody else would notice it had gone.
liveTerminalIds.subscribe((ids) => multiInput.update((s) => pruneClosed(s, new Set(ids))));

function focusActiveTerminal(): void {
  liveTerminal(get(activeTabId))?.term.focus();
}

/**
 * The multi-input hotkey. It opens the picker, and pressed again inside the picker it confirms, so
 * the same chord both starts and finishes a quick selection.
 */
export function toggleMultiInputPicker(): void {
  const state = get(multiInput);
  if (state.mode === 'selecting') {
    confirmMultiInput();
    return;
  }
  // With no terminal open there is nothing to pick, and a picker with nothing on screen to show it
  // would swallow Enter and Escape with no visible way out.
  if (get(terminalEntries).length === 0) return;
  const focused = get(activeTabId);
  multiInput.set(openPicker(state, liveTerminal(focused) ? focused : null));
  // The terminal would otherwise keep the keyboard, and the picker's digits would reach its shell.
  (document.activeElement as HTMLElement | null)?.blur?.();
}

export function toggleMultiInputMember(id: string): void {
  multiInput.update((s) => toggleMember(s, id));
}

export function selectAllTerminals(): void {
  multiInput.update((s) => setDraft(s, get(terminalEntries).map((e) => e.id)));
}

export function selectNoTerminals(): void {
  multiInput.update((s) => setDraft(s, []));
}

export function confirmMultiInput(): void {
  multiInput.update(confirmPicker);
  focusActiveTerminal();
}

export function cancelMultiInput(): void {
  multiInput.update(cancelPicker);
  focusActiveTerminal();
}

export function stopMultiInput(): void {
  multiInput.set(MULTI_INPUT_OFF);
  focusActiveTerminal();
}

/**
 * Repeats input typed into sourceId to the rest of its group.
 *
 * The command line captured from the source goes with each copy. It is what was typed, which is
 * what the audit trail records; the targets' own echo of it has not arrived yet when Enter is
 * pressed, so reading their buffers instead would record nothing.
 */
export function mirrorInput(sourceId: string, data: string, commandLine: string): void {
  for (const id of mirrorTargets(get(multiInput), sourceId)) {
    liveTerminal(id)?.io.sendInput(data, commandLine);
  }
}
