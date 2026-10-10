// The frontend half of forwarded-agent confirmation (ADR-022), driven the way the backend drives it:
// AgentConfirmRequired / AgentConfirmClosed in through the runtime, the answer out through the
// gateway. The Go half, through a real SSH handshake to the next host, is test/unit/agentforwarding.
import { setGateway, setRuntime } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import type { RuntimeGateway } from '../backend/gateway';
import { subscribeToEvents } from './subscribe';
import { pendingAgentConfirms, type AgentConfirmEvent } from '../stores/agentConfirmState';
import { lastError } from '../stores/appState';
import { resolveAgentConfirmRpc } from '../api/agentForwarding';
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

function request(requestId: string, over: Partial<AgentConfirmEvent> = {}): AgentConfirmEvent {
  return {
    requestId, sessionId: 's1', connectionName: 'bastion', host: 'bastion.corp',
    keyLabel: 'deploy', keyFingerprint: 'SHA256:k', remoteUser: 'git',
    verified: true, destination: 'SHA256:d', knownAs: ['git.corp'], forwardsFurther: false, windowMinutes: 15,
    ...over,
  };
}
const raise = (r: unknown) => handlers.get('AgentConfirmRequired')!(r);
const withdraw = (requestId: string) => handlers.get('AgentConfirmClosed')!({ requestId });
const queue = () => get(pendingAgentConfirms);

async function run() {
  assert(handlers.has('AgentConfirmRequired'), 'AgentConfirmRequired handler registered; without it a server waits for an answer nobody can give');
  assert(handlers.has('AgentConfirmClosed'), 'AgentConfirmClosed handler registered');

  // --- the queue ---

  pendingAgentConfirms.set([]);
  raise(request('a1'));
  raise(request('a2', { sessionId: 's2' }));
  assert(queue().map((q) => q.requestId).join() === 'a1,a2', 'two waiting requests are queued oldest first');
  raise(request('a1'));
  assert(queue().length === 2, 'a repeated request id does not stack a second dialog');

  raise(undefined);
  raise({ ...request('x'), requestId: '' });
  raise({ ...request('y'), sessionId: '' });
  assert(queue().length === 2, 'a request without a request or session id is never shown: it could not be answered');

  // An unverified destination must never offer a window, whatever the event says.
  raise(request('u1', { verified: false, windowMinutes: 15, knownAs: undefined as any }));
  const unverified = queue().find((q) => q.requestId === 'u1')!;
  assert(unverified.windowMinutes === 0, 'an unverified destination is offered no window even if the payload claims one');
  assert(Array.isArray(unverified.knownAs) && unverified.knownAs.length === 0, 'a missing known-hosts list is an empty one');
  raise(request('u2', { verified: 'yes' as any }));
  assert(queue().find((q) => q.requestId === 'u2')!.verified === false, 'only a literal true counts as verified');

  withdraw('a1');
  assert(!queue().some((q) => q.requestId === 'a1'), 'a request the backend withdrew leaves the screen');
  withdraw('never-raised');
  assert(queue().length === 3, 'withdrawing an unknown id changes nothing');

  // --- the answer ---

  let fake = createFakeGateway();
  fake.program('ResolveAgentConfirm', undefined);
  setGateway(fake);
  lastError.set(null);
  assert(await resolveAgentConfirmRpc('s2', 'a2', 'once') === true, 'an accepted answer reports success');
  const call = fake.calls.find((c) => c.method === 'ResolveAgentConfirm');
  assert(!!call && call.args.join() === 's2,a2,once', `the session, the request and the decision reach the backend; got ${call?.args}`);

  fake = createFakeGateway();
  fake.program('ResolveAgentConfirm', () => { throw new Error('this signature request is no longer waiting for an answer'); });
  setGateway(fake);
  lastError.set(null);
  assert(await resolveAgentConfirmRpc('s2', 'a2', 'once') === false, 'a refused answer reports failure');
  assert(get(lastError) !== null, 'a refused answer is shown to the user');

  setGateway(null);
  assert(await resolveAgentConfirmRpc('s2', 'a2', 'deny') === false, 'without a backend the answer is not claimed delivered');

  console.log('agentConfirm.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
