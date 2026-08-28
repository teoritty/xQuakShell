import assert from 'node:assert/strict';
import { isInvalidMove } from '../pathMove';

// moveLocalPaths itself reaches the backend, so what is worth pinning here is the rule it applies
// before every rename: the skip that stops a directory being moved inside itself. Getting this
// wrong does not fail loudly — it detaches a subtree.
{
  assert.equal(isInvalidMove('/home/u/src', '/home/u/src'), true, 'a move onto itself is refused');
  assert.equal(
    isInvalidMove('/home/u/src', '/home/u/src/nested'),
    true,
    'a move into its own subtree is refused',
  );
  assert.equal(
    isInvalidMove('/home/u/src', '/home/u/other'),
    false,
    'a move to a sibling directory is allowed',
  );
}

console.log('OK fileTree/move');
