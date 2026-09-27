// Multi-input: typing into one terminal and having every chosen terminal receive the same keys.
//
// Three modes. Off. Selecting, where the user picks terminals and nothing typed reaches any of
// them. Active, where input to a member is repeated to every other member. The picker is a mode of
// its own rather than a dialog because the choice is made on the terminals themselves - clicking
// them, or pressing the number drawn on them - and a dialog would cover exactly what is being
// chosen.
//
// Pure: every transition takes a state and returns the next one, so the rules can be tested
// without a store, a terminal or a keyboard.

export type MultiInputState =
  | { mode: 'off' }
  /** previous is the group that was active when the picker opened, restored on cancel. */
  | { mode: 'selecting'; draft: string[]; previous: string[] }
  | { mode: 'active'; members: string[] };

export const MULTI_INPUT_OFF: MultiInputState = { mode: 'off' };

/**
 * Repeating keystrokes to a single terminal is just typing, so a group means two or more. Asking for
 * fewer is how the picker turns multi-input off, which keeps "stop" reachable from the keyboard
 * without spending a key a shell might want.
 */
export const MIN_GROUP = 2;

/**
 * Opens the picker. From off it starts with the terminal the user was typing in, the one they will
 * almost always want in the group; from active it starts with the current group, so changing the
 * group is an edit rather than a rebuild.
 */
export function openPicker(state: MultiInputState, focusedId: string | null): MultiInputState {
  if (state.mode === 'selecting') return state;
  if (state.mode === 'active') {
    return { mode: 'selecting', draft: [...state.members], previous: [...state.members] };
  }
  return { mode: 'selecting', draft: focusedId ? [focusedId] : [], previous: [] };
}

export function toggleMember(state: MultiInputState, id: string): MultiInputState {
  if (state.mode !== 'selecting') return state;
  const draft = state.draft.includes(id) ? state.draft.filter((d) => d !== id) : [...state.draft, id];
  return { ...state, draft };
}

export function setDraft(state: MultiInputState, ids: string[]): MultiInputState {
  if (state.mode !== 'selecting') return state;
  return { ...state, draft: [...ids] };
}

/** Enter: starts the group, or turns multi-input off when fewer than two are chosen. */
export function confirmPicker(state: MultiInputState): MultiInputState {
  if (state.mode !== 'selecting') return state;
  return state.draft.length >= MIN_GROUP ? { mode: 'active', members: [...state.draft] } : MULTI_INPUT_OFF;
}

/** Escape: leaves things exactly as they were before the picker opened. */
export function cancelPicker(state: MultiInputState): MultiInputState {
  if (state.mode !== 'selecting') return state;
  return state.previous.length >= MIN_GROUP ? { mode: 'active', members: [...state.previous] } : MULTI_INPUT_OFF;
}

/**
 * Drops terminals that have closed. A group left with one member is no longer a group, and
 * silently becomes off rather than lingering with a green outline that means nothing.
 */
export function pruneClosed(state: MultiInputState, live: ReadonlySet<string>): MultiInputState {
  if (state.mode === 'off') return state;
  if (state.mode === 'selecting') {
    const draft = state.draft.filter((id) => live.has(id));
    const previous = state.previous.filter((id) => live.has(id));
    if (draft.length === state.draft.length && previous.length === state.previous.length) return state;
    return { ...state, draft, previous };
  }
  const members = state.members.filter((id) => live.has(id));
  if (members.length === state.members.length) return state;
  return members.length >= MIN_GROUP ? { mode: 'active', members } : MULTI_INPUT_OFF;
}

/**
 * Where input typed into sourceId must also go. Nothing unless the source is itself a member:
 * typing into a terminal outside the group reaches that terminal alone, so the green outline is an
 * exact statement of what a keystroke will touch.
 */
export function mirrorTargets(state: MultiInputState, sourceId: string): string[] {
  if (state.mode !== 'active' || !state.members.includes(sourceId)) return [];
  return state.members.filter((id) => id !== sourceId);
}

/** Whether id is chosen: in the picker's draft while selecting, in the group while active. */
export function isChosen(state: MultiInputState, id: string): boolean {
  if (state.mode === 'selecting') return state.draft.includes(id);
  if (state.mode === 'active') return state.members.includes(id);
  return false;
}
