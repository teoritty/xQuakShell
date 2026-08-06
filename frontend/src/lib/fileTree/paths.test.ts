// frontend/src/lib/fileTree/paths.test.ts
import {
  remoteParent,
  remoteBasename,
  remoteJoin,
  normalizeRemotePathInput,
  localSeparator,
  isAtFilesystemRoot,
  parentDirectory,
  localBasename,
  localJoin,
  normalizeLocalPathInput,
} from './paths';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}

// --- remote: POSIX only ---
assert(remoteParent('/a/b/c') === '/a/b', 'remote parent drops the last segment');
assert(remoteParent('/a') === '/', 'the parent of a top-level entry is the root');
assert(remoteParent('/') === '/', 'the root is its own parent, so goUp cannot escape it');

assert(remoteBasename('/a/b/c.txt') === 'c.txt', 'remote basename is the last segment');
assert(remoteBasename('/a/b/') === 'b', 'a trailing slash does not produce an empty name');
assert(remoteBasename('/') === 'item', 'the root falls back rather than yielding an empty name');
assert(remoteBasename('/', 'fallback') === 'fallback', 'the fallback is caller-chosen');

assert(remoteJoin('/', 'x') === '/x', 'joining at the root does not double the slash');
assert(remoteJoin('/a/b', 'x') === '/a/b/x', 'joining below the root inserts one slash');

assert(normalizeRemotePathInput('a/b') === '/a/b', 'a relative entry is made absolute');
assert(normalizeRemotePathInput('/a//b') === '/a/b', 'repeated slashes collapse');
assert(normalizeRemotePathInput('/a/b/') === '/a/b', 'a trailing slash is dropped');
assert(normalizeRemotePathInput('\\a\\b') === '/a/b', 'backslashes typed into the remote bar become slashes');
assert(normalizeRemotePathInput('/') === '/', 'the root survives normalization');

// --- local: separator decided per path ---
assert(localSeparator('C:\\Users') === '\\', 'a backslash means Windows');
assert(localSeparator('/home/u') === '/', 'no backslash means POSIX');

assert(isAtFilesystemRoot('/'), 'POSIX root');
assert(isAtFilesystemRoot('C:'), 'a bare drive letter is a root');
assert(isAtFilesystemRoot('C:\\'), 'a drive root is a root');
assert(isAtFilesystemRoot('c:\\'), 'the drive letter is case-insensitive');
assert(!isAtFilesystemRoot('C:\\Users'), 'a directory on a drive is not a root');
assert(!isAtFilesystemRoot('/home'), 'a directory under the POSIX root is not a root');
assert(isAtFilesystemRoot(''), 'an unset path counts as a root, so goUp does nothing before mount');

assert(parentDirectory('/home/u/docs') === '/home/u', 'POSIX parent');
assert(parentDirectory('/home') === '/', 'the parent of a top-level directory is the root');
assert(parentDirectory('/') === '/', 'the POSIX root is its own parent');
assert(parentDirectory('C:\\Users\\me') === 'C:\\Users', 'Windows parent');
// A bare "C:" is a per-drive working directory on Windows, not the drive root.
assert(parentDirectory('C:\\Users') === 'C:\\', 'the parent of a top-level Windows directory is the drive root, not "C:"');
assert(parentDirectory('C:\\') === 'C:\\', 'a drive root is its own parent');
assert(parentDirectory('/home/u/') === '/home', 'a trailing separator does not add an empty segment');

assert(localBasename('C:\\a\\b.txt') === 'b.txt', 'Windows basename');
assert(localBasename('/a/b.txt') === 'b.txt', 'POSIX basename');
assert(localBasename('C:\\a\\') === 'a', 'a trailing separator does not produce an empty name');
assert(localBasename('/') === 'item', 'the root falls back');

assert(localJoin('C:\\a', 'b') === 'C:\\a\\b', 'Windows join uses a backslash');
assert(localJoin('C:\\', 'b') === 'C:\\b', 'joining at a drive root does not double the separator');
assert(localJoin('/a', 'b') === '/a/b', 'POSIX join uses a slash');
assert(localJoin('/', 'b') === '/b', 'joining at the POSIX root does not double the slash');

assert(normalizeLocalPathInput('C:/Users/me') === 'C:\\Users\\me', 'slashes typed on Windows become backslashes');
assert(normalizeLocalPathInput('C:') === 'C:\\', 'a bare drive letter becomes the drive root');
assert(normalizeLocalPathInput('C:\\') === 'C:\\', 'a drive root is already normal');
assert(normalizeLocalPathInput('C:\\Users\\') === 'C:\\Users', 'a trailing backslash is dropped');
assert(normalizeLocalPathInput('C:\\\\Users') === 'C:\\Users', 'repeated backslashes collapse');
assert(normalizeLocalPathInput('/home//u/') === '/home/u', 'POSIX input collapses and trims');
assert(normalizeLocalPathInput('/') === '/', 'the POSIX root survives');
assert(normalizeLocalPathInput('\\', 'C:\\home') === 'C:\\home', 'an input that normalizes away falls back to home');
assert(normalizeLocalPathInput('\\') === '', 'with no home there is nothing to fall back to');

console.log('OK fileTree/paths');
