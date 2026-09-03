import { planRename } from './renamePlan';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const items = [
  { id: 'a', name: 'Alpha' },
  { id: 'b', name: 'Beta' },
];

// The happy path, and the only case that may produce a save.
{
  const plan = planRename('b', '  Bravo  ', items);
  assert(plan !== null, 'a changed name must produce a plan');
  assert(plan!.target.id === 'b', `target = ${plan!.target.id}, want b`);
  assert(plan!.name === 'Bravo', `name = ${plan!.name}, want the trimmed name`);
}

// Four ways to have nothing to do. Each returns null for its own reason, and the caller closes the
// editor regardless — that is what stops a row getting stuck in edit mode.
{
  assert(planRename(null, 'Bravo', items) === null, 'no open editor is not a rename');
  assert(planRename('b', '   ', items) === null, 'a name trimmed to nothing keeps the old name');
  assert(planRename('gone', 'Bravo', items) === null, 'a row that vanished is not a rename');
  assert(planRename('b', 'Beta', items) === null, 'an unchanged name must not rewrite the record');
  assert(planRename('b', '  Beta  ', items) === null, 'the comparison is against the trimmed name');
}

// The plan points at the live item, so the caller can spread it and change only the name.
{
  const plan = planRename('a', 'Alpha 2', items);
  assert(plan!.target === items[0], 'target must be the item itself, not a copy');
}

console.log('renamePlan tests passed');
