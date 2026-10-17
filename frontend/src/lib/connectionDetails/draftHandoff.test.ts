import { connections, type Connection } from '../../stores/appState';
import { createDraftFromConnection } from './connectionDraft';
import { saveDraftNow, withPassword } from './draftHandoff';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const record = {
  id: 'c1', name: 'db', host: 'db.example', port: 22, folderId: 'f1', order: 3,
  users: [{ id: 'u1', username: 'deploy', authMethod: 'password' }],
} as Connection;

const saved: Partial<Connection>[] = [];
const save = async (c: Partial<Connection>) => {
  saved.push(c);
};

connections.set([record]);
await saveDraftNow({ ...createDraftFromConnection(record, 22), name: 'database' }, record, save);
assert(saved.length === 1, 'an edit left behind is saved, not dropped with the debounce');
assert(saved[0].id === 'c1' && saved[0].name === 'database', 'it is saved as an edit of the connection it was made on');
assert(saved[0].folderId === 'f1' && saved[0].order === 3, 'the save keeps where the connection sits in the tree');

saved.length = 0;
connections.set([]);
await saveDraftNow({ ...createDraftFromConnection(record, 22), name: 'database' }, record, save);
assert(saved.length === 0, 'a moved or deleted connection is not saved; its old id would create a second copy');

const draft = createDraftFromConnection(record, 22);
const next = withPassword(draft, { userId: 'u1' }, 'pw-1');
assert(next?.users[0].passAuth?.passwordId === 'pw-1', 'a late password lands on the user it was typed for');
assert(draft.users[0].passAuth === undefined, 'the draft passed in is not changed under the editor');
assert(withPassword(draft, { userId: 'removed' }, 'pw-1') === null, 'a user removed meanwhile gets nothing');

console.log('draftHandoff: ok');
