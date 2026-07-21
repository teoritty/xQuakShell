// Pure orchestration of conflict resolution, independent of any UI. Given a
// plan's conflicts, a persisted default, and a `prompt` function that shows the
// dialog for one conflict, it produces the per-target resolutions to send to the
// backend — or null if the user cancelled the whole batch.
//
// This is the FileZilla decision flow:
//  - a non-'ask' default is applied to every conflict without prompting;
//  - otherwise each conflict is prompted in turn, and "Always use this action"
//    resolves every remaining conflict without further prompts;
//  - "remember as default" surfaces the action to persist to settings;
//  - cancelling any prompt aborts the entire batch.

import type { ExecutePlanDTO, PlannedFileDTO, ResolvedActionDTO } from '../../backend/gateway';
import type { ConflictAction, ExistsDefault } from './conflictActions';

// ConflictDecision is what the dialog returns for a single conflict.
export interface ConflictDecision {
  action: ConflictAction;
  newName?: string;
  // applyToAll: "Always use this action" — reuse for the rest of this batch.
  applyToAll: boolean;
  // rememberDefault: persist this action as the settings default (the dialog's
  // "Apply to current queue only" checkbox, inverted).
  rememberDefault: boolean;
}

// ConflictPrompt shows the dialog for one conflict; resolving to null cancels
// the batch. index/total drive the "Conflict N of M" caption.
export type ConflictPrompt = (
  file: PlannedFileDTO,
  index: number,
  total: number,
) => Promise<ConflictDecision | null>;

export interface ResolveResult {
  resolutions: ResolvedActionDTO[];
  // persistDefault is non-null when the user asked to remember an action.
  persistDefault: ConflictAction | null;
}

// resolveConflicts drives resolution. Returns null iff the batch was cancelled.
export async function resolveConflicts(
  conflicts: PlannedFileDTO[],
  settingsDefault: ExistsDefault,
  prompt: ConflictPrompt,
): Promise<ResolveResult | null> {
  if (conflicts.length === 0) {
    return { resolutions: [], persistDefault: null };
  }

  // A persisted default short-circuits the dialog entirely.
  if (settingsDefault !== 'ask') {
    return {
      resolutions: conflicts.map((f) => ({ target: f.target, action: settingsDefault })),
      persistDefault: null,
    };
  }

  const resolutions: ResolvedActionDTO[] = [];
  let sticky: ConflictAction | null = null;
  let persistDefault: ConflictAction | null = null;

  for (let i = 0; i < conflicts.length; i++) {
    const file = conflicts[i];
    if (sticky !== null) {
      resolutions.push({ target: file.target, action: sticky });
      continue;
    }
    const decision = await prompt(file, i, conflicts.length);
    if (decision === null) return null; // cancelled the whole batch

    const entry: ResolvedActionDTO = { target: file.target, action: decision.action };
    // An explicit rename name only applies to this one file; a sticky rename
    // auto-numbers the rest on the backend.
    if (decision.action === 'rename' && decision.newName) entry.newName = decision.newName;
    resolutions.push(entry);

    if (decision.applyToAll) sticky = decision.action;
    if (decision.rememberDefault) persistDefault = decision.action;
  }

  return { resolutions, persistDefault };
}

// buildExecuteRequest assembles the backend payload from a plan and resolutions.
export function buildExecuteRequest(
  plan: TransferPlanLike,
  resolutions: ResolvedActionDTO[],
): ExecutePlanDTO {
  return { plan, resolutions };
}

// TransferPlanLike is the structural shape buildExecuteRequest needs; using the
// DTO directly would couple this pure module to the generated binding import,
// which is unnecessary.
type TransferPlanLike = ExecutePlanDTO['plan'];
