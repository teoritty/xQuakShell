// The monospace families offered for the terminal, in the order the picker shows them.
//
// A list of names is data, not dialog logic, and it lived in SettingsDialog.svelte only because
// that is where the picker is rendered. Adding a family should not mean editing the screen.
//
// Every entry is a CSS font stack ending in a generic family, so a machine missing the first name
// still gets a monospace face rather than the browser's proportional default.
export const TERMINAL_FONT_STACKS: string[] = [
  'Cascadia Code, Consolas, Courier New, monospace',
  'Consolas, Courier New, monospace',
  'Courier New, monospace',
  'Fira Code, monospace',
  'JetBrains Mono, monospace',
  'Source Code Pro, monospace',
  'Ubuntu Mono, monospace',
  'Hack, monospace',
  'Inconsolata, monospace',
  'Menlo, Monaco, monospace',
  'SF Mono, monospace',
  'IBM Plex Mono, monospace',
  'Roboto Mono, monospace',
];
