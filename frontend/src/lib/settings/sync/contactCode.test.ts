import { codeFingerprint, formatContactCode, parseInviteInput } from './contactCode';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const groups = '91796 59138 05751 85871 52528 03481 07460 08300 23148 85620 11178 34775';
const dashed = groups.replace(/ /g, '-');

// The code carries the username to look up and the fingerprint to hold the lookup to, and reads
// back exactly.
{
  const code = formatContactCode('alice', groups);
  assert(code === `xqs-member:alice#fp=${dashed}`, `code = ${code}; it is the username and the dashed fingerprint`);
  const parsed = parseInviteInput(code);
  assert(parsed?.username === 'alice' && parsed.fingerprint === groups, `parsed = ${JSON.stringify(parsed)}; a code reads back as it was made`);
}

// A username that is not URL-safe survives the round trip rather than splitting the code.
{
  const parsed = parseInviteInput(formatContactCode('ops team#2', groups));
  assert(parsed?.username === 'ops team#2' && parsed.fingerprint === groups, `parsed = ${JSON.stringify(parsed)}; '#' in a username is not the fragment`);
}

// Without both halves there is no code to hand out: an empty one is shown as nothing to copy.
assert(formatContactCode('', groups) === '', 'no username, no code');
assert(formatContactCode('alice', '12345') === '', 'no whole fingerprint, no code');

// A plain username still looks the account up; the person then compares by hand.
{
  const parsed = parseInviteInput('  bob ');
  assert(parsed?.username === 'bob' && parsed.fingerprint === '', `parsed = ${JSON.stringify(parsed)}; a username carries nothing to check`);
}
assert(parseInviteInput('   ') === null, 'blank input is nothing to look up');

// A damaged code is refused, never read as a username: that would look the account up and skip
// the very check the code was sent to make.
for (const bad of [
  `xqs-member:alice#fp=${dashed.slice(0, -1)}`,
  `xqs-member:alice#fp=${dashed}-1`,
  `xqs-member:alice`,
  `xqs-member:#fp=${dashed}`,
  `xqs-member:%E0%A4%A#fp=${dashed}`,
]) {
  assert(parseInviteInput(bad) === null, `${bad} must not parse`);
}
assert(parseInviteInput(`XQS-MEMBER:alice#fp=${dashed}`)?.fingerprint === groups, 'the prefix is not case sensitive');

// Verifying a member takes their code or the bare fingerprint they sent; anything else is no
// fingerprint at all.
assert(codeFingerprint(formatContactCode('alice', groups)) === groups, 'a code yields its fingerprint');
assert(codeFingerprint(dashed) === groups, 'a bare fingerprint is accepted too');
assert(codeFingerprint(`xqs-member:alice#fp=${dashed.slice(0, -1)}`) === '', 'a truncated code yields nothing');
assert(codeFingerprint('alice') === '', 'a username is not a fingerprint');

console.log('contactCode tests passed');
