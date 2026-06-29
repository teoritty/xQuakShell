import type { ConnectionDetailsDraft } from './types';
import { isDraftHopUiId, stripDraftHopIdsForSave, filterDraftHops, adoptPersistedHopIds } from './hopIds';
import { buildConnectionSavePayload } from './savePayload';
import {
  cancelPendingAutosave,
  bumpAutosaveGeneration,
  createAutosaveTimerState,
  isStaleAutosaveGeneration,
  scheduleSavedIndicatorReset,
} from './autosave';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const sshDraft: ConnectionDetailsDraft = {
  editingId: 'c1',
  name: 'Server',
  protocol: 'ssh',
  host: 'host.example.com',
  port: 22,
  tags: ['prod'],
  users: [{ id: 'u1', username: 'alice', authMethod: 'key' }],
  defaultUserId: 'u1',
  jumpHops: [
    { id: 'draft-hop-abc', host: 'bastion', port: 22, username: 'jump', authMethod: 'key' },
    { id: 'hop-persist-123', host: 'bastion2', port: 2222, username: 'jump2', authMethod: 'key' },
    { id: '', host: '', port: 22, username: '', authMethod: 'key' },
  ],
};

assert(isDraftHopUiId('draft-hop-123'), 'draft-hop ids are UI-only');
assert(isDraftHopUiId('h-legacy-123'), 'legacy UI ids are UI-only');
assert(!isDraftHopUiId('h-custom-persisted'), 'broad h-prefix must not be UI-only');
assert(!isDraftHopUiId('550e8400-e29b-41d4-a716-446655440000'), 'uuid must be persistent');
assert(!isDraftHopUiId('hop-persist-123'), 'persisted hop ids must not be UI-only');

const stripped = stripDraftHopIdsForSave(filterDraftHops(sshDraft.jumpHops));
assert(stripped.length === 2, 'empty-host hops are filtered');
assert(stripped[0].id === '', 'UI-only hop id stripped');
assert(stripped[1].id === 'hop-persist-123', 'backend hop id preserved');

assert(stripped[1].id === 'hop-persist-123', 'backend hop id preserved');

const inProgressHop = { id: 'draft-hop-new', host: '', port: 22, username: '', authMethod: 'key' as const };
const adoptedSingle = adoptPersistedHopIds(
  [{ id: 'draft-hop-old', host: 'bastion', port: 22, username: 'jump', authMethod: 'key' }, inProgressHop],
  [{ id: 'backend-uuid-1', host: 'bastion', port: 22, username: 'jump', authMethod: 'key' }],
);
assert(adoptedSingle.length === 2, 'in-progress hop row is kept');
assert(adoptedSingle[0].id === 'backend-uuid-1', 'persisted hop adopts backend uuid');
assert(adoptedSingle[1].id === 'draft-hop-new' && adoptedSingle[1].host === '', 'empty-host hop unchanged');

const adoptedMixed = adoptPersistedHopIds(
  [
    { id: 'd1', host: 'hop1', port: 22, username: 'a', authMethod: 'key' },
    { id: 'd2', host: '', port: 22, username: '', authMethod: 'key' },
    { id: 'd3', host: 'hop3', port: 22, username: 'c', authMethod: 'key' },
  ],
  [
    { id: 'be-1', host: 'hop1', port: 22, username: 'a', authMethod: 'key' },
    { id: 'be-3', host: 'hop3', port: 22, username: 'c', authMethod: 'key' },
  ],
);
assert(adoptedMixed[0].id === 'be-1', 'first filled hop gets first saved id');
assert(adoptedMixed[1].host === '' && adoptedMixed[1].id === 'd2', 'empty hop keeps draft id');
assert(adoptedMixed[2].id === 'be-3', 'second filled hop gets second saved id');

const sshPayload = buildConnectionSavePayload(sshDraft, { folderId: 'f1', order: 1 });
assert(Array.isArray(sshPayload.jumpChain) && (sshPayload.jumpChain as unknown[]).length === 2, 'ssh keeps hops');
assert((sshPayload.users as unknown[]).length === 1, 'ssh keeps users');

const pluginPayload = buildConnectionSavePayload(
  { ...sshDraft, protocol: 'rdp' },
  { folderId: 'f1', order: 1 },
);
// Protocol switch must not erase reversible SSH draft fields during autosave.
assert(
  (pluginPayload.jumpChain as unknown[]).length === 2,
  'non-ssh must retain jump chain for protocol switching',
);
assert(
  (pluginPayload.users as unknown[]).length === 1,
  'non-ssh must retain users for protocol switching',
);
assert(
  pluginPayload.defaultUserId === 'u1',
  'non-ssh must retain default user for protocol switching',
);

const autosaveState = createAutosaveTimerState();
const gen = bumpAutosaveGeneration(autosaveState);
cancelPendingAutosave(autosaveState, { invalidate: true });
assert(isStaleAutosaveGeneration(autosaveState, gen), 'cancel with invalidate stales in-flight save');

let status: 'idle' | 'saving' | 'saved' = 'saved';
const indicatorState = createAutosaveTimerState();
const oldGen = bumpAutosaveGeneration(indicatorState);
scheduleSavedIndicatorReset(indicatorState, oldGen, () => status, (s) => { status = s; });
bumpAutosaveGeneration(indicatorState);
status = 'saved';

await new Promise((resolve) => setTimeout(resolve, 1600));
assert(status === 'saved', 'stale saved-indicator reset must not clear status');

console.log('connectionDraft.test.ts: all passed');
