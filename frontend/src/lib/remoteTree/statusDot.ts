// The single implementation of the tree's little status light.
//
// Two callers share it: the connection ping (pre-existing behaviour, pinned by
// pingCharacterization.test.ts) and discovery node status (ADR-014). Keeping
// one mapping is the point — a second "dot" implementation is how the two drift
// into two visual languages for the same idea.
import type { DiscoveryTone } from '../../api/discovery';

export type StatusTone = DiscoveryTone;

export interface StatusDot {
  tone: StatusTone;
  /**
   * Optional plugin-supplied override. ADR-014 constrains it to a strict
   * six-digit hex; anything else is IGNORED (not sanitized, not rejected) and
   * the tone's theme colour is used instead. Silently falling back keeps a
   * sloppy plugin from painting an unreadable row while still refusing to let
   * arbitrary text reach a CSS declaration.
   */
  color?: string;
  tooltip?: string;
}

export const STATUS_COLOR_RE = /^#[0-9a-fA-F]{6}$/;

// Tone -> theme colour. The `ok`/`warn` values are the literals the connection
// ping has always rendered; changing them changes the ping too, which is why
// pingCharacterization.test.ts pins them.
const TONE_COLORS: Record<StatusTone, string> = {
  ok: '#4caf50',
  warn: '#ffb300',
  error: 'var(--danger, #f44747)',
  busy: 'var(--accent, #4b8bbf)',
  neutral: 'var(--text-secondary, #9e9e9e)',
  unknown: 'var(--text-disabled, #6e6e6e)',
};

export function isValidStatusColor(color: string | undefined): boolean {
  return typeof color === 'string' && STATUS_COLOR_RE.test(color);
}

export function statusDotColor(status: StatusDot | null | undefined): string {
  if (!status) return 'transparent';
  if (isValidStatusColor(status.color)) return status.color as string;
  return TONE_COLORS[status.tone] ?? TONE_COLORS.unknown;
}

export function statusDotTooltip(status: StatusDot | null | undefined): string {
  return status?.tooltip ?? '';
}
