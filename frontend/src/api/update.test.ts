import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { fetchUpdateStatus, NO_UPDATE_STATUS } from './update';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  const available = {
    currentVersion: '1.0.0',
    latestVersion: '1.0.1',
    releaseUrl: 'https://example.test/releases/1.0.1',
    updateAvailable: true,
    checked: true,
  };
  fake.program('GetUpdateStatus', available);
  let status = await fetchUpdateStatus();
  assert(status.updateAvailable && status.latestVersion === '1.0.1', 'status is returned as the backend reported it');
  assert(get(lastError) === null, 'a successful check reports no error');

  // A build whose backend has no update service answers with nothing. The wrapper must degrade to
  // "we never looked" rather than hand back an undefined the banner would read as false.
  fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);
  fake.program('GetUpdateStatus', undefined);
  status = await fetchUpdateStatus();
  assert(status.checked === false && status.updateAvailable === false, 'a missing binding yields the no-update status');

  // Being offline is the ordinary case for this call, and it must never look like an update.
  fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);
  fake.program('GetUpdateStatus', () => { throw new Error('boom'); });
  status = await fetchUpdateStatus();
  assert(status.updateAvailable === false, 'a failed check never claims an update exists');
  assert(status.currentVersion === NO_UPDATE_STATUS.currentVersion, 'a failed check falls back to the empty status');

  console.log('update.test.ts: all passed');
}

run().catch((e) => { console.error(e); process.exit(1); });
