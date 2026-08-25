// Opening a local shell: the RPC, the store, and the focus that follows.
//
// Orchestration lives here rather than in the toolbar button so the button, the hotkey and
// anything added later all take the same path - including the focus step, which is easy to forget
// at a second call site and produces a tab that opens behind whatever the user was looking at.
import { get } from 'svelte/store';
import { activeTabId, showError } from '../stores/appState';
import { openLocalTerminal as openLocalTerminalRpc } from '../api/localTerminal';
import { upsertLocalTerminal } from '../stores/localTerminalState';
import { t } from '../i18n/messages';

/**
 * Opens a shell and focuses its tab.
 *
 * The tab is added from the RPC's own answer rather than from an event, because the id has to be
 * known here to focus it. A LocalTerminalOutput arriving before that would be buffered by the
 * event funnel and replayed when the renderer mounts, so nothing is lost by not racing.
 */
export async function openLocalTerminal(): Promise<void> {
  try {
    const tab = await openLocalTerminalRpc();
    if (!tab) return;
    upsertLocalTerminal({ id: tab.id, title: tab.title });
    activeTabId.set(tab.id);
  } catch (e) {
    // Worth telling the user about, unlike a failed resize: the most likely cause is a system
    // with no pseudo-console at all, where nothing they do will make the button work.
    showError(get(t)('localTerminal.openFailed'), String(e));
  }
}
