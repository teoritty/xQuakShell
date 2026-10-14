import { normalizeFingerprint, parseSyncLink, sameFingerprint } from './syncLink';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const groups = '91796 59138 05751 85871 52528 03481 07460 08300 23148 85620 11178 34775';
const dashed = groups.replace(/ /g, '-');

// The link xqsyncd prints: the address and the fingerprint, both recovered exactly.
{
  const link = parseSyncLink(`https://sync.example.org:8443/#fp=${dashed}`);
  assert(link?.url === 'https://sync.example.org:8443', `url = ${link?.url}; the fragment and the trailing slash are not part of the address`);
  assert(link?.fingerprint === groups, `fingerprint = ${link?.fingerprint}; the link's fingerprint is what the probe is held to`);
}

// A bare address carries no fingerprint, so the person compares it themselves.
{
  const link = parseSyncLink('https://127.0.0.1:18443');
  assert(link?.url === 'https://127.0.0.1:18443' && link.fingerprint === '', 'a bare address has no fingerprint to check against');
}
assert(parseSyncLink('sync.example.org:8443')?.url === 'https://sync.example.org:8443', 'a host without a scheme is https');
assert(parseSyncLink('  https://sync.example.org/base/  ')?.url === 'https://sync.example.org/base', 'a path is kept, whitespace dropped');

// Nothing but https is a sync server, and credentials in a URL are never sent anywhere.
for (const bad of ['http://sync.example.org', 'ftp://sync.example.org', 'https://user:pw@sync.example.org', '', '   ', 'https://']) {
  assert(parseSyncLink(bad) === null, `${JSON.stringify(bad)} must not parse as a sync server`);
}

// A truncated or padded fingerprint in a link is no fingerprint: the probe is then compared by the
// person, never accepted against a partial match.
assert(parseSyncLink(`https://s.example/#fp=${dashed.slice(0, -1)}`)?.fingerprint === '', 'one digit short is not a fingerprint');
assert(parseSyncLink(`https://s.example/#fp=${dashed}-1`)?.fingerprint === '', 'one digit long is not a fingerprint');

assert(normalizeFingerprint(groups.replace(/ /g, '')) === groups, 'digits alone are grouped by five');
assert(normalizeFingerprint('abcde') === '', 'letters are not a fingerprint');
assert(sameFingerprint(groups, dashed), 'spacing does not matter');
assert(!sameFingerprint(groups, groups.replace('91796', '91797')), 'one digit apart is a different server');
assert(!sameFingerprint('', ''), 'two absent fingerprints are not a match');

console.log('syncLink tests passed');
