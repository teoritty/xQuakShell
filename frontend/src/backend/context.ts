// Injection seam for the Wails gateways: production code resolves the live
// bridge lazily on first access, while tests can inject a fake via
// setGateway/setRuntime before any code under test reads it.
import type { AppGateway, RuntimeGateway } from './gateway';
import { liveApp, liveRuntime } from './liveGateway';

let gateway: AppGateway | null | undefined; // undefined = not yet resolved
let runtime: RuntimeGateway | null | undefined;

export function getGateway(): AppGateway | null {
  if (gateway === undefined) gateway = liveApp();
  return gateway;
}

export function setGateway(g: AppGateway | null): void {
  gateway = g;
}

export function getRuntime(): RuntimeGateway | null {
  if (runtime === undefined) runtime = liveRuntime();
  return runtime;
}

export function setRuntime(r: RuntimeGateway | null): void {
  runtime = r;
}
