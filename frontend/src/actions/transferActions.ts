// Orchestrates conflict-aware drag-and-drop transfers: plan → resolve conflicts
// → execute. The three entry points (upload / download / local copy) share one
// runPlan flow; only the plan and execute RPCs differ.
import type { ExecutePlanDTO, TransferPlanDTO } from '../backend/gateway';
import { fetchSettings, putSettings, type AppSettings } from '../api/settings';
import {
  planUpload,
  planDownload,
  planLocalCopy,
  executeUpload,
  executeDownload,
  executeLocalCopy,
} from '../api/transferPlan';
import { resolveConflicts } from '../lib/transfer/conflictResolver';
import { normalizeExistsDefault, type ConflictAction } from '../lib/transfer/conflictActions';
import { promptConflict } from '../stores/conflictPrompt';
import { cancelTransfer } from '../api/remoteFs';

export async function startUploadDrop(sessionId: string, localPaths: string[], remoteDir: string): Promise<void> {
  const plan = await planUpload(sessionId, localPaths, remoteDir);
  await runPlan(plan, 'upload', (req) => executeUpload(sessionId, req));
}

export async function startDownloadDrop(sessionId: string, remotePaths: string[], localDir: string): Promise<void> {
  const plan = await planDownload(sessionId, remotePaths, localDir);
  await runPlan(plan, 'download', (req) => executeDownload(sessionId, req));
}

export async function startLocalCopyDrop(srcPaths: string[], destDir: string): Promise<void> {
  const plan = await planLocalCopy(srcPaths, destDir);
  await runPlan(plan, 'localcopy', (req) => executeLocalCopy(req));
}

async function runPlan(
  plan: TransferPlanDTO,
  kind: 'upload' | 'download' | 'localcopy',
  execute: (req: ExecutePlanDTO) => Promise<void>,
): Promise<void> {
  // Lifecycle invariant: the backend owns the Transfers-panel item. The planner
  // publishes one under plan.opID and emits exactly one terminal event for it on
  // every path; this layer only displays state and never deletes an item.
  //
  // A plan with no files was already closed as completed during planning and
  // carries no op id, so there is nothing left to own here.
  if (!plan.files || plan.files.length === 0) return;

  // claimed means the executor took the item over and will close it. Anything
  // else leaving this function — an abandoned batch or a thrown exception
  // between the phases — must hand the item back to the backend to close, or it
  // spins on "Scanning…" until the app restarts.
  let claimed = false;
  try {
    const conflicts = plan.files.filter((f) => !!f.conflict);

    // Only fetch settings when there is a conflict to resolve (avoids a round trip
    // on the common clean-transfer path). The full object is kept so a "remember
    // as default" write can save it whole — SaveSettings replaces all settings.
    let settings: AppSettings | null = null;
    let settingsDefault = normalizeExistsDefault(undefined);
    if (conflicts.length > 0) {
      settings = await fetchSettings();
      settingsDefault = normalizeExistsDefault(defaultActionFor(kind, settings));
    }

    const result = await resolveConflicts(conflicts, settingsDefault, (file, index, total) =>
      promptConflict({ file, index, total, kind }),
    );
    if (result === null) return; // user cancelled the batch — the finally closes the item

    if (result.persistDefault && settings) {
      // Persisting the "remember my choice" default is a side write. It must
      // never abort a transfer the user already confirmed: log and continue.
      try {
        await putSettings(withDefaultAction(settings, kind, result.persistDefault));
      } catch (e) {
        console.warn('persist default conflict action failed', e);
      }
    }

    try {
      await execute({ plan, resolutions: result.resolutions });
      claimed = true;
    } catch {
      // The banner is already up (callBackend showed it before rethrowing). What
      // matters here is that the executor never took the item over — an RPC that
      // fails before ExecutePlan runs emits no terminal event — so leaving
      // `claimed` false lets the finally hand the item back to be closed.
    }
  } finally {
    if (!claimed && plan.opID) cancelTransfer(plan.opID);
  }
}

// defaultActionFor reads the persisted default for a transfer kind. Local copies
// reuse the upload default (they place files, with no remote side).
function defaultActionFor(kind: string, settings: AppSettings | null): string | undefined {
  if (!settings) return undefined;
  return kind === 'download' ? settings.defaultDownloadExistsAction : settings.defaultUploadExistsAction;
}

function withDefaultAction(settings: AppSettings, kind: string, action: ConflictAction): AppSettings {
  if (kind === 'download') return { ...settings, defaultDownloadExistsAction: action };
  return { ...settings, defaultUploadExistsAction: action };
}
