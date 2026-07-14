// frontend/src/lib/tiles/geometry.ts
// Turns (tile count, orientation, dividers) into a concrete CSS-grid spec.
// Slot index -> fixed grid-area, per the canonical layouts. Pure & positional:
// operations are responsible for placing tiles into the right slot order.

import type { Orientation, Dividers } from './types';

export interface GridSpec {
  columns: string;
  rows: string;
  areas: string[]; // "rowStart / colStart / rowEnd / colEnd" per slot
}

function fr(a: number, b: number): string {
  return `${Math.round(a * 100)}fr ${Math.round(b * 100)}fr`;
}

export function computeGrid(n: number, orientation: Orientation, d: Dividers): GridSpec {
  const cols2 = fr(d.main, 1 - d.main);
  const rows2 = fr(d.main, 1 - d.main);
  const cross2 = fr(d.cross, 1 - d.cross);

  if (n <= 1) {
    return { columns: '1fr', rows: '1fr', areas: ['1 / 1 / 2 / 2'] };
  }
  if (n === 2) {
    return orientation === 'h'
      ? { columns: cols2, rows: '1fr', areas: ['1 / 1 / 2 / 2', '1 / 2 / 2 / 3'] }
      : { columns: '1fr', rows: rows2, areas: ['1 / 1 / 2 / 2', '2 / 1 / 3 / 2'] };
  }
  if (n === 3) {
    return orientation === 'h'
      ? {
          columns: cols2,
          rows: cross2,
          areas: ['1 / 1 / 3 / 2', '1 / 2 / 2 / 3', '2 / 2 / 3 / 3'],
        }
      : {
          columns: cross2,
          rows: rows2,
          areas: ['1 / 1 / 2 / 3', '2 / 1 / 3 / 2', '2 / 2 / 3 / 3'],
        };
  }
  // n === 4 : 2x2, main = column split, cross = row split
  return {
    columns: fr(d.main, 1 - d.main),
    rows: fr(d.cross, 1 - d.cross),
    areas: ['1 / 1 / 2 / 2', '1 / 2 / 2 / 3', '2 / 1 / 3 / 2', '2 / 2 / 3 / 3'],
  };
}
