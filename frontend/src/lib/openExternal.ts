import { getRuntime } from '../backend/context';

/**
 * Opens a URL in the user's real browser.
 *
 * `window.open` does not do this from inside a Wails webview, and it fails differently on every
 * platform - which is why issue #41 reads as two unrelated bugs. WebView2 answers it with a second
 * chrome-less in-app window: wrong, but visibly something. WebKitGTK, which is what the Linux
 * builds run on, drops the call silently unless the host handles its `create` signal, so the button
 * does nothing whatsoever and the app looks broken. Wails' BrowserOpenURL hands the URL to the OS
 * and behaves the same everywhere.
 *
 * http and https only. Every caller today passes a hardcoded GitHub URL, so nothing needs the
 * restriction yet - but this is now the single door from the app out to the operating system, and
 * BrowserOpenURL will pass `file://`, `smb://` or any custom scheme straight to the shell handler.
 * A URL reaching here from a plugin README, a discovery node or an error payload is one refactor
 * away, and refusing everything else costs a line.
 *
 * Returns whether the URL was handed over, so a caller can tell "opened" from "there is no runtime
 * here" - which is the case in every unit test, and in the log viewer window.
 */
export function openExternal(url: string): boolean {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return false;
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    return false;
  }
  const runtime = getRuntime();
  if (!runtime) return false;
  runtime.BrowserOpenURL(url);
  return true;
}
