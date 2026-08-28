import assert from 'node:assert/strict';
import { formatConnection, formatTimestamp, isDenied } from './auditEntryView';
import type { AuditEntry } from '../api/settings';

function entry(overrides: Partial<AuditEntry>): AuditEntry {
  return {
    id: 1,
    timestamp: '2026-08-20T10:30:00Z',
    category: 'command',
    sessionId: '',
    connectionId: '',
    connectionName: '',
    host: '',
    username: '',
    input: '',
    ...overrides,
  } as AuditEntry;
}

{
  assert.equal(isDenied(entry({ input: 'plugin vault.getSecret result=denied' })), true);
  assert.equal(isDenied(entry({ input: 'plugin vault.getSecret result=allowed' })), false);
}

// A record is worth more than a tidy column: an unparseable timestamp is shown as it was written
// rather than as "Invalid Date", which would lose it.
{
  assert.equal(formatTimestamp('not a date', 'en'), 'not a date');
}

// The interface language decides the order, so the same instant reads differently in each - which
// is the whole reason the locale is a parameter rather than the platform default.
{
  const en = formatTimestamp('2026-08-20T10:30:00Z', 'en-US');
  const ru = formatTimestamp('2026-08-20T10:30:00Z', 'ru-RU');
  assert.notEqual(en, ru, 'a formatted timestamp must follow the locale it is given');
}

{
  assert.equal(formatConnection(entry({ connectionName: 'web', host: 'h1' })), 'web @ h1');
  assert.equal(formatConnection(entry({ connectionName: 'web' })), 'web');
  assert.equal(formatConnection(entry({ host: 'h1' })), 'h1');
  assert.equal(formatConnection(entry({})), '', 'an entry that recorded neither shows nothing');
}

console.log('OK auditEntryView');
