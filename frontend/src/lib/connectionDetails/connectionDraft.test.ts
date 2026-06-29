import type { ConnectionDetailsDraft } from './types';
import { isDraftHopUiId, stripDraftHopIdsForSave, filterDraftHops } from './hopIds';
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

const sshPayload = buildConnectionSavePayload(sshDraft, { folderId: 'f1', order: 1 });
assert(Array.isArray(sshPayload.jumpChain) && (sshPayload.jumpChain as unknown[]).length === 2, 'ssh keeps hops');
assert((sshPayload.users as unknown[]).length === 1, 'ssh keeps users');

const pluginPayload = buildConnectionSavePayload(
  { ...sshDraft, protocol: 'rdp' },
  { folderId: 'f1', order: 1 },
);
assert((pluginPayload.jumpChain as unknown[]).length === 0, 'non-ssh clears jump chain');
assert((pluginPayload.users as unknown[]).length === 0, 'non-ssh clears users');
assert(pluginPayload.defaultUserId === '', 'non-ssh clears default user');

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
