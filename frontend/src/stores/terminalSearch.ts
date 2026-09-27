// The terminal search panel: what is being searched for, in which terminals, and what was found.
//
// The panel replaces the connection tree in the sidebar while it is open, the way an editor's
// search view replaces its file explorer: results need the height a list needs, and the tree is one
// hotkey away. Matches are also tinted inside the terminals themselves.
import { get, writable } from 'svelte/store';
import { activeTabId } from './appState';
import { terminalEntries } from './terminalTools';
import { liveTerminal, liveTerminalIds } from '../terminal/terminalRegistry';
import { compileQuery, type SearchOptions } from '../terminal/search/matcher';
import { flattenHits, runSearch, type SearchHit, type TerminalHits } from '../terminal/search/searchRun';
import { forgetClosedTerminals, linesOf, onTerminalTextChanged } from '../terminal/search/textCache';
import { highlightHits, revealHit } from '../terminal/search/highlights';

export interface TerminalSearchState {
  open: boolean;
  query: string;
  options: SearchOptions;
  /** The terminals searched. Kept while the panel is closed, so reopening searches the same set. */
  scope: string[];
  groups: TerminalHits[];
  total: number;
  truncated: boolean;
  error: string;
  /** Index into the flattened hits, -1 when none is current. */
  active: number;
  /** Bumped to ask the panel to focus and select its query box. */
  focusRequest: number;
}

const initial: TerminalSearchState = {
  open: false,
  query: '',
  options: { caseSensitive: false, wholeWord: false, regex: false },
  scope: [],
  groups: [],
  total: 0,
  truncated: false,
  error: '',
  active: -1,
  focusRequest: 0,
};

export const terminalSearch = writable<TerminalSearchState>(initial);

let clearHighlights: (() => void)[] = [];
/** The terminal whose match was last selected, so moving on can clear what it left selected. */
let revealedIn: string | null = null;

function paintHighlights(state: TerminalSearchState): void {
  clearHighlights.forEach((undo) => undo());
  clearHighlights = [];
  if (!state.open) return;
  const current = flattenHits(state.groups)[state.active] ?? null;
  for (const group of state.groups) {
    const term = liveTerminal(group.terminalId)?.term;
    if (term) clearHighlights.push(highlightHits(term, group.hits, current));
  }
}

/** Re-runs the query against the scope. keepActive holds the current hit's position on a refresh. */
function rerun(keepActive: boolean): void {
  const state = get(terminalSearch);
  const compiled = compileQuery(state.query, state.options);
  let next: TerminalSearchState = { ...state, groups: [], total: 0, truncated: false, error: '', active: -1 };
  if (compiled && !compiled.ok) next = { ...next, error: compiled.error };
  if (compiled?.ok) {
    const sources = get(terminalEntries)
      .filter((e) => state.scope.includes(e.id))
      .flatMap((e) => {
        const term = liveTerminal(e.id)?.term;
        return term ? [{ terminalId: e.id, lines: linesOf(e.id, term) }] : [];
      });
    const outcome = runSearch(sources, compiled.re);
    const active = outcome.total === 0 ? -1 : keepActive ? Math.min(Math.max(state.active, 0), outcome.total - 1) : 0;
    next = { ...next, ...outcome, active };
  }
  terminalSearch.set(next);
  paintHighlights(next);
}

function defaultScope(): string[] {
  const live = get(terminalEntries).map((e) => e.id);
  const focused = get(activeTabId);
  return live.includes(focused) ? [focused] : live;
}

/**
 * The search hotkey. Opens the panel scoped to the terminal in focus - the one the user is almost
 * always asking about - and seeds the query with that terminal's selection, as an editor seeds its
 * find box. Pressed while the panel is open it only brings the focus back to the query.
 */
export function openTerminalSearch(): void {
  const state = get(terminalSearch);
  const selection = liveTerminal(get(activeTabId))?.term.getSelection() ?? '';
  const seed = selection && !selection.includes('\n') ? selection : state.query;
  const scope = state.scope.length > 0 ? state.scope : defaultScope();
  terminalSearch.set({ ...state, open: true, query: seed, scope, focusRequest: state.focusRequest + 1 });
  rerun(false);
}

export function closeTerminalSearch(): void {
  terminalSearch.update((s) => ({ ...s, open: false }));
  paintHighlights(get(terminalSearch));
  liveTerminal(get(activeTabId))?.term.focus();
}

export function setSearchQuery(query: string): void {
  terminalSearch.update((s) => ({ ...s, query }));
  rerun(false);
}

export function toggleSearchOption(option: keyof SearchOptions): void {
  terminalSearch.update((s) => ({ ...s, options: { ...s.options, [option]: !s.options[option] } }));
  rerun(false);
}

export function setSearchScope(scope: string[]): void {
  terminalSearch.update((s) => ({ ...s, scope: [...scope] }));
  rerun(false);
}

export function toggleSearchScope(id: string): void {
  const scope = get(terminalSearch).scope;
  setSearchScope(scope.includes(id) ? scope.filter((s) => s !== id) : [...scope, id]);
}

/** Makes a hit current: its terminal comes to the front, scrolls to it and selects it. */
export function showHit(hit: SearchHit): void {
  const state = get(terminalSearch);
  const index = flattenHits(state.groups).indexOf(hit);
  if (index < 0) return;
  terminalSearch.set({ ...state, active: index });
  paintHighlights(get(terminalSearch));
  activeTabId.set(hit.terminalId);
  // A tab brought forward is display:none until the next frame, and a hidden terminal cannot
  // scroll or measure a selection.
  if (revealedIn && revealedIn !== hit.terminalId) liveTerminal(revealedIn)?.term.clearSelection();
  revealedIn = hit.terminalId;
  requestAnimationFrame(() => {
    const term = liveTerminal(hit.terminalId)?.term;
    if (term) revealHit(term, hit);
  });
}

/** Enter and Shift+Enter in the query box: the next or previous hit, wrapping at the ends. */
export function stepHit(delta: 1 | -1): void {
  const state = get(terminalSearch);
  const hits = flattenHits(state.groups);
  if (hits.length === 0) return;
  const base = state.active < 0 ? (delta > 0 ? -1 : 0) : state.active;
  showHit(hits[(base + delta + hits.length) % hits.length]);
}

// New output in a searched terminal refreshes the results, batched: a build log arrives in hundreds
// of writes a second, and one search per write would be all the panel ever did.
let refreshTimer: ReturnType<typeof setTimeout> | null = null;
onTerminalTextChanged((id) => {
  const state = get(terminalSearch);
  if (!state.open || !state.scope.includes(id) || refreshTimer) return;
  refreshTimer = setTimeout(() => {
    refreshTimer = null;
    if (get(terminalSearch).open) rerun(true);
  }, 300);
});

// A closed terminal leaves the scope and the cache; if it was the whole scope, the search is over.
liveTerminalIds.subscribe((ids) => {
  const live = new Set(ids);
  forgetClosedTerminals(live);
  const state = get(terminalSearch);
  const scope = state.scope.filter((id) => live.has(id));
  if (scope.length === state.scope.length) return;
  terminalSearch.set({ ...state, scope });
  if (state.open) rerun(true);
});
