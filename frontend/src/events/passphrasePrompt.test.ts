// The frontend half of the key passphrase prompt, driven the way the backend drives it: events in
// through the runtime, answers out through the gateway. The Go half, from the prompt to a real SSH
// handshake, is test/unit/passphraseprompt.
import { setGateway, setRuntime } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import type { RuntimeGateway } from '../backend/gateway';
import { subscribeToEvents } from './subscribe';
import { pendingPassphrasePrompts } from '../stores/passphrasePromptState';
import { lastError } from '../stores/appState';
import { resolvePassphraseRpc, cancelPassphraseRpc } from '../api/sessions';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const handlers = new Map<string, (data: any) => void>();
const runtime: RuntimeGateway = {
  EventsOn(event: string, cb: (data: any) => void) {
    handlers.set(event, cb);
  },
  BrowserOpenURL() {},
};
setRuntime(runtime);
subscribeToEvents();

const raise = (requestId: string, label = 'prod key') =>
  handlers.get('PassphraseRequired')!({ requestId, identityId: 'id-' + requestId, label });
const withdraw = (requestId: string) => handlers.get('PassphrasePromptClosed')!({ requestId });
const queueIds = () => get(pendingPassphrasePrompts).map((p) => p.requestId);

async function run() {
  assert(handlers.has('PassphraseRequired'), 'PassphraseRequired handler registered; without it the backend asks and nobody hears');
  assert(handlers.has('PassphrasePromptClosed'), 'PassphrasePromptClosed handler registered');

  // --- the queue ---

  pendingPassphrasePrompts.set([]);
  raise('r1', 'deploy key');
  raise('r2');
  assert(queueIds().join() === 'r1,r2', `two waiting sessions keep two prompts, oldest first; got ${queueIds()}`);
  assert(get(pendingPassphrasePrompts)[0].label === 'deploy key', 'the prompt carries the key label the dialog shows');

  raise('r1', 'deploy key');
  assert(queueIds().length === 2, 'a repeated request id does not stack a second dialog');

  handlers.get('PassphraseRequired')!({ identityId: 'x', label: 'no id' });
  handlers.get('PassphraseRequired')!(undefined);
  assert(queueIds().length === 2, 'a prompt without a request id is ignored: it could never be answered');

  // The backend withdraws a prompt whose session ended; the dialog must go with it.
  withdraw('r1');
  assert(queueIds().join() === 'r2', `withdrawing one prompt leaves the other; got ${queueIds()}`);
  withdraw('never-raised');
  assert(queueIds().join() === 'r2', 'withdrawing an unknown id changes nothing');

  // --- the answers ---

  let fake = createFakeGateway();
  fake.program('ResolvePassphrase', undefined);
  setGateway(fake);
  lastError.set(null);
  assert(await resolvePassphraseRpc('r2', 'pw-secret') === true, 'a passphrase the backend accepted reports success');
  const call = fake.calls.find((c) => c.method === 'ResolvePassphrase');
  assert(!!call && call.args[0] === 'r2' && call.args[1] === 'pw-secret', 'the request id and the typed passphrase reach the backend, and nothing else');
  assert(call!.args.length === 2, 'no key id or session id travels with the answer; the backend already holds them');
  assert(get(lastError) === null, 'no error for an accepted passphrase');

  // A refusal is shown to the user and the passphrase is not part of what is shown.
  fake = createFakeGateway();
  fake.program('ResolvePassphrase', () => { throw new Error('no passphrase prompt is waiting for this request'); });
  setGateway(fake);
  lastError.set(null);
  assert(await resolvePassphraseRpc('r2', 'pw-secret') === false, 'a refused passphrase reports failure so the dialog stays up');
  assert(get(lastError) !== null, 'a refused passphrase is reported to the user');
  assert(!JSON.stringify(get(lastError)).includes('pw-secret'), 'the error shown never contains the passphrase');

  fake = createFakeGateway();
  fake.program('CancelPassphrase', undefined);
  setGateway(fake);
  assert(await cancelPassphraseRpc('r2') === true, 'cancel reports success');
  const cancelCall = fake.calls.find((c) => c.method === 'CancelPassphrase');
  assert(!!cancelCall && cancelCall.args.length === 1 && cancelCall.args[0] === 'r2', 'cancel sends only the request id');

  setGateway(null);
  assert(await resolvePassphraseRpc('r2', 'x') === false, 'without a backend the answer is not claimed delivered');

  console.log('passphrasePrompt.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
