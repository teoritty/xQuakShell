import { EditorView } from '@codemirror/view';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { tags as t } from '@lezer/highlight';

/** Editor chrome + autocomplete tooltip theme aligned with app CSS variables. */
export const appCodeMirrorTheme = EditorView.theme(
  {
    '&': {
      color: 'var(--text-primary)',
      backgroundColor: 'var(--bg-tertiary)',
    },
    '.cm-content': {
      caretColor: 'var(--text-bright)',
    },
    '.cm-cursor, .cm-dropCursor': {
      borderLeftColor: 'var(--text-bright)',
    },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
      backgroundColor: 'var(--accent-muted) !important',
    },
    '.cm-activeLine': {
      backgroundColor: 'var(--bg-hover)',
    },
    '.cm-gutters': {
      backgroundColor: 'var(--bg-secondary)',
      color: 'var(--text-secondary)',
      border: 'none',
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'var(--bg-hover)',
    },
    '.cm-tooltip': {
      backgroundColor: 'var(--bg-secondary)',
      color: 'var(--text-primary)',
      border: '1px solid var(--border-color)',
      borderRadius: '4px',
      boxShadow: '0 4px 12px rgba(0, 0, 0, 0.45)',
    },
    '.cm-tooltip-autocomplete': {
      fontFamily: 'var(--font-mono, monospace)',
    },
    '.cm-tooltip-autocomplete > ul': {
      listStyle: 'none',
      margin: '0',
      padding: '4px 0',
      maxHeight: '220px',
      overflowY: 'auto',
    },
    '.cm-tooltip-autocomplete > ul > li': {
      padding: '3px 10px',
      color: 'var(--text-primary)',
      cursor: 'pointer',
    },
    '.cm-tooltip-autocomplete > ul > li[aria-selected="true"]': {
      backgroundColor: 'var(--bg-active)',
      color: 'var(--text-bright)',
    },
    '.cm-tooltip-autocomplete completion-section': {
      display: 'list-item',
      listStyle: 'none',
      padding: '4px 10px 2px',
      fontSize: '10px',
      fontWeight: '600',
      textTransform: 'uppercase',
      letterSpacing: '0.05em',
      color: 'var(--text-secondary)',
      pointerEvents: 'none',
    },
    '.cm-completionLabel': {
      color: 'inherit',
    },
    '.cm-completionDetail': {
      color: 'var(--text-secondary)',
      marginLeft: '8px',
      fontStyle: 'italic',
    },
    '.cm-completionMatchedText': {
      color: 'var(--accent)',
      textDecoration: 'underline',
    },
    '.cm-tooltip.cm-completionInfo': {
      backgroundColor: 'var(--bg-secondary)',
      color: 'var(--text-primary)',
      padding: '8px 12px',
      maxWidth: 'min(420px, 90vw)',
    },
  },
  { dark: true },
);

export const appCodeMirrorHighlight = syntaxHighlighting(
  HighlightStyle.define([
    { tag: t.keyword, color: 'var(--accent)' },
    { tag: [t.string, t.special(t.string)], color: '#ce9178' },
    { tag: t.comment, color: 'var(--text-secondary)', fontStyle: 'italic' },
    { tag: t.variableName, color: 'var(--text-primary)' },
    { tag: t.operator, color: '#d4d4d4' },
    { tag: t.number, color: '#b5cea8' },
    { tag: t.definition(t.variableName), color: '#4ec9b0' },
  ]),
);
