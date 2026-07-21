// Bridge between the (pure) conflict resolver and the ConflictDialog component.
// The resolver awaits promptConflict for each conflict; the dialog renders the
// active request and calls respondConflict with the user's decision. Only one
// prompt is ever active at a time because the resolver awaits each in turn.
import { writable } from 'svelte/store';
import type { PlannedFileDTO } from '../backend/gateway';
import type { ConflictDecision } from '../lib/transfer/conflictResolver';

export interface ConflictRequest {
  file: PlannedFileDTO;
  index: number; // 0-based position in the batch
  total: number;
  kind: string; // 'upload' | 'download' | 'localcopy' — drives dialog wording
}

export const conflictRequest = writable<ConflictRequest | null>(null);

let pending: ((decision: ConflictDecision | null) => void) | null = null;

export function promptConflict(req: ConflictRequest): Promise<ConflictDecision | null> {
  return new Promise((resolve) => {
    pending = resolve;
    conflictRequest.set(req);
  });
}

export function respondConflict(decision: ConflictDecision | null): void {
  const resolve = pending;
  pending = null;
  conflictRequest.set(null);
  if (resolve) resolve(decision);
}
