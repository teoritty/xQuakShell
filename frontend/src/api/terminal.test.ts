import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { sendTerminalInput, terminalResize } from './terminal';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  // sendTerminalInput: forwards args, defaults commandLine to ''
  let fake = createFakeGateway();
  fake.program('SendTerminalInput', undefined);
  setGateway(fake);
  await sendTerminalInput('sess-1', 'ls\n');
  let call = fake.calls.find((c) => c.method === 'SendTerminalInput');
  assert(!!call && call.args[0] === 'sess-1' && call.args[1] === 'ls\n' && call.args[2] === '', 'sendTerminalInput forwards sessionId/data and defaults commandLine');

  await sendTerminalInput('sess-1', 'ls\n', 'ls');
  call = fake.calls.filter((c) => c.method === 'SendTerminalInput').pop();
  assert(!!call && call.args[2] === 'ls', 'sendTerminalInput forwards explicit commandLine');

  // sendTerminalInput: swallows RPC errors silently (no lastError, no throw)
  fake = createFakeGateway();
  fake.program('SendTerminalInput', () => { throw new Error('input failed'); });
  setGateway(fake);
  let threw: unknown = null;
  try { await sendTerminalInput('sess-1', 'x'); } catch (e) { threw = e; }
  assert(threw === null, 'sendTerminalInput does not throw on RPC failure');

  // sendTerminalInput: no-op when gateway absent
  setGateway(null as any);
  threw = null;
  try { await sendTerminalInput('sess-1', 'x'); } catch (e) { threw = e; }
  assert(threw === null, 'sendTerminalInput no-ops when gateway absent');

  // terminalResize: forwards args
  fake = createFakeGateway();
  fake.program('TerminalResize', undefined);
  setGateway(fake);
  await terminalResize('sess-1', 80, 24);
  const resizeCall = fake.calls.find((c) => c.method === 'TerminalResize');
  assert(!!resizeCall && resizeCall.args[0] === 'sess-1' && resizeCall.args[1] === 80 && resizeCall.args[2] === 24, 'terminalResize forwards sessionId/cols/rows');

  // terminalResize: swallows RPC errors silently (non-critical)
  fake = createFakeGateway();
  fake.program('TerminalResize', () => { throw new Error('resize failed'); });
  setGateway(fake);
  threw = null;
  try { await terminalResize('sess-1', 80, 24); } catch (e) { threw = e; }
  assert(threw === null, 'terminalResize does not throw on RPC failure');

  // terminalResize: no-op when gateway absent
  setGateway(null as any);
  threw = null;
  try { await terminalResize('sess-1', 80, 24); } catch (e) { threw = e; }
  assert(threw === null, 'terminalResize no-ops when gateway absent');

  console.log('terminal.test.ts passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
