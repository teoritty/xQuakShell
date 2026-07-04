export function createSingleFlightRunner<T>() {
  const inflight = new Map<string, Promise<T>>();

  return async function run(key: string, fn: () => Promise<T>): Promise<T> {
    const existing = inflight.get(key);
    if (existing) {
      return existing;
    }

    const promise = fn().finally(() => {
      inflight.delete(key);
    });
    inflight.set(key, promise);
    return promise;
  };
}
