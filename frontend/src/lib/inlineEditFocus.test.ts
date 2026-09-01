// Source-scanning guard, in the spirit of architecture.test.ts rule 3.
//
// An inline editor that commits on blur is only dismissable if something focused
// it in the first place. Three of them - the folder rename, the connection rename
// and the tag input - opened unfocused, so clicking away fired no blur and the
// row stayed in edit mode for the rest of the session. The tell was that clicking
// *into* the input first made it behave: the handler was correct all along and
// simply never ran.
//
// The HTML `autofocus` attribute does not fix it either; focusSelect.ts exists
// because that attribute is unreliable here. So the rule is a test: an input that
// closes on blur must carry use:focusSelect.
//
// The second rule here is the other half of the same idea: while an inline editor is open it owns
// the keyboard, so its keydown must not reach the row behind it. The connection tree's rename input
// did not stop it, and the row's own Enter opens a session - so applying a rename with Enter renamed
// the connection and connected to it on the one keypress, and pressing Delete mid-rename offered to
// delete the connection being renamed. The file trees already did this; the two remote-tree rows had
// drifted from it. Guarding at the row instead does not work: the editor clears its own editing flag
// on Enter before the event has finished bubbling, so the row sees a row that is no longer editing.
//
// Components cannot be rendered in this suite (no DOM, no Svelte runtime), so the
// link is asserted on the source, the technique discoveryMarkup.test.ts uses.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}

const HERE = dirname(fileURLToPath(import.meta.url));

// Every component with an inline editor that opens in place and closes on blur.
const INLINE_EDITORS = [
  join(HERE, 'remoteTree', 'RemoteTreeFolderRow.svelte'),
  join(HERE, 'remoteTree', 'RemoteTreeConnectionRow.svelte'),
  join(HERE, 'connectionDetails', 'ConnectionTags.svelte'),
  join(HERE, 'FileTreeNode.svelte'),
  join(HERE, 'LocalFileTreeNode.svelte'),
];

function stripToFixpoint(src: string, re: RegExp): string {
  let prev: string;
  do {
    prev = src;
    src = src.replace(re, '');
  } while (src !== prev);
  return src;
}

// The comments in these files talk about blur and focus; strip them so the prose
// explaining the rule does not satisfy the rule.
function stripComments(src: string): string {
  let out = stripToFixpoint(src, /<!--[\s\S]*?-->/g);
  out = stripToFixpoint(out, /\/\*[\s\S]*?\*\//g);
  return out.replace(/(^|[^:])\/\/.*$/gm, '$1');
}

// Splits the source into <input ...> tags, ending each at a '>' that is not
// inside an attribute expression.
//
// A plain /<input\b[^>]*>/ is wrong here and quietly so: every one of these
// inputs has an arrow function in a handler, and the '>' of '=>' ends the match
// early. On the tag input that arrow sits before on:blur, so the truncated tag
// looked like it had no blur handler and the guard skipped the very component
// the bug was found in. Tracking brace depth is what makes the scan see the
// whole tag.
function inputTags(src: string): string[] {
  const tags: string[] = [];
  const OPEN = '<input';

  for (let start = src.indexOf(OPEN); start !== -1; start = src.indexOf(OPEN, start + 1)) {
    let depth = 0;
    for (let i = start + OPEN.length; i < src.length; i++) {
      const c = src[i];
      if (c === '{') depth++;
      else if (c === '}') depth--;
      else if (c === '>' && depth === 0) {
        tags.push(src.slice(start, i + 1));
        break;
      }
    }
  }
  return tags;
}

let checked = 0;

for (const file of INLINE_EDITORS) {
  const code = stripComments(readFileSync(file, 'utf8'));
  const tags = inputTags(code);

  assert(tags.length > 0, `${file} has no <input>; this list is stale and the guard now covers nothing`);

  for (const tag of tags) {
    if (!/\bon:blur\b/.test(tag)) continue;
    checked++;
    assert(
      /\buse:focusSelect\b/.test(tag),
      `${file} has an input that closes on blur but is never focused, so clicking away leaves it ` +
        `open forever. Add use:focusSelect - see frontend/src/lib/focusSelect.ts.`
    );
    assert(
      /\bon:keydown\|[^=]*\bstopPropagation\b/.test(tag),
      `${file} has an inline editor whose keydown reaches the row behind it, so one Enter both ` +
        `applies the edit and fires the row's own verb. Write on:keydown|stopPropagation.`
    );
  }

  if (/\buse:focusSelect\b/.test(code)) {
    assert(
      /from '\.{1,2}(\/\.\.)*\/?focusSelect'/.test(code) || /from '.*focusSelect'/.test(code),
      `${file} uses focusSelect without importing it`
    );
  }
}

// If the blur-closing inputs ever stop existing, the loop above passes by finding
// nothing to check. Naming the number keeps that silence from looking like health.
assert(
  checked >= 5,
  `only ${checked} blur-closing inputs were checked, expected at least 5; ` +
    `either an inline editor lost its blur handler or this list has drifted from the components`
);

console.log('inlineEditFocus.test.ts: all passed');
