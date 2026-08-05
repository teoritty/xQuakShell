// The plugin-surface end of a terminal (ADR-015).
//
// Same renderer, different producer: bytes arrive on PluginSurfaceOutput instead of
// TerminalOutput, and input and resizes go to the surface handlers instead of the session ones.
import { sendSurfaceInput, resizeSurface } from '../api/surfaces';
import { subscribeById, type TerminalIO } from './terminalIO';

export function surfaceTerminalIO(surfaceId: string): TerminalIO {
  return {
    id: surfaceId,
    subscribe(onData) {
      return subscribeById('PluginSurfaceOutput', 'surfaceId', surfaceId, 'data', onData);
    },
    sendInput(data) {
      // No commandLine: that capture exists for the SSH audit trail, and a surface's far end is a
      // plugin the host cannot interpret. Sending an empty one would put a blank entry in a log
      // that is supposed to say what was run.
      void sendSurfaceInput(surfaceId, data);
    },
    resize(cols, rows) {
      void resizeSurface(surfaceId, cols, rows);
    },
  };
}
