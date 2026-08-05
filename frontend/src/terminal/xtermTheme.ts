// The terminal's colours.
//
// A table of values with no behaviour, lifted out of the renderer so the file that draws a terminal
// is about drawing one. Settings may override any of these at runtime; this is what a terminal
// looks like before anyone has said otherwise.

export const defaultTerminalTheme = {
  background: '#1e1e1e',
  foreground: '#cccccc',
  cursor: '#ffffff',
  selectionBackground: 'rgba(255, 255, 255, 0.30)',
  selectionForeground: '#000000',
  selectionInactiveBackground: 'rgba(255, 255, 255, 0.15)',
  black: '#1e1e1e',
  red: '#f44747',
  green: '#6a9955',
  yellow: '#d7ba7d',
  blue: '#569cd6',
  magenta: '#c586c0',
  cyan: '#4ec9b0',
  white: '#d4d4d4',
  brightBlack: '#808080',
  brightRed: '#f44747',
  brightGreen: '#6a9955',
  brightYellow: '#d7ba7d',
  brightBlue: '#569cd6',
  brightMagenta: '#c586c0',
  brightCyan: '#4ec9b0',
  brightWhite: '#e0e0e0',
};
