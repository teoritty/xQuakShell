// frontend/src/lib/filePanelCapability.ts
// Single source of truth for "does this session show a remote file browser?".
// Shared by SessionView (which renders the panel) and TileGroup (which decides
// whether to show the per-tile collapse button), so the rule lives in one place.

import type { Session } from '../stores/appState';
import type { ConnectionProtocol } from '../actions/protocolActions';

export function hasFilePanel(session: Session, protocols: ConnectionProtocol[]): boolean {
  if (session.state !== 'ready') return false;
  const proto = protocols.find((p) => p.id === (session.protocol || 'ssh'));
  return session.protocol === 'ssh' || proto?.remoteFs === true;
}
