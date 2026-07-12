// Svelte action that focuses and selects an input as soon as it is mounted in
// the DOM. Used by the file-tree inline rename/create inputs, where relying on a
// reactive block or the HTML `autofocus` attribute is unreliable — the element
// may not exist yet when the effect runs, so keystrokes never reach it.
export function focusSelect(node: HTMLInputElement) {
  // Defer to the next microtask so the element is laid out before focusing;
  // WebView2 occasionally drops a focus() call issued in the same tick as mount.
  queueMicrotask(() => {
    node.focus();
    node.select();
  });
}
