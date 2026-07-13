// frontend/src/lib/tiles/dropZones.ts
// Turns a cursor position over a tile into a drop Zone. Edge zones are only
// returned when they're in `allowed` (the caller passes allowedEdges), so a
// drop can never produce a non-canonical layout.

import type { Edge, Zone } from './types';

export interface Rect {
  left: number;
  top: number;
  width: number;
  height: number;
}

const EDGE_BAND = 0.25;

export function zoneAt(rect: Rect, x: number, y: number, allowed: Edge[]): Zone {
  if (rect.width <= 0 || rect.height <= 0) return 'center';
  const px = (x - rect.left) / rect.width;
  const py = (y - rect.top) / rect.height;
  const dist: Record<Edge, number> = { left: px, right: 1 - px, top: py, bottom: 1 - py };

  let best: Edge | null = null;
  let bestDist = EDGE_BAND;
  for (const edge of allowed) {
    if (dist[edge] < bestDist) {
      bestDist = dist[edge];
      best = edge;
    }
  }
  return best ?? 'center';
}
