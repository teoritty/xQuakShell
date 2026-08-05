// The node details panel's target and its refresh signal (ADR-015 §3).
//
// The regression this file exists for: a plugin pushing a newer panel used to re-set the target
// object. Every derived value stayed equal, so nothing re-ran and the push was silently lost. The
// refresh is a counter for exactly that reason, and a counter is testable without a component.
import { get } from 'svelte/store';
import { detailsConnectionId } from './appState';
import {
  closeNodeDetails,
  isCurrentNode,
  nodeDetailsRevision,
  nodeDetailsTarget,
  nodeDetailsYieldToConnection,
  openNodeDetails,
  requestNodeDetailsReload,
} from './nodeDetailsState';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const target = { connectionId: 'c1', pluginId: 'p1', nodeId: 'n1', label: 'web' };

function reset() {
  nodeDetailsTarget.set(null);
  nodeDetailsRevision.set(0);
  detailsConnectionId.set('');
}

function testPushForTheOpenNodeBumpsTheRevision() {
  reset();
  openNodeDetails(target);

  const before = get(nodeDetailsRevision);
  assert(
    requestNodeDetailsReload('c1', 'p1', 'n1'),
    'a push naming the open node must be accepted'
  );
  assert(
    get(nodeDetailsRevision) === before + 1,
    'the panel reloads off the revision, so a push must move it'
  );
}

function testPushForAnotherNodeIsIgnored() {
  reset();
  openNodeDetails(target);

  for (const [c, p, n] of [
    ['c2', 'p1', 'n1'],
    ['c1', 'p2', 'n1'],
    ['c1', 'p1', 'n2'],
  ]) {
    assert(!requestNodeDetailsReload(c, p, n), `a push for ${c}/${p}/${n} must not be accepted`);
  }
  assert(get(nodeDetailsRevision) === 0, 'a push for another node must not cost a round trip');
}

function testPushWithNoPanelOpenIsIgnored() {
  reset();
  assert(!requestNodeDetailsReload('c1', 'p1', 'n1'), 'nothing is on screen to refresh');
  assert(get(nodeDetailsRevision) === 0, 'the revision must not move with no panel open');
}

function testRepeatedPushesKeepMoving() {
  reset();
  openNodeDetails(target);
  requestNodeDetailsReload('c1', 'p1', 'n1');
  requestNodeDetailsReload('c1', 'p1', 'n1');
  assert(
    get(nodeDetailsRevision) === 2,
    'each push is a separate reason to re-read: a coalesced counter would drop the last snapshot'
  );
}

function testIsCurrentNodeFollowsTheTarget() {
  reset();
  assert(!isCurrentNode('c1', 'p1', 'n1'), 'no target means no current node');
  openNodeDetails(target);
  assert(isCurrentNode('c1', 'p1', 'n1'), 'the open node is the current one');
  closeNodeDetails();
  assert(!isCurrentNode('c1', 'p1', 'n1'), 'a closed panel has no current node');
}

// The sidebar has one details slot: opening a node closes the connection editor and vice versa.
function testTheDetailsSlotIsShared() {
  reset();
  detailsConnectionId.set('c1');
  openNodeDetails(target);
  assert(get(detailsConnectionId) === '', 'opening a node panel closes the connection editor');

  nodeDetailsYieldToConnection();
  assert(get(nodeDetailsTarget) === null, 'selecting a connection closes the node panel');
}

const tests = [
  testPushForTheOpenNodeBumpsTheRevision,
  testPushForAnotherNodeIsIgnored,
  testPushWithNoPanelOpenIsIgnored,
  testRepeatedPushesKeepMoving,
  testIsCurrentNodeFollowsTheTarget,
  testTheDetailsSlotIsShared,
];

for (const test of tests) {
  test();
}
console.log(`nodeDetailsState.test passed (${tests.length} cases)`);
