import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  getPluginContributions,
  executePluginCommand,
  preparePluginViewPanel,
  relayPluginViewMessage,
  releasePluginViewPanel,
} from './pluginRuntime';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

function withoutMethod<T extends object>(target: T, methodName: string): T {
  return new Proxy(target, {
    get(t, prop: string) {
      if (prop === methodName) return undefined;
      return (t as unknown as Record<string, unknown>)[prop];
    },
  }) as T;
}

async function run() {
  let fake = createFakeGateway();

  // getPluginContributions: empty-shape default when method absent
  let noMethod = withoutMethod(fake, 'GetPluginContributions');
  setGateway(noMethod);
  let contrib = await getPluginContributions();
  assert(
    Array.isArray(contrib.commands) && contrib.commands.length === 0 &&
    Array.isArray(contrib.views) && contrib.views.length === 0 &&
    Array.isArray(contrib.statusBar) && contrib.statusBar.length === 0 &&
    Array.isArray(contrib.authMethods) && contrib.authMethods.length === 0 &&
    Array.isArray(contrib.tunnelProviders) && contrib.tunnelProviders.length === 0,
    'getPluginContributions returns empty-shape default when method absent'
  );

  // getPluginContributions: pass-through on success
  fake = createFakeGateway();
  const shape = { commands: [{ pluginId: 'p', id: 'c', fullId: 'p.c', title: 'T' }], views: [], statusBar: [], authMethods: [], tunnelProviders: [] };
  fake.program('GetPluginContributions', shape);
  setGateway(fake);
  contrib = await getPluginContributions();
  assert(contrib.commands.length === 1 && contrib.commands[0].id === 'c', 'getPluginContributions returns gateway result');

  // getPluginContributions: falls back to empty-shape default + reports error on RPC failure
  fake = createFakeGateway();
  fake.program('GetPluginContributions', () => { throw new Error('boom'); });
  setGateway(fake);
  lastError.set(null);
  contrib = await getPluginContributions();
  assert(contrib.commands.length === 0, 'getPluginContributions falls back to empty-shape default on error');
  assert(get(lastError) !== null, 'getPluginContributions reports error via lastError');

  // executePluginCommand: throws when method absent
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'ExecutePluginCommand');
  setGateway(noMethod);
  let threw: unknown = null;
  try { await executePluginCommand('p', 'c'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'Plugin commands are unavailable', 'executePluginCommand throws when method absent');

  // executePluginCommand: JSON-string result parsed to object; args stringified
  fake = createFakeGateway();
  fake.program('ExecutePluginCommand', '{"a":"b"}');
  setGateway(fake);
  let result = await executePluginCommand('plug1', 'cmd1', { x: 1 });
  assert((result as unknown as { a: string }).a === 'b', 'executePluginCommand parses JSON string result');
  let call = fake.calls.find((c) => c.method === 'ExecutePluginCommand');
  assert(!!call && call.args[2] === JSON.stringify({ x: 1 }), 'executePluginCommand serializes args via JSON.stringify');

  // executePluginCommand: non-JSON string result wrapped as { message }
  fake = createFakeGateway();
  fake.program('ExecutePluginCommand', 'hi');
  setGateway(fake);
  result = await executePluginCommand('plug1', 'cmd1');
  assert(result.message === 'hi', 'executePluginCommand wraps non-JSON string result as { message }');
  call = fake.calls.find((c) => c.method === 'ExecutePluginCommand');
  assert(!!call && call.args[2] === null, 'executePluginCommand passes null rawArgs when no args given');

  // executePluginCommand: null/falsy result becomes {}
  fake = createFakeGateway();
  fake.program('ExecutePluginCommand', null);
  setGateway(fake);
  result = await executePluginCommand('plug1', 'cmd1');
  assert(Object.keys(result).length === 0, 'executePluginCommand returns {} for falsy result');

  // executePluginCommand: object result passes through
  fake = createFakeGateway();
  fake.program('ExecutePluginCommand', { already: 'object' });
  setGateway(fake);
  result = await executePluginCommand('plug1', 'cmd1');
  assert((result as unknown as { already: string }).already === 'object', 'executePluginCommand passes object results through unchanged');

  // preparePluginViewPanel: returns token; throws when absent
  fake = createFakeGateway();
  fake.program('PreparePluginViewPanel', 'panel-token-xyz');
  setGateway(fake);
  const token = await preparePluginViewPanel('plug1', 'panel1');
  assert(token === 'panel-token-xyz', 'preparePluginViewPanel returns RPC result');

  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'PreparePluginViewPanel');
  setGateway(noMethod);
  threw = null;
  try { await preparePluginViewPanel('plug1', 'panel1'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'Plugin view relay is unavailable', 'preparePluginViewPanel throws when absent');

  // relayPluginViewMessage: serializes message; throws when absent
  fake = createFakeGateway();
  fake.program('RelayPluginViewMessage', undefined);
  setGateway(fake);
  await relayPluginViewMessage('tok1', { kind: 'ping', n: 1 });
  call = fake.calls.find((c) => c.method === 'RelayPluginViewMessage');
  assert(!!call && call.args[0] === 'tok1', 'relayPluginViewMessage forwards token');
  assert(!!call && call.args[1] === JSON.stringify({ kind: 'ping', n: 1 }), 'relayPluginViewMessage stringifies message');

  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'RelayPluginViewMessage');
  setGateway(noMethod);
  threw = null;
  try { await relayPluginViewMessage('tok', { a: 1 }); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'Plugin view relay is unavailable', 'relayPluginViewMessage throws when absent');

  // releasePluginViewPanel: fire-and-forget; no-op when absent
  fake = createFakeGateway();
  fake.program('ReleasePluginViewPanel', undefined);
  setGateway(fake);
  releasePluginViewPanel('tok1');
  assert(fake.calls.some((c) => c.method === 'ReleasePluginViewPanel' && c.args[0] === 'tok1'), 'releasePluginViewPanel forwards token');

  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'ReleasePluginViewPanel');
  setGateway(noMethod);
  releasePluginViewPanel('tok2'); // must not throw when method absent

  console.log('pluginRuntime.test.ts passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
