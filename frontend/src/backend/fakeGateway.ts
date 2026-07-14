// In-memory stand-in for AppGateway, used by orchestration-logic unit tests
// that want to assert on which backend calls were made and with what
// arguments, without touching the real Wails bridge.
import type { AppGateway } from './gateway';

export interface FakeGateway extends AppGateway {
  calls: { method: string; args: unknown[] }[];
  program(method: string, result: unknown | ((...a: unknown[]) => unknown)): void;
}

// Proxy trap intercepts every property access. 'calls' and 'program' are
// handled as special-cased own properties so they resolve to plain values
// (an array, a function) rather than being treated as a recorded backend
// call themselves.
export function createFakeGateway(): FakeGateway {
  const calls: FakeGateway['calls'] = [];
  const programmed = new Map<string, unknown | ((...a: unknown[]) => unknown)>();

  const handler: ProxyHandler<object> = {
    get(_t, prop: string) {
      if (prop === 'calls') return calls;
      if (prop === 'program') {
        return (method: string, result: unknown | ((...a: unknown[]) => unknown)) => {
          programmed.set(method, result);
        };
      }
      return (...args: unknown[]) => {
        calls.push({ method: prop, args });
        const r = programmed.get(prop);
        // A programmed function that throws must surface as a REJECTED promise,
        // so callers see the same shape as a failing async Wails RPC.
        try {
          const val = typeof r === 'function' ? (r as (...a: unknown[]) => unknown)(...args) : r;
          return Promise.resolve(val);
        } catch (e) {
          return Promise.reject(e);
        }
      };
    },
  };

  return new Proxy({}, handler) as FakeGateway;
}
