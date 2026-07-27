// Atomic RPC wrappers for conflict-aware transfers. Each is a thin call routed
// through callBackend for uniform error handling; no store access here.
import { callBackend, callBackendVoid } from '../backend/callBackend';
import type { ExecutePlanDTO, TransferPlanDTO } from '../backend/gateway';

const EMPTY_PLAN: TransferPlanDTO = { kind: '', opID: '', dirs: [], files: [] };

export async function planUpload(sessionId: string, localPaths: string[], remoteDir: string): Promise<TransferPlanDTO> {
  return callBackend('Plan upload', EMPTY_PLAN, (app) => app.PlanUpload(sessionId, localPaths, remoteDir));
}

export async function planDownload(sessionId: string, remotePaths: string[], localDir: string): Promise<TransferPlanDTO> {
  return callBackend('Plan download', EMPTY_PLAN, (app) => app.PlanDownload(sessionId, remotePaths, localDir));
}

export async function planLocalCopy(srcPaths: string[], destDir: string): Promise<TransferPlanDTO> {
  return callBackend('Plan copy', EMPTY_PLAN, (app) => app.PlanLocalCopy(srcPaths, destDir));
}

// The three execute calls rethrow after showing the banner. Their caller has to
// know whether the executor actually took the plan over: an RPC that fails
// before ExecutePlan runs (no session context, no backend) emits no terminal
// event, so the caller must hand the panel item back to be closed. Swallowing
// the rejection is what left items stuck on "Scanning…".
export async function executeUpload(sessionId: string, req: ExecutePlanDTO): Promise<void> {
  return callBackendVoid('Upload', (app) => app.ExecuteUpload(sessionId, req), { rethrow: true });
}

export async function executeDownload(sessionId: string, req: ExecutePlanDTO): Promise<void> {
  return callBackendVoid('Download', (app) => app.ExecuteDownload(sessionId, req), { rethrow: true });
}

export async function executeLocalCopy(req: ExecutePlanDTO): Promise<void> {
  return callBackendVoid('Copy', (app) => app.ExecuteLocalCopy(req), { rethrow: true });
}
