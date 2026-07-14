// frontend/src/lib/tiles/resizeMath.ts
// Pointer delta -> clamped divider ratio. Mirrors the 20%..80% clamp used by
// SessionView.svelte's splits so tiles never collapse to nothing.

export const MIN_RATIO = 0.2;
export const MAX_RATIO = 0.8;

export function nextRatio(start: number, deltaPx: number, sizePx: number): number {
  if (sizePx <= 0) return start;
  const raw = start + deltaPx / sizePx;
  return Math.max(MIN_RATIO, Math.min(MAX_RATIO, raw));
}
