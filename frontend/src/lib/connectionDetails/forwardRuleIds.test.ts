import type { ForwardRule } from '../../stores/appState';
import {
  adoptPersistedRuleIds,
  createDraftRuleUiId,
  filterDraftRules,
  isDraftRuleUiId,
  stripDraftRuleIdsForSave,
} from './forwardRuleIds';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

assert(isDraftRuleUiId('draft-rule-abc'), 'draft-rule ids are UI-only');
assert(!isDraftRuleUiId('a1b2c3d4'), 'persisted rule ids must not be UI-only');

const completeLocal: ForwardRule = {
  id: createDraftRuleUiId(),
  kind: 'local',
  bindAddress: '127.0.0.1',
  bindPort: 8080,
  targetHost: 'db.internal',
  targetPort: 5432,
  enabled: true,
};
const incomplete: ForwardRule = {
  id: createDraftRuleUiId(),
  kind: 'local',
  bindAddress: '127.0.0.1',
  bindPort: 0,
  targetHost: '',
  targetPort: 0,
  enabled: true,
};

const filtered = filterDraftRules([completeLocal, incomplete]);
assert(filtered.length === 1, 'incomplete rules are filtered');
assert(filtered[0].targetHost === 'db.internal', 'complete rule is kept');

const stripped = stripDraftRuleIdsForSave(filtered);
assert(stripped[0].id === '', 'UI-only rule id stripped');

const adopted = adoptPersistedRuleIds(
  [completeLocal, incomplete],
  [{ ...completeLocal, id: 'backend-rule-1' }],
);
assert(adopted[0].id === 'backend-rule-1', 'complete rule adopts backend id');
assert(adopted[1].bindPort === 0, 'incomplete rule unchanged');

console.log('forwardRuleIds.test.ts: all passed');
