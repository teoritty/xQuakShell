import { maxNesting, measureFuncs, svelteAsTypeScript } from './funcShapes';
import ts from 'typescript';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error('FAIL: ' + msg);
}

function nestingOf(src: string): number {
  return maxNesting(ts.createSourceFile('x.ts', src, ts.ScriptTarget.ES2020, true));
}

assert(nestingOf('const a = 1;') === 0, 'straight line code has no nesting');
assert(nestingOf('if (a) {} else if (b) {} else if (c) {}') === 1, 'an else-if chain stays at one level');
assert(nestingOf('if (a) {} else { if (b) {} }') === 2, 'a plain else opens a level');
assert(nestingOf('switch (a) { case 1: break; case 2: break; }') === 1, 'a switch is one level whatever its case count');
assert(nestingOf('switch (a) { case 1: for (;;) {} }') === 2, 'statements inside a case nest below the switch');
assert(nestingOf('for (;;) { if (a) { for (;;) {} } }') === 3, 'for/if/for is three levels');
assert(nestingOf('{ const a = 1; }') === 0, 'a bare block is not control flow');

const shapes = measureFuncs(
  'x.ts',
  ['export function outer(a: number, b: number): number {', '  const inner = () => a + b;', '  return inner();', '}'].join('\n')
);
assert(shapes.get('x.ts::outer')?.params === 2, 'parameters are counted');
assert(shapes.get('x.ts::outer')?.codeLines === 2, `body code lines = ${shapes.get('x.ts::outer')?.codeLines}, want 2`);
assert(shapes.has('x.ts::inner'), 'an arrow bound to a const is a named function');

const component = ['<div>hi</div>', '', '<script lang="ts">', '  function handle(e: Event) {', '    console.log(e);', '  }', '</script>'].join('\n');
const asTs = svelteAsTypeScript(component);
assert(asTs.split('\n').length === component.split('\n').length, 'blanking markup preserves line numbers');
const svelteShapes = measureFuncs('X.svelte', component);
assert(svelteShapes.get('X.svelte::handle')?.codeLines === 1, 'a component handler is measured');

console.log('funcShapes.test passed');
