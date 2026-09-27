// The order terminals are numbered in, for the multi-input picker and the search scope.
//
// Reading order: tile by tile in layout order, and within a tile its tabs left to right. The number
// a terminal gets is the number printed on it, so it has to follow what the user sees rather than
// the order the terminals happened to open in.

/** The part of a tile the numbering reads. */
export interface OrderedTile {
  tabs: string[];
  activeTabId: string;
}

export interface NumberedTerminal {
  id: string;
  /** 1-based; the digit that selects it in the picker when it is 9 or less. */
  number: number;
  /** Whether it is its tile's active tab, and therefore on screen. */
  visible: boolean;
}

/** Numbers the live terminals among the tiles' tabs. Tabs that are not terminals are skipped. */
export function numberTerminals(tiles: OrderedTile[], live: ReadonlySet<string>): NumberedTerminal[] {
  const out: NumberedTerminal[] = [];
  for (const tile of tiles) {
    for (const id of tile.tabs) {
      if (!live.has(id)) continue;
      out.push({ id, number: out.length + 1, visible: tile.activeTabId === id });
    }
  }
  return out;
}

/** The terminal a digit key picks: 1-9 are themselves, 0 is the tenth. */
export function terminalForDigit(numbered: NumberedTerminal[], key: string): string | null {
  if (!/^[0-9]$/.test(key)) return null;
  const number = key === '0' ? 10 : Number(key);
  return numbered.find((t) => t.number === number)?.id ?? null;
}
