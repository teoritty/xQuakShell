// The SSH session end of a terminal. Behaviour is unchanged from when Terminal.svelte called
// these directly — this file exists so the renderer stops naming one of its two possible
// producers.
import { sendTerminalInput, terminalResize } from '../api/terminal';
import { subscribeById, type TerminalIO } from './terminalIO';

export function sessionTerminalIO(sessionId: string): TerminalIO {
  return {
    id: sessionId,
    subscribe(onData) {
      return subscribeById('TerminalOutput', 'sessionId', sessionId, 'output', onData);
    },
    sendInput(data, commandLine) {
      // commandLine is the SSH session's command-line capture for the audit trail. It has no
      // counterpart on a surface, which is why the interface carries it and one implementation
      // ignores it rather than the renderer branching on which producer it is talking to.
      sendTerminalInput(sessionId, data, commandLine);
    },
    resize(cols, rows) {
      terminalResize(sessionId, cols, rows);
    },
  };
}
