// frontend/src/lib/fileTree/dragPayload.test.ts
import { readDragPayload, isMultiDrag, type DragDataReader } from './dragPayload';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}
// Stands in for DataTransfer.getData, which returns '' for a key that was never set.
function reader(data: Record<string, string>): DragDataReader {
  return (key: string) => data[key] ?? '';
}

// --- a single-row remote drag ---
let p = readDragPayload(reader({ 'text/session-id': 's1', 'text/remote-path': '/a/b' }));
assert(p.sessionId === 's1', 'the session id comes through');
assert(p.remotePaths.join() === '/a/b', 'a single remote path reads as a one-element list');
assert(p.localPaths.length === 0, 'nothing invents local paths');

// --- a multi-row remote drag ---
p = readDragPayload(reader({ 'text/session-id': 's1', 'text/selected-paths': '["/a","/b"]' }));
assert(p.remotePaths.join() === '/a,/b', 'a JSON array reads as the whole selection');

// --- the list key wins over the single key ---
p = readDragPayload(reader({ 'text/selected-paths': '["/a"]', 'text/remote-path': '/ignored' }));
assert(p.remotePaths.join() === '/a', 'a present selection list takes precedence over the single path');

// --- local drags read the same way ---
p = readDragPayload(reader({ 'text/local-path': 'C:\\x' }));
assert(p.localPaths.join() === 'C:\\x', 'a single local path reads as a one-element list');
p = readDragPayload(reader({ 'text/local-selected-paths': '["C:\\\\x","C:\\\\y"]' }));
assert(p.localPaths.length === 2, 'a local selection list reads as the whole selection');

// --- a drag with nothing in it ---
p = readDragPayload(reader({}));
assert(p.sessionId === '' && p.remotePaths.length === 0 && p.localPaths.length === 0, 'an empty drag yields empty everything');

// --- malformed payloads do nothing rather than throwing ---
p = readDragPayload(reader({ 'text/selected-paths': 'not json', 'text/local-path': 'C:\\x' }));
assert(p.remotePaths.length === 0, 'unparseable JSON yields no remote paths');
assert(p.localPaths.join() === 'C:\\x', 'a broken half does not take the other half down with it');
p = readDragPayload(reader({ 'text/selected-paths': '{"not":"an array"}' }));
assert(p.remotePaths.length === 0, 'valid JSON that is not an array yields no paths');
p = readDragPayload(reader({ 'text/selected-paths': '["/a",7,null]' }));
assert(p.remotePaths.join() === '/a', 'non-string entries are dropped rather than dragged as undefined');

// --- isMultiDrag: only a selected row carries the selection ---
const selection = new Set(['/a', '/b']);
assert(isMultiDrag(selection, '/a'), 'dragging a selected row of a multi-selection carries all of it');
assert(!isMultiDrag(selection, '/c'), 'dragging an unselected row moves that row alone');
assert(!isMultiDrag(new Set(['/a']), '/a'), 'a single-row selection is not a multi drag');
assert(!isMultiDrag(new Set(), '/a'), 'no selection is not a multi drag');

console.log('OK fileTree/dragPayload');
