import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { refreshFolders } from '../actions/folderActions';
import { folders } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const fake = createFakeGateway();
fake.program('GetFolders', [{ id: 'f1', name: 'F', parentId: '', order: 0 }]);
setGateway(fake);
await refreshFolders();
assert(get(folders).length === 1, 'injected gateway reaches legacy api.ts through the barrel');
console.log('seam.char.test passed');
