// Source-scanning guard, in the spirit of discoveryMarkup.test.ts.
//
// style.css normalises every form control, and it deliberately states checkbox and radio sizing
// at the specificity of a bare `input` (0,0,1) so a component can still override it. That makes
// the rule easy to lose by accident: any shared stylesheet rule written as
// `.some-wrapper input { width: 100% }` scores (0,1,1), beats the normalisation, and stretches
// the checkbox or radio to the width of its row.
//
// That is not a cosmetic difference. In the connection users list the stretched radio consumed
// the whole flex basis of its label, the "Default" text next to it was flexed down to zero
// width, and — being nowrap — spilled out of its own box and painted over the delete button.
//
// A reviewer cannot keep noticing this forever, so the rule is a test: in the shared (unscoped)
// stylesheets, a sizing declaration must never reach a bare `input` descendant without excluding
// checkboxes and radios.
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}

const HERE = dirname(fileURLToPath(import.meta.url));
const SRC = join(HERE, '..', '..');
const GLOBAL_CSS = join(SRC, 'style.css');

function cssFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...cssFiles(full));
    else if (entry.name.endsWith('.css')) out.push(full);
  }
  return out;
}

// Strip comments so prose about `input { width }` is not read as a rule.
function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, '');
}

type Rule = { prelude: string; selectors: string[]; body: string };

// Only top-level commas separate selectors: `input:where([type="a"], [type="b"])` is one.
function splitSelectors(prelude: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let current = '';
  for (const ch of prelude) {
    if (ch === '(' || ch === '[') depth++;
    else if (ch === ')' || ch === ']') depth--;
    if (ch === ',' && depth === 0) {
      out.push(current);
      current = '';
      continue;
    }
    current += ch;
  }
  out.push(current);
  return out.map((s) => s.trim()).filter(Boolean);
}

function rules(css: string): Rule[] {
  const out: Rule[] = [];
  const re = /([^{}]+)\{([^{}]*)\}/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(css)) !== null) {
    if (m[1].includes('@')) continue; // at-rule preludes carry no selector
    out.push({ prelude: m[1].trim(), selectors: splitSelectors(m[1]), body: m[2] });
  }
  return out;
}

// Declarations that resize the control's box. flex-basis is included because on a flex item it
// is what actually decides the used width.
const SIZING_RE = /(^|[;\s])(width|min-width|flex-basis|flex)\s*:/;

// The selector's rightmost compound, with any :where()/:not() argument lists removed so their
// contents are not mistaken for the subject itself.
function subject(selector: string): string {
  let s = selector;
  let prev = '';
  while (s !== prev) {
    prev = s;
    s = s.replace(/\([^()]*\)/g, '');
  }
  return s.split(/[\s>+~]+/).filter(Boolean).pop() || '';
}

function targetsBareInput(selector: string): boolean {
  const last = subject(selector);
  // `input`, `input:focus`, `input::placeholder` — a type selector with no [type=…] narrowing.
  return /^input(?![\w-])/.test(last) && !last.includes('[type');
}

function excludesWidgets(selector: string): boolean {
  return selector.includes('[type="checkbox"]') && selector.includes('[type="radio"]');
}

const shared = cssFiles(SRC).filter((f) => f !== GLOBAL_CSS);
assert(shared.length > 0, 'no shared stylesheets found — the scan would pass vacuously');

const offenders: string[] = [];
for (const file of shared) {
  for (const rule of rules(stripComments(readFileSync(file, 'utf8')))) {
    if (!SIZING_RE.test(rule.body)) continue;
    for (const sel of rule.selectors) {
      if (targetsBareInput(sel) && !excludesWidgets(sel)) {
        offenders.push(`${relative(SRC, file)}: ${sel}`);
      }
    }
  }
}
assert(
  offenders.length === 0,
  'shared CSS sizes a bare `input` descendant without excluding checkbox/radio, which overrides ' +
    'the widget sizing in style.css and stretches the control:\n  ' + offenders.join('\n  '),
);

// The normalisation the rule above protects has to keep existing, and keep costing no more
// specificity than a bare element selector — :where() is what holds that.
const globalCss = stripComments(readFileSync(GLOBAL_CSS, 'utf8'));
// The rule that selects the widgets, not the one that excludes them from the text-field reset —
// both name the same two types, so the negation form has to be filtered out.
const widgetRule = rules(globalCss).find(
  (r) =>
    r.prelude.startsWith('input:where(') &&
    r.prelude.includes('[type="checkbox"]') &&
    r.prelude.includes('[type="radio"]') &&
    !r.prelude.includes(':not('),
);
assert(
  widgetRule !== undefined,
  'style.css no longer normalises checkbox/radio via `input:where([type="checkbox"], [type="radio"])`',
);
assert(
  /(^|[;\s])width\s*:/.test(widgetRule!.body),
  'the checkbox/radio normalisation in style.css no longer states a width, so nothing pins the widget box',
);

console.log('sharedControlWidth: ok');
