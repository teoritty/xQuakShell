import { internalDragHighlight } from './dragHighlight';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// No drag in progress: nothing is highlighted.
assert(internalDragHighlight(null, '/home/user') === 'none', 'null dragOverPath highlights nothing');

// The regression this function exists for. When the cursor is over a file row or over empty
// space, the pane handler sets dragOverPath to the pane's own current directory, and the drop
// lands in that directory — so the whole pane is the target and must be filled. This used to
// work on Windows only by accident: WebView2 reports 'Files' in dataTransfer.types even for
// internal drags, which tripped the OS-drop router's highlight. WebKitGTK does not, so the
// pane never filled on Linux. The highlight is now derived from our own drag state instead.
assert(
  internalDragHighlight('/home/user', '/home/user') === 'pane',
  'cursor over a file row or empty area highlights the whole pane',
);

// A folder row under the cursor is a more specific target than the pane, so it owns the
// highlight and the pane must not also fill. This mirrors highlightTargetAt() in osFileDrop.ts,
// which likewise prefers the row over the zone.
assert(
  internalDragHighlight('/home/user/docs', '/home/user') === 'row',
  'a folder row under the cursor takes the highlight instead of the pane',
);

// Root directory is not special-cased anywhere; it must behave like any other current path.
assert(internalDragHighlight('/', '/') === 'pane', 'root pane fills like any other directory');
assert(internalDragHighlight('/etc', '/') === 'row', 'a folder row under root still wins');

// Windows-style local paths flow through the same function from LocalFileTree, so equality must
// not assume forward slashes.
assert(
  internalDragHighlight('C:\\Users\\fedor', 'C:\\Users\\fedor') === 'pane',
  'windows local path fills the pane',
);
assert(
  internalDragHighlight('C:\\Users\\fedor\\Desktop', 'C:\\Users\\fedor') === 'row',
  'windows local folder row wins over the pane',
);

// An empty current path (pane not yet navigated) must not be treated as equal to a real target.
assert(internalDragHighlight('/home', '') === 'row', 'empty current path does not swallow a real target');

console.log('dragHighlight: all assertions passed');
