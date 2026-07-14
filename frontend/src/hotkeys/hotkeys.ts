export function normalizeHotkey(input: string): string {
  if (!input) return '';
  const rawParts = input.split('+').map((x) => x.trim()).filter(Boolean);
  if (rawParts.length === 0) return '';
  const modifiers = new Set<string>();
  let key = '';
  for (const part of rawParts) {
    const lower = part.toLowerCase();
    if (lower === 'ctrl' || lower === 'control') modifiers.add('Ctrl');
    else if (lower === 'shift') modifiers.add('Shift');
    else if (lower === 'alt' || lower === 'option') modifiers.add('Alt');
    else if (lower === 'meta' || lower === 'cmd' || lower === 'win' || lower === 'super') modifiers.add('Meta');
    else if (lower === ' ') key = 'Space';
    else if (part.length === 1) key = part.toUpperCase();
    else key = part[0].toUpperCase() + part.slice(1);
  }
  const ordered: string[] = [];
  if (modifiers.has('Ctrl')) ordered.push('Ctrl');
  if (modifiers.has('Meta')) ordered.push('Meta');
  if (modifiers.has('Alt')) ordered.push('Alt');
  if (modifiers.has('Shift')) ordered.push('Shift');
  if (key) ordered.push(key);
  return ordered.join('+');
}

export function parseHotkeyEvent(e: KeyboardEvent): string {
  const parts: string[] = [];
  if (e.ctrlKey) parts.push('Ctrl');
  if (e.metaKey) parts.push('Meta');
  if (e.altKey) parts.push('Alt');
  if (e.shiftKey) parts.push('Shift');
  const ignoreKeys = new Set(['Control', 'Meta', 'Alt', 'Shift']);
  if (!ignoreKeys.has(e.key)) {
    if (e.key === ' ') parts.push('Space');
    else if (e.key.length === 1) parts.push(e.key.toUpperCase());
    else parts.push(e.key);
  }
  return normalizeHotkey(parts.join('+'));
}
