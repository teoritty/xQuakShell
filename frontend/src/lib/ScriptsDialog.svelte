<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from './Modal.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import CodeMirrorEditor from './CodeMirrorEditor.svelte';
  import { sendTerminalInput } from '../stores/api';
  import { activeSessionId, sessions } from '../stores/appState';
  import { Terminal, Play, Plus, BookOpen, Pencil, Trash2 } from 'lucide-svelte';

  export let show = false;

  const DEFAULT_PRESETS = [
    { id: 'p1', name: 'grep', content: 'grep -r "pattern" .', hint: 'Search for pattern in files', hotkey: '' },
    { id: 'p2', name: 'find', content: 'find . -name "*.log" -mtime -1', hint: 'Find recent log files', hotkey: '' },
    { id: 'p3', name: 'tail -f', content: 'tail -f /var/log/syslog', hint: 'Follow log file', hotkey: '' },
    { id: 'p4', name: 'systemctl', content: 'systemctl status', hint: 'Service status', hotkey: '' },
    { id: 'p5', name: 'docker ps', content: 'docker ps -a', hint: 'List containers', hotkey: '' },
    { id: 'p6', name: 'disk usage', content: 'df -h', hint: 'Disk space', hotkey: '' },
    { id: 'p7', name: 'top processes', content: 'ps aux --sort=-%mem | head -20', hint: 'Memory usage', hotkey: '' },
    { id: 'p8', name: 'network', content: 'ss -tulpn', hint: 'Listening ports', hotkey: '' },
  ];

  const STORAGE_KEY = 'scripts-presets';

  function loadPresets(): typeof DEFAULT_PRESETS {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw) {
        const arr = JSON.parse(raw);
        if (Array.isArray(arr) && arr.length > 0) return arr;
      }
    } catch {}
    const initial = DEFAULT_PRESETS.map((p) => ({ ...p, id: 'p' + Date.now() + '-' + Math.random().toString(36).slice(2) }));
    savePresets(initial);
    return initial;
  }

  function savePresets(p: typeof presets) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(p));
    } catch {}
  }

  let presets = loadPresets();
  let selectedPreset: string | null = null;
  let customContent = '';
  let tooltipPopup = { show: false, text: '', x: 0, y: 0 };
  let hoveredHint = '';
  let editPresetId: string | null = null;
  let deleteConfirm = { show: false, presetId: '', presetName: '', fromEdit: false };
  let editPresetName = '';
  let editPresetContent = '';
  let editPresetHint = '';
  let editPresetHotkey = '';
  let hotkeyConflict = '';
  let showEditModal = false;
  let lintErrors: string[] = [];
  let cmEditor: CodeMirrorEditor | null = null;

  $: activeSession = $sessions.find((s) => s.sessionId === $activeSessionId);
  $: hasActiveTerminal = activeSession?.state === 'ready';

  function runScript(content: string) {
    if (!content.trim()) return;
    const sid = $activeSessionId;
    if (!sid) return;
    sendTerminalInput(sid, content.trim() + '\n');
    show = false;
  }

  function runPreset(p: (typeof presets)[0]) {
    runScript(p.content);
  }

  function runCustom() {
    const content = cmEditor?.getValue?.() ?? customContent;
    runScript(content);
  }

  function formatHotkey(h: string): string {
    if (!h) return '';
    return h
      .replace(/Control/g, 'Ctrl')
      .replace(/Meta/g, 'Win')
      .replace(/Alt/g, 'Alt')
      .replace(/Shift/g, 'Shift');
  }

  function parseHotkey(e: KeyboardEvent): string {
    const parts: string[] = [];
    if (e.ctrlKey) parts.push('Ctrl');
    if (e.metaKey) parts.push('Meta');
    if (e.altKey) parts.push('Alt');
    if (e.shiftKey) parts.push('Shift');
    if (e.key && e.key.length === 1) parts.push(e.key.toUpperCase());
    else if (e.key && !['Control', 'Meta', 'Alt', 'Shift'].includes(e.key)) parts.push(e.key);
    return parts.join('+');
  }

  function checkHotkeyConflict(hotkey: string, excludeId?: string): string {
    if (!hotkey) return '';
    const conflict = presets.find((p) => p.id !== excludeId && p.hotkey && p.hotkey === hotkey);
    return conflict ? conflict.name : '';
  }

  function addPreset() {
    const content = cmEditor?.getValue?.() ?? customContent;
    if (!content.trim()) return;
    const id = 'p' + Date.now() + '-' + Math.random().toString(36).slice(2);
    presets = [...presets, { id, name: 'New preset', content: content.trim(), hint: '', hotkey: '' }];
    savePresets(presets);
    editPresetId = id;
    editPresetName = 'New preset';
    editPresetContent = content.trim();
    editPresetHint = '';
    editPresetHotkey = '';
    hotkeyConflict = '';
    showEditModal = true;
  }

  function startEditPreset(p: (typeof presets)[0]) {
    editPresetId = p.id;
    editPresetName = p.name;
    editPresetContent = p.content;
    editPresetHint = p.hint || '';
    editPresetHotkey = p.hotkey || '';
    hotkeyConflict = checkHotkeyConflict(editPresetHotkey, p.id);
    showEditModal = true;
  }

  function saveEditPreset() {
    if (!editPresetId) return;
    const conflict = checkHotkeyConflict(editPresetHotkey, editPresetId);
    if (conflict) {
      hotkeyConflict = conflict;
      return;
    }
    presets = presets.map((p) =>
      p.id === editPresetId
        ? {
            ...p,
            name: editPresetName.trim() || p.name,
            content: editPresetContent,
            hint: editPresetHint.trim() || undefined,
            hotkey: editPresetHotkey || undefined,
          }
        : p
    );
    savePresets(presets);
    showEditModal = false;
  }

  function deletePreset(id: string) {
    presets = presets.filter((p) => p.id !== id);
    savePresets(presets);
    if (selectedPreset === id) selectedPreset = null;
    showEditModal = false;
  }

  function requestDeletePreset(p: (typeof presets)[0]) {
    deleteConfirm = { show: true, presetId: p.id, presetName: p.name };
  }

  function confirmDeletePreset() {
    if (deleteConfirm.presetId) {
      deletePreset(deleteConfirm.presetId);
      if (deleteConfirm.fromEdit) showEditModal = false;
      deleteConfirm = { show: false, presetId: '', presetName: '', fromEdit: false };
    }
  }

  function hideTooltipPopup() {
    tooltipPopup = { show: false, text: '', x: 0, y: 0 };
    hoveredHint = '';
  }

  function basicBashLint(content: string): string[] {
    const errs: string[] = [];
    const lines = content.split('\n');
    let inSingle = false;
    let inDouble = false;
    let inHeredoc = false;
    let heredocDelim = '';
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      for (let j = 0; j < line.length; j++) {
        const c = line[j];
        if (c === '\\' && j < line.length - 1) {
          j++;
          continue;
        }
        if (!inSingle && !inDouble && c === "'") inSingle = true;
        else if (inSingle && c === "'") inSingle = false;
        else if (!inSingle && !inDouble && c === '"') inDouble = true;
        else if (inDouble && c === '"') inDouble = false;
      }
      if (line.trim().match(/^<<-?\s*['"]?(\w+)['"]?/)) {
        const m = line.match(/^<<-?\s*['"]?(\w+)['"]?/);
        if (m) {
          inHeredoc = true;
          heredocDelim = m[1];
        }
      }
      if (inHeredoc && line.trim() === heredocDelim) {
        inHeredoc = false;
      }
    }
    if (inSingle) errs.push('Unclosed single quote');
    if (inDouble) errs.push('Unclosed double quote');
    if (inHeredoc) errs.push('Unclosed heredoc');
    return errs;
  }

  $: currentContent = cmEditor?.getValue?.() ?? customContent;
  $: lintErrors = basicBashLint(currentContent);

  function handleHotkeyCapture(e: KeyboardEvent) {
    e.preventDefault();
    e.stopPropagation();
    editPresetHotkey = parseHotkey(e);
    hotkeyConflict = checkHotkeyConflict(editPresetHotkey, editPresetId || undefined);
  }

  onMount(() => {
    const handler = (e: KeyboardEvent) => {
      if (!$activeSessionId) return;
      const hotkey = parseHotkey(e);
      const stored = (() => {
        try {
          const raw = localStorage.getItem(STORAGE_KEY);
          if (raw) return JSON.parse(raw);
        } catch {}
        return [];
      })();
      const preset = stored.find((p: { hotkey?: string }) => p.hotkey && p.hotkey === hotkey);
      if (preset) {
        e.preventDefault();
        e.stopPropagation();
        runScript(preset.content);
      }
    };
    window.addEventListener('keydown', handler, true);
    return () => window.removeEventListener('keydown', handler, true);
  });
</script>

{#if show}
  <Modal title="Scripts & Commands" show={show} contentClass="scripts-modal" on:close={() => (show = false)}>
    <div class="scripts-body">
      <p class="scripts-desc">Run preset or custom commands in the active terminal. Press hotkey to run.</p>
      {#if !hasActiveTerminal}
        <p class="scripts-warn">No active terminal. Open a connection first.</p>
      {/if}

      <div class="scripts-layout">
        <div class="editor-section">
          <h4>Custom script</h4>
          <div class="editor-wrap">
            <CodeMirrorEditor bind:this={cmEditor} bind:value={customContent} minHeight="280px" />
          </div>
          {#if lintErrors.length > 0}
            <div class="lint-errors">
              {#each lintErrors as err}
                <span class="lint-err">{err}</span>
              {/each}
            </div>
          {/if}
          <div class="editor-actions">
            <button class="secondary" on:click={addPreset} disabled={!customContent.trim()}>
              <Plus size={12} /> Save as preset
            </button>
            <button class="primary" on:click={runCustom} disabled={!hasActiveTerminal || !customContent.trim()}>
              <Play size={12} /> Run
            </button>
          </div>
          <p class="cheat-hint-tip">Hold Ctrl and hover over a preset to see its hint.</p>
        </div>

        <div class="presets-section">
          <h4>Presets & Saved</h4>
          <div class="preset-list">
            {#each presets as p}
              <div
                class="preset-item"
                class:selected={selectedPreset === p.id}
                role="button"
                tabindex="0"
                on:click={() => (selectedPreset = selectedPreset === p.id ? null : p.id)}
                on:keydown={(e) => e.key === 'Enter' && (selectedPreset = selectedPreset === p.id ? null : p.id)}
                on:mouseenter={() => { hoveredHint = p.hint || ''; }}
                on:mousemove={(e) => {
                  if (e.ctrlKey && hoveredHint) {
                    if (!tooltipPopup.show) tooltipPopup = { show: true, text: hoveredHint, x: e.clientX, y: e.clientY };
                    else tooltipPopup = { ...tooltipPopup, x: e.clientX, y: e.clientY };
                  } else {
                    hideTooltipPopup();
                  }
                }}
                on:mouseleave={() => hideTooltipPopup()}
              >
                <span class="preset-name">{p.name}</span>
                <div class="preset-actions">
                  {#if p.hotkey}
                    <span class="hotkey-badge" title="Hotkey">{formatHotkey(p.hotkey)}</span>
                  {/if}
                  <button class="icon-btn" title="Edit" on:click|stopPropagation={() => startEditPreset(p)}>
                    <Pencil size={11} />
                  </button>
                  <button class="icon-btn" title="Run" disabled={!hasActiveTerminal} on:click|stopPropagation={() => runPreset(p)}>
                    <Play size={12} />
                  </button>
                  <button class="icon-btn danger" title="Delete" on:click|stopPropagation={() => requestDeletePreset(p)}>
                    <Trash2 size={11} />
                  </button>
                </div>
              </div>
            {/each}
          </div>
          <button class="secondary small-btn" on:click={addPreset}>
            <Plus size={12} /> Add preset
          </button>
        </div>
      </div>
    </div>
  </Modal>

  {#if tooltipPopup.show}
    <div
      class="tooltip-popup"
      style="left: {tooltipPopup.x + 12}px; top: {tooltipPopup.y + 12}px"
      role="tooltip"
    >
      <BookOpen size={12} />
      {tooltipPopup.text}
    </div>
  {/if}

  <ConfirmDialog
    show={deleteConfirm.show}
    title="Delete script?"
    message="Delete preset &quot;{deleteConfirm.presetName}&quot;?"
    confirmLabel="Delete"
    on:confirm={confirmDeletePreset}
    on:cancel={() => (deleteConfirm = { show: false, presetId: '', presetName: '', fromEdit: false })}
  />
{/if}

{#if showEditModal && editPresetId}
  <Modal title="Edit preset" show={showEditModal} on:close={() => (showEditModal = false)}>
    <div class="edit-preset-form">
      <label class="field">
        <span class="field-label">Name</span>
        <input type="text" bind:value={editPresetName} placeholder="Preset name" />
      </label>
      <label class="field">
        <span class="field-label">Command</span>
        <textarea bind:value={editPresetContent} rows="5" placeholder="Bash command..." class="edit-textarea"></textarea>
      </label>
      <label class="field">
        <span class="field-label">Hint (optional)</span>
        <input type="text" bind:value={editPresetHint} placeholder="Description" />
      </label>
      <label class="field">
        <span class="field-label">Hotkey (click field then press key)</span>
        <div class="hotkey-row">
          <input
            type="text"
            readonly
            value={formatHotkey(editPresetHotkey) || 'Click here, then press key...'}
            class="hotkey-input"
            on:keydown={handleHotkeyCapture}
          />
          <button type="button" class="ghost small-btn" on:click={() => { editPresetHotkey = ''; hotkeyConflict = ''; }}>Clear</button>
        </div>
        {#if hotkeyConflict}
          <span class="hotkey-conflict">Conflict with: {hotkeyConflict}</span>
        {/if}
      </label>
      <div class="edit-actions">
        <button class="primary" on:click={saveEditPreset}>Save</button>
        <button class="secondary danger" on:click={() => editPresetId && (deleteConfirm = { show: true, presetId: editPresetId, presetName: editPresetName, fromEdit: true })}>Delete</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .scripts-body {
    padding: 8px 0;
  }
  .scripts-desc {
    font-size: 12px;
    color: var(--text-secondary);
    margin-bottom: 12px;
  }
  .scripts-warn {
    font-size: 11px;
    color: var(--danger);
    margin-bottom: 8px;
  }
  .scripts-layout {
    display: flex;
    gap: 20px;
    min-height: 320px;
  }
  .editor-section {
    flex: 1;
    min-width: 0;
  }
  .presets-section {
    flex: 0 0 220px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .presets-section h4,
  .editor-section h4 {
    font-size: 12px;
    margin-bottom: 8px;
  }
  .preset-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex: 1;
    overflow-y: auto;
  }
  .preset-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 4px;
    padding: 4px 8px;
    border-radius: 4px;
    cursor: pointer;
    font-size: 12px;
  }
  .preset-item:hover {
    background: var(--bg-hover);
  }
  .preset-item.selected {
    background: var(--bg-active);
  }
  .preset-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .preset-actions {
    display: flex;
    align-items: center;
    gap: 2px;
    flex-shrink: 0;
  }
  .hotkey-badge {
    font-size: 9px;
    padding: 0 4px;
    background: var(--bg-tertiary);
    border-radius: 2px;
    color: var(--text-secondary);
  }
  .icon-btn {
    background: transparent;
    border: none;
    color: var(--text-secondary);
    padding: 2px;
    cursor: pointer;
    display: flex;
    opacity: 0.7;
  }
  .icon-btn:hover {
    opacity: 1;
    color: var(--accent);
  }
  .icon-btn.danger:hover {
    color: var(--danger);
  }
  .icon-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }
  .small-btn {
    font-size: 11px;
    padding: 4px 8px;
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .editor-section {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .editor-wrap {
    flex: 1;
    min-height: 280px;
    display: flex;
    flex-direction: column;
  }
  .lint-errors {
    font-size: 11px;
    color: var(--danger);
    margin-top: 4px;
  }
  .lint-err {
    margin-right: 8px;
  }
  .editor-actions {
    display: flex;
    gap: 8px;
    margin-top: 8px;
  }
  .cheat-hint-tip {
    margin-top: 8px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .tooltip-popup {
    position: fixed;
    z-index: 3000;
    padding: 6px 10px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    font-size: 12px;
    color: var(--text-primary);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.6);
    pointer-events: none;
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .edit-preset-form {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 8px 0;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .field-label {
    font-size: 11px;
    color: var(--text-secondary);
  }
  .edit-textarea {
    font-family: var(--font-mono, monospace);
    font-size: 12px;
    padding: 8px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    color: var(--text-primary);
    resize: vertical;
  }
  .hotkey-row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .hotkey-input {
    flex: 1;
    min-width: 0;
  }
  .hotkey-conflict {
    font-size: 12px;
    color: var(--danger);
  }
  .edit-actions {
    display: flex;
    gap: 8px;
    margin-top: 8px;
  }
</style>
