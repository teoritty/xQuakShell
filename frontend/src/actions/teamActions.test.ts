import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { get } from 'svelte/store';
import { createTeam, inviteMember, lookupInvitee, confirmBinding, moveConnectionsToVault } from './teamActions';
import { detailsConnectionId, favorites, selectedConnectionId, selectedConnectionIds } from '../stores/appState';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

async function run() {
  // A blank team name never reaches the backend.
  {
    const fake = createFakeGateway();
    setGateway(fake);
    const res = await createTeam('   ');
    assert(res.error !== '' && res.vault === '', 'a blank name is refused');
    assert(!fake.calls.some((c) => c.method === 'CreateTeam'), 'no RPC for a blank name');
  }

  // Creating trims the name and returns the new vault's id.
  {
    const fake = createFakeGateway();
    fake.program('CreateTeam', 'ab12');
    setGateway(fake);
    const res = await createTeam('  Ops ');
    assert(res.error === '' && res.vault === 'ab12', 'the new team id is returned');
    assert(fake.calls[0].args[0] === 'Ops', 'the name is trimmed before it is sent');
  }

  // The invite carries back exactly the fingerprint the lookup showed.
  {
    const fake = createFakeGateway();
    fake.program('LookupTeamInvitee', { username: 'bob', fingerprint: '11111 22222', known: false, verified: false });
    fake.program('InviteToTeam', undefined);
    setGateway(fake);
    const { invitee } = await lookupInvitee(' bob ');
    assert(invitee?.fingerprint === '11111 22222', 'the lookup returns the fingerprint to compare');
    assert(fake.calls[0].args[0] === 'bob', 'the username is trimmed');
    const err = await inviteMember('v1', invitee!, 'editor', true);
    const call = fake.calls.find((c) => c.method === 'InviteToTeam');
    const req = call?.args[0] as { fingerprint: string; role: string; verified: boolean };
    assert(err === '' && req.fingerprint === '11111 22222' && req.role === 'editor' && req.verified, 'the invite sends what was shown');
  }

  // A refusal comes back as the message to show.
  {
    const fake = createFakeGateway();
    fake.program('ConfirmCredentialBinding', () => {
      throw new Error('No credential of yours is bound to that connection.');
    });
    setGateway(fake);
    const err = await confirmBinding({ vault: 'v', connection: 'c', user: 'u' });
    assert(err.startsWith('No credential'), 'the backend message is returned');
  }

  // A moved connection has a new id, and everything that pointed at the old one follows it: the
  // open editor, the selection and a favourite. Otherwise moving the connection the editor shows
  // closed the editor, and a moved favourite silently stopped being one.
  {
    const fake = createFakeGateway();
    fake.program('MoveConnectionToVault', (id: unknown) => `new-${id}`);
    fake.program('GetAllConnections', []);
    setGateway(fake);
    favorites.set(new Set(['a', 'keep']));
    selectedConnectionIds.set(new Set(['a', 'b']));
    selectedConnectionId.set('a');
    detailsConnectionId.set('b');
    const err = await moveConnectionsToVault(['a', 'b'], 'v1', '');
    const moves = fake.calls.filter((c) => c.method === 'MoveConnectionToVault');
    assert(err === '' && moves.length === 2, 'each connection is moved');
    assert(JSON.stringify(moves[0].args) === JSON.stringify(['a', 'v1', '']), 'the destination and folder are sent as given');
    assert(get(detailsConnectionId) === 'new-b' && get(selectedConnectionId) === 'new-a', 'the editor and the selection follow the move');
    assert(get(selectedConnectionIds).has('new-b'), 'a multi-selection follows too');
    assert(get(favorites).has('new-a') && get(favorites).has('keep') && !get(favorites).has('a'), 'a favourite follows its connection');
    assert(fake.calls.some((c) => c.method === 'GetAllConnections'), 'the tree is re-read');
  }

  // The first refusal stops the batch and is returned; the moves already made still count.
  {
    const fake = createFakeGateway();
    fake.program('MoveConnectionToVault', (id: unknown) => {
      if (id === 'b') throw new Error('Your role in this team does not allow that.');
      return `new-${id}`;
    });
    fake.program('GetAllConnections', []);
    setGateway(fake);
    detailsConnectionId.set('a');
    const err = await moveConnectionsToVault(['a', 'b', 'c'], 'v1', '');
    assert(err.startsWith('Your role'), `error = ${err}`);
    assert(fake.calls.filter((c) => c.method === 'MoveConnectionToVault').length === 2, 'nothing is tried after the refusal');
    assert(get(detailsConnectionId) === 'new-a', 'the move that happened is still followed');
  }

  console.log('teamActions: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
