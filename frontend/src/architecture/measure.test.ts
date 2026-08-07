import { blankComments, countCodeLines, measureSource, svelteScript } from './measure';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error('FAIL: ' + msg);
}

// blankComments must keep line numbers stable, which is the whole reason it
// exists alongside stripComments.
assert(blankComments('a\n/* x\n y */\nb').split('\n').length === 4, 'block comment keeps its newlines');
assert(blankComments('const u = "https://x";').includes('https://x'), 'a URL in a string is not a comment');
assert(blankComments('code(); // why').trim() === 'code();', 'trailing comment is blanked, code survives');
assert(blankComments('<!-- note -->\nx').split('\n')[1] === 'x', 'html comment is blanked');

assert(countCodeLines('a\n\n\nb') === 2, 'blank lines do not count');
assert(countCodeLines('// only a comment\ncode()') === 1, 'comment-only lines do not count');
assert(countCodeLines('code() // trailing') === 1, 'a trailing comment does not exempt its line');
assert(
  countCodeLines('a\n/* three\n   line\n   comment */\nb') === 2,
  'a multi-line comment costs nothing, so rationale stays free'
);

const component = [
  '<script lang="ts">',
  '  let x = 1;',
  '  // a note',
  '  let y = 2;',
  '</script>',
  '',
  '<!-- markup comment -->',
  '<div>{x}{y}</div>',
  '',
  '<style>',
  '  div { color: red; }',
  '</style>',
].join('\n');

assert(svelteScript(component).includes('let x = 1;'), 'script block is extracted');
assert(!svelteScript(component).includes('<div>'), 'markup is not part of the script');

const measured = measureSource('frontend/src/lib/X.svelte', component);
assert(measured.scriptCodeLines === 2, `script code lines = ${measured.scriptCodeLines}, want 2`);
assert(
  measured.codeLines === 8,
  `total code lines = ${measured.codeLines}, want 8 (markup and style count, comments do not)`
);

const plain = measureSource('frontend/src/api/x.ts', 'export const a = 1;\n// note\n');
assert(plain.scriptCodeLines === undefined, 'a .ts file has no separate script budget');
assert(plain.codeLines === 1, `code lines = ${plain.codeLines}, want 1`);

// HTML5 allows whitespace between the tag name and the `>` of an end tag, so
// each of these closes the script as far as a browser and the Svelte compiler
// are concerned. An end tag the pattern misses does not fail loudly: the block
// runs to the end of the file, matches nothing, and the component reports an
// empty script - which is a component the script and function budgets no longer
// measure at all.
const spacedEndTag = ['<script>', '  let x = 1;', '</script >', '<div>{x}</div>'].join('\n');
assert(svelteScript(spacedEndTag).includes('let x = 1;'), '`</script >` closes the block');

const newlineEndTag = ['<script>', '  let x = 1;', '</script', '>', '<div>{x}</div>'].join('\n');
assert(svelteScript(newlineEndTag).includes('let x = 1;'), '`</script\\n>` closes the block');

const tabEndTag = ['<script>', '  let x = 1;', '</script\t>', '<div>{x}</div>'].join('\n');
assert(svelteScript(tabEndTag).includes('let x = 1;'), '`</script\\t>` closes the block');

// The markup after the end tag must stay outside the script, or a missed end
// tag would be hidden by the extraction succeeding on too much.
assert(!svelteScript(spacedEndTag).includes('<div>'), 'markup after `</script >` is not script');

const spaced = measureSource('frontend/src/lib/Spaced.svelte', spacedEndTag);
assert(
  spaced.scriptCodeLines === 1,
  `script code lines = ${spaced.scriptCodeLines}, want 1; a space in the end tag must not hide the script`
);

console.log('measure.test passed');
