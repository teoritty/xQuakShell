import {
  defaultSelection,
  toggleAlias,
  selectAll,
  masterState,
  selectedKeyCount,
  countDuplicates,
  describeHost,
  importButtonLabel,
  describeNotice,
  describeResult,
} from './importSelection';
import type { SSHConfigHost } from '../../api/sshConfig';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

function host(partial: Partial<SSHConfigHost> & { alias: string }): SSHConfigHost {
  return {
    hostName: partial.alias,
    port: 22,
    user: '',
    keyCount: 0,
    jumpAliases: [],
    duplicate: false,
    ...partial,
  };
}

const hosts: SSHConfigHost[] = [
  host({ alias: 'web', hostName: 'web.example.com', user: 'deploy', keyCount: 1 }),
  host({ alias: 'db', hostName: 'db.example.com', duplicate: true, keyCount: 2 }),
  host({ alias: 'cache', hostName: 'cache.example.com', port: 2222 }),
];

function run() {
  // Duplicates start unselected so a re-import offers only what is new.
  const initial = defaultSelection(hosts);
  assert(initial.has('web') && initial.has('cache'), 'new hosts are selected by default');
  assert(!initial.has('db'), 'duplicates are not selected by default');
  assert(countDuplicates(hosts) === 1, 'countDuplicates counts flagged rows');

  // A duplicate can still be opted in.
  const withDup = toggleAlias(initial, 'db');
  assert(withDup.has('db'), 'toggling adds an unselected alias');
  assert(!initial.has('db'), 'toggleAlias does not mutate its input');
  assert(!toggleAlias(withDup, 'db').has('db'), 'toggling again removes it');

  assert(masterState(hosts, new Set()) === 'none', 'empty selection is none');
  assert(masterState(hosts, initial) === 'some', 'partial selection is some');
  assert(masterState(hosts, selectAll(hosts)) === 'all', 'full selection is all');
  assert(selectAll(hosts).size === 3, 'selectAll takes every host');

  assert(selectedKeyCount(hosts, initial) === 1, 'key count follows the selection');
  assert(selectedKeyCount(hosts, selectAll(hosts)) === 3, 'key count sums the selected rows');
  assert(selectedKeyCount(hosts, new Set()) === 0, 'no selection reads no keys');

  assert(describeHost(hosts[0]) === 'deploy@web.example.com', 'default port is not shown');
  assert(describeHost(hosts[1]) === 'db.example.com', 'a missing user is omitted');
  assert(describeHost(hosts[2]) === 'cache.example.com:2222', 'a non-default port is shown');

  assert(importButtonLabel(0) === 'Select hosts to import', 'zero selection prompts instead of counting');
  assert(importButtonLabel(1) === 'Import 1 connection', 'one is singular');
  assert(importButtonLabel(4) === 'Import 4 connections', 'many is plural');

  // Every notice kind must produce real prose; a raw identifier reaching the
  // user would mean a backend kind the UI was never taught.
  const kinds = [
    'matchBlockSkipped',
    'proxyCommandUnsupported',
    'includeUnreadable',
    'identityFileMissing',
    'jumpHostUnresolved',
    'limitReached',
  ] as const;
  for (const kind of kinds) {
    const text = describeNotice({ kind, target: 'web' });
    assert(text.length > 20, `notice ${kind} has a real sentence`);
    assert(!text.includes(kind), `notice ${kind} does not leak its identifier`);
  }
  const unknown = describeNotice({ kind: 'somethingNew', target: '' });
  assert(unknown.length > 20 && !unknown.includes('somethingNew'), 'an unknown kind degrades to prose');
  assert(describeNotice({ kind: 'includeUnreadable', target: '' }).indexOf('()') === -1, 'an empty target adds no empty parens');

  assert(
    describeResult({ connections: [1], importedKeys: 0, failedKeys: 0, skippedAliases: [] }) ===
      '1 connection imported.',
    'a plain result reads simply'
  );
  const full = describeResult({ connections: [1, 2], importedKeys: 2, failedKeys: 1, skippedAliases: ['x'] });
  assert(full.includes('2 connections imported'), 'result counts connections');
  assert(full.includes('2 keys added'), 'result counts imported keys');
  assert(full.includes('1 key could not be read'), 'result reports failed keys');
  assert(full.includes('1 no longer in the config'), 'result reports skipped aliases');

  console.log('importSelection tests passed');
}

run();
