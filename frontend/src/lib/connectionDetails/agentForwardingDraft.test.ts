// The connection editor's agent forwarding rules (ADR-022), and their path into the save payload.
import {
  emptyAgentForwarding,
  forwardableKeys,
  listedWithoutConsent,
  setConfirmPolicy,
  setForwardingEnabled,
  setWindowMinutes,
  toggleForwardedKey,
  DEFAULT_WINDOW_MINUTES,
  MAX_WINDOW_MINUTES,
} from './agentForwardingDraft';
import { createDraftFromConnection } from './connectionDraft';
import { buildConnectionSavePayload } from './savePayload';
import type { Connection, SSHIdentityMeta } from '../../stores/appState';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const keys: SSHIdentityMeta[] = [
  { id: 'lent', comment: 'deploy', keyType: 'ed25519', allowForwarding: true },
  { id: 'kept', comment: 'prod', keyType: 'ed25519', allowForwarding: false },
  { id: 'old', comment: 'legacy', keyType: 'ed25519' },
];

// --- defaults ---

const empty = emptyAgentForwarding();
assert(!empty.enabled && empty.identityIds.length === 0, 'a connection starts with forwarding off and no keys');
assert(empty.confirm === 'each', 'the default asks for every signature');
assert(empty.windowMinutes === DEFAULT_WINDOW_MINUTES, 'the default window matches the backend default');

// --- edits ---

const on = setForwardingEnabled(null, true);
assert(on.enabled && on.identityIds.length === 0, 'switching on a connection that never had forwarding selects no key by itself');
const withKey = toggleForwardedKey(on, 'lent');
assert(withKey.identityIds.join() === 'lent', 'a key is added');
assert(toggleForwardedKey(withKey, 'lent').identityIds.length === 0, 'the same toggle removes it');
assert(on.identityIds.length === 0, 'edits do not mutate the settings they started from');
assert(setConfirmPolicy(on, 'session-window').confirm === 'session-window', 'the policy is set');

assert(setWindowMinutes(on, 0).windowMinutes === 1, 'a window below one minute is raised to one');
assert(setWindowMinutes(on, 1000).windowMinutes === MAX_WINDOW_MINUTES, 'a window above the maximum is lowered to it');
assert(setWindowMinutes(on, 7.6).windowMinutes === 8, 'a fractional window is rounded');
assert(setWindowMinutes(on, NaN).windowMinutes === DEFAULT_WINDOW_MINUTES, 'an unreadable window falls back to the default');

// --- the key's half of the consent ---

const offered = forwardableKeys(keys).map((k) => k.id);
assert(offered.join() === 'lent', `only keys that allow forwarding are offered; got ${offered}`);
const listed = { ...on, identityIds: ['lent', 'kept', 'old'] };
const withdrawn = listedWithoutConsent(listed, keys).map((k) => k.id).join();
assert(withdrawn === 'kept,old', `listed keys without consent are named; got ${withdrawn}`);
assert(listedWithoutConsent(null, keys).length === 0, 'no settings name nothing');

// --- round trip through the editor draft and the save payload ---

const conn: Connection = {
  id: 'c1', folderId: '', name: 'bastion', host: 'h', port: 22, order: 0,
  agentForwarding: { enabled: true, identityIds: ['lent'], confirm: 'session-window', windowMinutes: 5 },
};
const draft = createDraftFromConnection(conn, 22);
assert(draft.agentForwarding?.windowMinutes === 5 && draft.agentForwarding.identityIds.join() === 'lent', 'the draft carries the saved settings');
draft.agentForwarding!.identityIds.push('x');
assert(conn.agentForwarding!.identityIds.length === 1, 'editing the draft does not change the stored connection');
const payload = buildConnectionSavePayload(draft, { folderId: '', order: 0 }) as { agentForwarding?: { identityIds: string[] } };
assert(payload.agentForwarding?.identityIds.join() === 'lent,x', 'the save payload carries the edited settings');

const plain = createDraftFromConnection({ id: 'c2', folderId: '', name: 'n', host: 'h', port: 22, order: 0 }, 22);
assert(plain.agentForwarding === null, 'a connection without forwarding has no settings in its draft');
const plainPayload = buildConnectionSavePayload(plain, { folderId: '', order: 0 }) as { agentForwarding?: unknown };
assert(plainPayload.agentForwarding === undefined, 'and saves without any, so the vault stays as it was');

console.log('agentForwardingDraft.test.ts: all assertions passed');
