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
import { removeTransfer } from '../stores/appState';

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
  // The planner emits a transient "scanning" item under plan.opID during
  // enumeration. If nothing will actually execute below, retire that item so it
  // doesn't linger in the panel forever.
  if (!plan.files || plan.files.length === 0) {
    removeTransfer(plan.opID);
    return;
  }

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
  if (result === null) {
    removeTransfer(plan.opID); // user cancelled the batch — retire the scan item
    return;
  }

  if (result.persistDefault && settings) {
    await putSettings(withDefaultAction(settings, kind, result.persistDefault));
  }

  await execute({ plan, resolutions: result.resolutions });
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
