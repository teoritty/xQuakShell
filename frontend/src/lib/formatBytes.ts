// Pure, human-readable formatting of a byte rate using binary (1024) units.
// Kept dependency-free so it can be unit-tested in isolation.

const UNITS: string[] = ['KiB/s', 'MiB/s', 'GiB/s'];

/**
 * Format a transfer speed (bytes per second) for display.
 *
 * Returns an empty string for non-finite or non-positive input so the caller
 * can simply hide the element. Below 1024 B/s the value is shown as whole
 * bytes; from KiB/s upward it uses one decimal place and never overflows past
 * GiB/s.
 */
export function formatBytesPerSec(bps: number): string {
  if (!Number.isFinite(bps) || bps <= 0) return '';
  if (bps < 1024) return `${Math.round(bps)} B/s`;

  let value = bps / 1024;
  let unit = UNITS[0];
  for (let i = 1; i < UNITS.length && value >= 1024; i++) {
    value /= 1024;
    unit = UNITS[i];
  }
  return `${value.toFixed(1)} ${unit}`;
}
