// The local shell end of a terminal.
//
// Same renderer, a third producer: bytes arrive on LocalTerminalOutput instead of TerminalOutput
// or PluginSurfaceOutput, and input and resizes go to the local terminal handlers.
import { sendLocalTerminalInput, resizeLocalTerminal } from '../api/localTerminal';
import { subscribeById, type TerminalIO } from './terminalIO';

export function localTerminalIO(id: string): TerminalIO {
  return {
    id,
    subscribe(onData) {
      return subscribeById('LocalTerminalOutput', 'id', id, 'data', onData);
    },
    // commandLine is accepted by the interface and dropped here, deliberately. It exists for the
    // SSH audit trail, which records what was done to somebody else's machine. This is the user's
    // own computer, where they already have a shell history, and capturing their command lines
    // would collect passwords typed into arguments in exchange for telling nobody anything new.
    sendInput(data) {
      void sendLocalTerminalInput(id, data);
    },
    resize(cols, rows) {
      void resizeLocalTerminal(id, cols, rows);
    },
  };
}
