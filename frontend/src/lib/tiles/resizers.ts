// frontend/src/lib/tiles/resizers.ts
// Rectangles (in % of the grid box) for the draggable divider bars of each
// canonical layout. Rendered absolutely over the grid by TileGrid.

import type { Orientation, Dividers } from './types';

export interface ResizerSpec {
  divider: 'main' | 'cross';
  axis: 'x' | 'y'; // 'x' = vertical bar dragged horizontally
  xPct: number;
  yPct: number;
  wPct: number;
  hPct: number;
}

const BAR = 0; // logical thickness handled in CSS; rect is a line at the ratio

export function computeResizers(n: number, orientation: Orientation, d: Dividers): ResizerSpec[] {
  if (n <= 1) return [];
  const main = d.main * 100;
  const cross = d.cross * 100;

  if (n === 2) {
    return orientation === 'h'
      ? [{ divider: 'main', axis: 'x', xPct: main, yPct: 0, wPct: BAR, hPct: 100 }]
      : [{ divider: 'main', axis: 'y', xPct: 0, yPct: main, wPct: 100, hPct: BAR }];
  }
  if (n === 3) {
    return orientation === 'h'
      ? [
          { divider: 'main', axis: 'x', xPct: main, yPct: 0, wPct: BAR, hPct: 100 },
          // cross bar spans only the right column
          { divider: 'cross', axis: 'y', xPct: main, yPct: cross, wPct: 100 - main, hPct: BAR },
        ]
      : [
          { divider: 'main', axis: 'y', xPct: 0, yPct: main, wPct: 100, hPct: BAR },
          // cross bar spans only the bottom row
          { divider: 'cross', axis: 'x', xPct: cross, yPct: main, wPct: BAR, hPct: 100 - main },
        ];
  }
  // n === 4 : full-length shared dividers
  return [
    { divider: 'main', axis: 'x', xPct: main, yPct: 0, wPct: BAR, hPct: 100 },
    { divider: 'cross', axis: 'y', xPct: 0, yPct: cross, wPct: 100, hPct: BAR },
  ];
}
