import { createSingleFlightRunner } from './repoFetchSingleFlight';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function testSingleFlightDedupesParallelCalls() {
  const run = createSingleFlightRunner<void>();
  let calls = 0;

  await Promise.all([
    run('repo-a', async () => {
      calls += 1;
      await delay(20);
    }),
    run('repo-a', async () => {
      calls += 1;
      await delay(20);
    }),
  ]);

  assert(calls === 1, `expected one fetch, got ${calls}`);
}

async function testSingleFlightAllowsDistinctKeys() {
  const run = createSingleFlightRunner<void>();
  let calls = 0;

  await Promise.all([
    run('repo-a', async () => {
      calls += 1;
    }),
    run('repo-b', async () => {
      calls += 1;
    }),
  ]);

  assert(calls === 2, `expected two fetches, got ${calls}`);
}

async function runTests() {
  await testSingleFlightDedupesParallelCalls();
  await testSingleFlightAllowsDistinctKeys();
}

runTests()
  .then(() => {
    console.log('repoFetchSingleFlight tests passed');
  })
  .catch((err) => {
    console.error(err);
    process.exit(1);
  });
