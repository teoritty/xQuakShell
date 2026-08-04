// Ownership rule for a file pane's row selection.
//
// The Local/Remote file panes kept `selectedPaths` as write-only state: a row
// click set it, and only pane-internal flows (delete, navigate) ever cleared it.
// Nothing watched pointer activity, so a highlighted row stayed highlighted
// forever — after clicking empty space in the same pane, the other pane, the
// terminal, or anywhere else in the app.
//
// The missing rule: a pane owns its selection only while the pointer keeps
// landing on one of *its own* rows. This function is that rule. It walks the
// ancestor chain of the event target once, answering both questions the panes
// need — "is this a row?" and "is that row mine?" — so a pane can clear its
// selection on any other pointerdown.
//
// The node shape is structural rather than `Element` so the rule is testable
// without a DOM; real elements satisfy it.

/** Class marking a selectable file row (see FileTreeNode / LocalFileTreeNode). */
export const ROW_CLASS = 'node-row';

export interface SelectionScopeNode {
  classList: { contains(name: string): boolean };
  parentElement: SelectionScopeNode | null;
}

/**
 * True when a pointerdown on `target` must leave `paneRoot`'s selection intact,
 * i.e. it landed on a row inside that pane. Everything else — empty pane space,
 * the pane header, another pane, anywhere else in the window, or a target that
 * is already detached — clears it.
 */
export function keepsPaneSelection(
  target: SelectionScopeNode | null,
  paneRoot: SelectionScopeNode | null,
): boolean {
  if (!target || !paneRoot) return false;
  let insideRow = false;
  for (let node: SelectionScopeNode | null = target; node; node = node.parentElement) {
    if (!insideRow && node.classList.contains(ROW_CLASS)) insideRow = true;
    // Reaching our own pane decides it: a row seen on the way up is ours.
    if (node === paneRoot) return insideRow;
  }
  return false;
}
