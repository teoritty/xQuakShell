import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { createTeam, inviteMember, lookupInvitee, confirmBinding } from './teamActions';

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

  console.log('teamActions: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
