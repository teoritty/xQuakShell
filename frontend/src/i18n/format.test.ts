import assert from 'node:assert/strict';
import { get } from 'svelte/store';
import { interpolate, pluralKey } from './format';
import { currentLocale, messages, t, translate } from './messages';

{
  assert.equal(interpolate('Copied {n} files', { n: 3 }), 'Copied 3 files');
  assert.equal(interpolate('No variables here', { n: 3 }), 'No variables here');
  assert.equal(interpolate('Plain', undefined), 'Plain');
}

// A placeholder the translator misspelled is left standing so they can see it. Blanking it would
// produce a sentence with a hole and no clue where the hole came from.
{
  assert.equal(interpolate('Copied {n} of {tota}', { n: 1, total: 9 }), 'Copied 1 of {tota}');
}

// The substitution must produce text and nothing else: a pack on disk is untrusted, and this is the
// property that stops one from injecting markup.
{
  const out = interpolate('Deleting {name}', { name: '<img src=x onerror=alert(1)>' });
  assert.equal(
    out,
    'Deleting <img src=x onerror=alert(1)>',
    'the value is carried through verbatim as text; the template layer escapes it on render',
  );
}

// Russian needs three forms chosen by rules no `count === 1` covers: 1/21/101 take one form,
// 2 to 4 another, 5 to 20 a third.
{
  const ru = {
    'files.selected_one': 'файл',
    'files.selected_few': 'файла',
    'files.selected_many': 'файлов',
  };
  assert.equal(pluralKey(ru, 'files.selected', 'ru', 1), 'files.selected_one');
  assert.equal(pluralKey(ru, 'files.selected', 'ru', 21), 'files.selected_one');
  assert.equal(pluralKey(ru, 'files.selected', 'ru', 3), 'files.selected_few');
  assert.equal(pluralKey(ru, 'files.selected', 'ru', 5), 'files.selected_many');
  assert.equal(pluralKey(ru, 'files.selected', 'ru', 11), 'files.selected_many');
}

{
  const en = { 'files.selected_one': 'file', 'files.selected_other': 'files' };
  assert.equal(pluralKey(en, 'files.selected', 'en', 1), 'files.selected_one');
  assert.equal(pluralKey(en, 'files.selected', 'en', 2), 'files.selected_other');
}

// A partially translated plural still has to render a sentence.
{
  const partial = { 'files.selected_other': 'files' };
  assert.equal(
    pluralKey(partial, 'files.selected', 'ru', 3),
    'files.selected_other',
    'a missing category falls through to _other rather than to a raw key',
  );
}

{
  assert.equal(
    pluralKey({ 'k_other': 'x' }, 'k', 'not-a-locale', 2),
    'k_other',
    'an unrecognised locale tag falls back to English rules instead of throwing',
  );
}

{
  messages.set({
    'settings.tab.files': 'Файлы',
    'files.selected_one': 'Выбран {count} файл',
    'files.selected_few': 'Выбрано {count} файла',
    'files.selected_many': 'Выбрано {count} файлов',
  });
  currentLocale.set('ru');

  const translateNow = get(t);
  assert.equal(translateNow('settings.tab.files'), 'Файлы');
  assert.equal(translateNow('files.selected', { count: 1 }), 'Выбран 1 файл');
  assert.equal(translateNow('files.selected', { count: 3 }), 'Выбрано 3 файла');
  assert.equal(translateNow('files.selected', { count: 7 }), 'Выбрано 7 файлов');
  assert.equal(translate('settings.tab.files'), 'Файлы', 'the non-store form reads the same catalogue');
}

// The backend lays English under every language, so reaching this means the key is in no pack at
// all - a missing string in the source, which must be visible rather than swallowed.
{
  messages.set({});
  assert.equal(get(t)('nothing.claims.this'), 'nothing.claims.this');
}

console.log('i18n format.test passed');
