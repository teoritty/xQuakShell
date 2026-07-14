import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  fetchConnections,
  putConnection,
  deleteConnectionById,
  moveConnectionsTo,
  reorderConnectionsIn,
} from './connections';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  const conn = { id: 'c1', name: 'Conn', host: 'h', port: 22, folderId: '' };
  fake.program('GetAllConnections', [conn]);
  let result = await fetchConnections();
  assert(result.length === 1 && result[0].id === 'c1', 'fetchConnections returns gateway result');

  fake.program('SaveConnection', conn);
  let saved = await putConnection({ name: 'Conn' });
  assert(saved?.id === 'c1', 'putConnection returns saved connection');
  let call = fake.calls.find((c) => c.method === 'SaveConnection');
  assert(!!call && (call.args[0] as any).name === 'Conn', 'SaveConnection called with payload');

  await deleteConnectionById('c1');
  call = fake.calls.find((c) => c.method === 'DeleteConnection');
  assert(!!call && call.args[0] === 'c1', 'DeleteConnection called with id');

  await moveConnectionsTo(['c1', 'c2'], 'f1');
  call = fake.calls.find((c) => c.method === 'MoveConnections');
  assert(!!call && (call.args[0] as string[]).length === 2 && call.args[1] === 'f1', 'MoveConnections called with args');

  await reorderConnectionsIn(['c1', 'c2'], 'f1');
  call = fake.calls.find((c) => c.method === 'ReorderConnections');
  assert(!!call && (call.args[0] as string[]).length === 2 && call.args[1] === 'f1', 'ReorderConnections called with args');

  assert(get(lastError) === null, 'no error reported for successful calls');

  // fallback behavior on failure
  fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);
  fake.program('GetAllConnections', () => { throw new Error('boom'); });
  result = await fetchConnections();
  assert(Array.isArray(result) && result.length === 0, 'fetchConnections falls back to []');
  assert(get(lastError) !== null, 'fetchConnections failure reports error');

  fake.program('SaveConnection', () => { throw new Error('boom'); });
  lastError.set(null);
  saved = await putConnection({ name: 'x' });
  assert(saved === null, 'putConnection falls back to null');
  assert(get(lastError) !== null, 'putConnection failure reports error');

  // no gateway
  setGateway(null as any);
  lastError.set(null);
  result = await fetchConnections();
  assert(Array.isArray(result) && result.length === 0, 'fetchConnections with no gateway returns []');
  saved = await putConnection({ name: 'x' });
  assert(saved === null, 'putConnection with no gateway returns null');
  await deleteConnectionById('c1'); // should not throw
  await moveConnectionsTo(['c1'], 'f1'); // should not throw
  await reorderConnectionsIn(['c1'], 'f1'); // should not throw

  console.log('connections.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
