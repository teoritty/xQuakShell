// Every terminal that is currently mounted, whichever producer is behind it: an SSH session, a
// local shell or a plugin surface. Search reads their buffers and multi-input writes to their
// inputs through here, so neither has to know which of the three it is dealing with.
//
// Registration is owned. When a tab moves between tiles its Terminal component is recreated, and
// the new one can mount before the old one is destroyed; an unregister that did not check it still
// owned the entry would remove the terminal the new component just registered.
import { writable, type Readable } from 'svelte/store';
import type { Terminal } from '@xterm/xterm';
import type { TerminalIO } from './terminalIO';

export interface LiveTerminal {
  term: Terminal;
  io: TerminalIO;
}

const live = new Map<string, { owner: symbol; entry: LiveTerminal }>();
const ids = writable<string[]>([]);

/** The ids of the mounted terminals, in no particular order. */
export const liveTerminalIds: Readable<string[]> = { subscribe: ids.subscribe };

let publishQueued = false;

// Coalesced to the end of the current task. A tab moving between tiles unmounts one Terminal and
// mounts another in the same update, possibly in that order, and publishing each step would tell
// every subscriber the terminal had closed - dropping it from a multi-input group and from the
// search scope - a moment before telling them it was back.
function publish(): void {
  if (publishQueued) return;
  publishQueued = true;
  queueMicrotask(() => {
    publishQueued = false;
    ids.set([...live.keys()]);
  });
}

/** Records a mounted terminal. Returns the matching unregister. */
export function registerTerminal(entry: LiveTerminal): () => void {
  const owner = Symbol(entry.io.id);
  live.set(entry.io.id, { owner, entry });
  publish();
  return () => {
    if (live.get(entry.io.id)?.owner !== owner) return;
    live.delete(entry.io.id);
    publish();
  };
}

export function liveTerminal(id: string): LiveTerminal | undefined {
  return live.get(id)?.entry;
}
