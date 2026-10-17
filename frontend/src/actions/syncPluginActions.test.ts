import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  installOfferedPlugin,
  keepPlugin,
  loadPluginSync,
  restartPluginForData,
  setPluginDataSync,
} from './syncPluginActions';
import { NO_CONSENTS } from '../lib/plugins/installConsent';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

async function run() {
  // Every consent the dialog collected reaches the request under its own name, and nothing else.
  {
    const fake = createFakeGateway();
    fake.program('InstallSyncPluginOffer', { status: 'installed' });
    setGateway(fake);
    const res = await installOfferedPlugin('com.example.echo', { ...NO_CONSENTS, exec: true, network: true }, false);
    assert(res.status === 'installed' && res.error === '', 'a matching build installs');
    const req = fake.calls[0].args[0] as Record<string, unknown>;
    assert(req.pluginId === 'com.example.echo', 'the offered plugin id is sent');
    assert(req.exec === true && req.network === true, 'the given consents are sent');
    assert(req.secret === false && req.auth === false && req.tunnel === false && req.multiSession === false,
      'a consent not given is sent as false, never left out');
    assert(req.acceptDifferent === false, 'a different build is not accepted unless asked');
  }

  // A mismatching release is a result, not an error: the panel asks whether to keep it.
  {
    const fake = createFakeGateway();
    fake.program('InstallSyncPluginOffer', { status: 'mismatch' });
    setGateway(fake);
    const res = await installOfferedPlugin('com.example.echo', NO_CONSENTS, false);
    assert(res.status === 'mismatch' && res.error === '', 'the mismatch comes back as a status');
  }

  // Accepting a different build sends acceptDifferent.
  {
    const fake = createFakeGateway();
    fake.program('InstallSyncPluginOffer', { status: 'installed' });
    setGateway(fake);
    await installOfferedPlugin('com.example.echo', NO_CONSENTS, true);
    assert((fake.calls[0].args[0] as { acceptDifferent: boolean }).acceptDifferent === true, 'acceptDifferent is sent');
  }

  // A refusal is the message to show; no status pretends it installed.
  {
    const fake = createFakeGateway();
    fake.program('InstallSyncPluginOffer', () => {
      throw new Error('secret access consent required for this plugin');
    });
    setGateway(fake);
    const res = await installOfferedPlugin('com.example.echo', NO_CONSENTS, false);
    assert(res.status === '' && res.error.startsWith('secret access'), 'the backend refusal is returned');
  }

  // The panel's data loads both lists; a failure in one is reported, the other still shows.
  {
    const fake = createFakeGateway();
    fake.program('ListSyncPluginOffers', [{ pluginId: 'a', version: '1', state: 'install' }]);
    fake.program('ListSyncPluginData', () => {
      throw new Error('Unlock the vault first.');
    });
    setGateway(fake);
    const res = await loadPluginSync();
    assert(res.offers.length === 1 && res.data.length === 0, 'the working half still shows');
    assert(res.error === 'Unlock the vault first.', 'the failing half is reported');
  }

  // The per-plugin toggle, keep and restart each send one RPC and return '' on success.
  {
    const fake = createFakeGateway();
    setGateway(fake);
    assert((await setPluginDataSync('com.example.notes', false)) === '', 'toggle succeeds');
    assert((await keepPlugin('com.example.echo')) === '', 'keep succeeds');
    assert((await restartPluginForData('com.example.notes')) === '', 'restart succeeds');
    const methods = fake.calls.map((c) => c.method);
    assert(
      JSON.stringify(methods) === JSON.stringify(['SetSyncPluginData', 'KeepSyncPlugin', 'RestartSyncPlugin']),
      `one RPC each, got ${methods.join(',')}`,
    );
    assert(fake.calls[0].args[0] === 'com.example.notes' && fake.calls[0].args[1] === false, 'the toggle sends the plugin and off');
  }

  console.log('syncPluginActions: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
