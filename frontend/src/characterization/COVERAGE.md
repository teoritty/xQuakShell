# `stores/api.ts` characterization coverage

Enumerates every export of `frontend/src/stores/api.ts` (106 exports, confirmed
against the current source) and marks each **characterized** (has at least one
assertion in one of the five `*.char.test.ts` files) or **trivial-atomic** (a
one-line RPC wrapper — "call method X with args, return result/fallback" — with
no orchestration; these get direct coverage from their Phase 2 per-file test
instead of a golden-master assertion here).

`*` = orchestration function per the plan's inventory. All `*` functions, and
all plugin/github/plugin-runtime functions regardless of `*`, are required to
be characterized; three such gaps were found and closed in this task (see
bottom).

## vault (2)

| export | status | test file |
|---|---|---|
| `unlockVault` * | characterized | library.char.test.ts |
| `lockVault` | characterized | library.char.test.ts |

## folders (6)

| export | status | test file |
|---|---|---|
| `refreshFolders` | characterized | seam.char.test.ts, library.char.test.ts |
| `saveFolder` | characterized | library.char.test.ts |
| `deleteFolder` | characterized | library.char.test.ts |
| `createNewFolderInFolder` * | characterized | library.char.test.ts (added) |
| `moveFolder` | trivial-atomic | — |
| `moveFolders` | characterized | library.char.test.ts |
| `reorderFolders` | trivial-atomic | — |

## connections (6)

| export | status | test file |
|---|---|---|
| `refreshAllConnections` | characterized | library.char.test.ts |
| `saveConnection` | characterized | library.char.test.ts |
| `createNewConnectionInFolder` * | characterized | library.char.test.ts |
| `deleteConnection` | trivial-atomic | — |
| `moveConnections` | trivial-atomic | — |
| `reorderConnections` | trivial-atomic | — |

## credentials (7)

| export | status | test file |
|---|---|---|
| `refreshIdentities` | trivial-atomic | — |
| `importPassword` | trivial-atomic | — |
| `deletePassword` | trivial-atomic | — |
| `importIdentity` | trivial-atomic | — |
| `importPuTTYPPK` | trivial-atomic | — |
| `importPuTTYRegPreview` | trivial-atomic | — |
| `importPuTTYRegAsConnections` | trivial-atomic | — |

## sessions (10)

| export | status | test file |
|---|---|---|
| `openSession` * | characterized | sessions.char.test.ts |
| `closeSession` * | characterized | sessions.char.test.ts |
| `createSessionFromSelection` * | characterized | sessions.char.test.ts |
| `focusNextSessionTab` * | characterized | sessions.char.test.ts |
| `focusPrevSessionTab` * | characterized | sessions.char.test.ts |
| `closeActiveSession` * | characterized | sessions.char.test.ts |
| `reportEmbedViewport` | trivial-atomic | — |
| `reportEmbedActivity` | trivial-atomic | — |
| `resolveHostKey` * | characterized | sessions.char.test.ts (added) |
| `getPlatform` | trivial-atomic | — |

## terminal (5)

| export | status | test file |
|---|---|---|
| `sendTerminalInput` | trivial-atomic | — |
| `terminalResize` | trivial-atomic | — |
| `takePendingTerminalOutput` | characterized | runtime.char.test.ts |
| `clearPendingTerminalOutput` | trivial-atomic | — (re-exported buffer helper, exercised transitively via `subscribeToEvents`'s `SessionStateChanged` handler in runtime.char.test.ts, but has no direct assertion) |
| `registerTerminalOutputConsumer` | characterized | runtime.char.test.ts |

## sftp / remote fs (13)

| export | status | test file |
|---|---|---|
| `listPath` | trivial-atomic | — |
| `uploadFile` | characterized | runtime.char.test.ts |
| `downloadFile` | trivial-atomic | — |
| `cancelTransfer` | trivial-atomic | — |
| `removePath` | trivial-atomic | — |
| `mkdirPath` | trivial-atomic | — |
| `createFilePath` | trivial-atomic | — |
| `copyLocalPath` | trivial-atomic | — |
| `renamePath` | trivial-atomic | — |
| `chmodPath` | trivial-atomic | — |
| `chownPath` | trivial-atomic | — |
| `chmodPathRecursive` | trivial-atomic | — |
| `chownPathRecursive` | trivial-atomic | — |

## local fs (12)

| export | status | test file |
|---|---|---|
| `removeLocalPath` | trivial-atomic | — |
| `mkdirLocalPath` | trivial-atomic | — |
| `renameLocalPath` | trivial-atomic | — |
| `createLocalFile` | trivial-atomic | — |
| `selectLocalFile` | trivial-atomic | — |
| `selectLocalDirectory` | trivial-atomic | — |
| `listLocalPath` | trivial-atomic | — |
| `getPortableDataRoot` | trivial-atomic | — |
| `getUserHomeDir` | trivial-atomic | — |
| `getTempDir` | trivial-atomic | — |
| `openFileWithSystem` | trivial-atomic | — |
| `startFileWatch` | trivial-atomic | — |

## known-hosts (2)

| export | status | test file |
|---|---|---|
| `addKnownHost` | trivial-atomic | — |
| `removeKnownHost` | trivial-atomic | — |

## settings (3)

| export | status | test file |
|---|---|---|
| `getSettings` * | characterized | runtime.char.test.ts |
| `saveSettings` * | characterized | runtime.char.test.ts (added) |
| `applyAppearanceSettings` * | characterized | runtime.char.test.ts (added) |

## hotkeys — PURE (2)

| export | status | test file |
|---|---|---|
| `normalizeHotkey` (re-export) | trivial-atomic | — (pure util, exercised transitively by `getSettings`/`saveSettings` tests; no direct assertion needed) |
| `parseHotkeyEvent` (re-export) | trivial-atomic | — |

## audit (6)

| export | status | test file |
|---|---|---|
| `searchAuditLog` | trivial-atomic | — |
| `deleteAuditEntry` | trivial-atomic | — |
| `clearAuditLog` | trivial-atomic | — |
| `getAuditSessionState` | trivial-atomic | — |
| `enableAuditSecretLogging` | trivial-atomic | — |
| `disableAuditSecretLogging` | trivial-atomic | — |

## protocols (6, incl. store)

| export | status | test file |
|---|---|---|
| `refreshConnectionProtocols` * | characterized | plugins.char.test.ts |
| `getPluginConnectionProtocols` * | characterized | plugins.char.test.ts |
| `invalidateProtocolsCache` | characterized | plugins.char.test.ts |
| `connectionProtocolCatalogKey` | characterized | runtime.char.test.ts |
| `connectionProtocols` (store) | characterized | plugins.char.test.ts |
| `sftpReadyPaths` (store) | characterized | runtime.char.test.ts |

## plugins (10)

| export | status | test file |
|---|---|---|
| `listPlugins` | characterized | plugins.char.test.ts |
| `pingPlugin` | characterized | plugins.char.test.ts |
| `setPluginEnabled` | characterized | plugins.char.test.ts |
| `selectPluginSourceDir` | characterized | plugins.char.test.ts (added) |
| `selectPluginBundleFile` | characterized | plugins.char.test.ts (added) |
| `getPluginSettings` | characterized | plugins.char.test.ts |
| `savePluginSettings` | characterized | plugins.char.test.ts |
| `generatePluginPublisherKeyPair` | characterized | plugins.char.test.ts (added) |
| `previewPluginInstall` | characterized | plugins.char.test.ts |
| `installPlugin` * | characterized | plugins.char.test.ts |

## github plugins (8)

| export | status | test file |
|---|---|---|
| `listGitHubRepositories` | characterized | plugins.char.test.ts |
| `addGitHubRepository` | characterized | plugins.char.test.ts |
| `removeGitHubRepository` | characterized | plugins.char.test.ts |
| `setGitHubRepositoryTrust` | characterized | plugins.char.test.ts |
| `fetchGitHubPlugins` | characterized | plugins.char.test.ts |
| `previewGitHubPluginInstall` | characterized | plugins.char.test.ts |
| `installGitHubPlugin` | characterized | plugins.char.test.ts |
| `uninstallGitHubPlugin` * | characterized | plugins.char.test.ts |

## plugin runtime (5)

| export | status | test file |
|---|---|---|
| `getPluginContributions` | characterized | plugins.char.test.ts |
| `executePluginCommand` | characterized | plugins.char.test.ts |
| `preparePluginViewPanel` | characterized | plugins.char.test.ts |
| `relayPluginViewMessage` | characterized | plugins.char.test.ts |
| `releasePluginViewPanel` | characterized | plugins.char.test.ts |

## events (1)

| export | status | test file |
|---|---|---|
| `subscribeToEvents` * | characterized | runtime.char.test.ts |

## misc top-level exports (1)

Not part of the plan's domain buckets but present in the module's export
surface; both are pure data/config, not orchestration.

| export | status | test file |
|---|---|---|
| `DEFAULT_SESSION_HOTKEYS` (const) | trivial-atomic | — (data literal, exercised transitively via `getSettings`/`saveSettings`) |

---

## Totals

- **106 exports** enumerated (matches the plan's inventory count exactly:
  101 direct top-level `export function/const` declarations in `api.ts`,
  plus 3 re-exported terminal-output-buffer helpers
  (`takePendingTerminalOutput`, `clearPendingTerminalOutput`,
  `registerTerminalOutputConsumer`) and 2 re-exported hotkey pure utils
  (`normalizeHotkey`, `parseHotkeyEvent`)).
- **characterized:** 53
- **trivial-atomic:** 53

## Gaps found and closed

All `*`-marked orchestration functions and all plugin/github/plugin-runtime
functions were required to be characterized. Four had no assertion prior to
this audit; each was given a test in the domain-appropriate existing file
(no new characterization file was created):

- `createNewFolderInFolder` (`api.ts:230`) — added to
  `frontend/src/characterization/library.char.test.ts`: asserts the fixed
  `'New folder'` name, correct `parentId`, and that `selectedFolderId` is set
  to the id `SaveFolder` returns (and left untouched when `saveFolder`
  resolves to `null`).
- `resolveHostKey` (`api.ts:446`) — added to
  `frontend/src/characterization/sessions.char.test.ts`: asserts positional
  RPC args, that `pendingHostKey` is cleared on success, and left untouched
  on RPC failure (with `lastError` set instead).
- `saveSettings` (`api.ts:847`) — added to
  `frontend/src/characterization/runtime.char.test.ts`: asserts hotkey
  normalization/defaulting in the outgoing payload and that RPC failure is
  swallowed via `handleError` (no rethrow).
- `applyAppearanceSettings` (`api.ts:864`) — added to
  `frontend/src/characterization/runtime.char.test.ts`: asserts the
  `if (!s) return` short-circuit on a null `getSettings()` result, and that a
  non-null result proceeds to `applyUiScalePercent` (which throws in this
  DOM-less Node test environment, itself part of the pinned behavior).

Additionally, three plugin-domain functions not marked `*` were still
required (per the plugin/github/plugin-runtime blanket rule) and had no
assertion:

- `selectPluginSourceDir` (`api.ts:1237`), `selectPluginBundleFile`
  (`api.ts:1248`), `generatePluginPublisherKeyPair` (`api.ts:1283`) — all
  three added to `frontend/src/characterization/plugins.char.test.ts`:
  absent-method fallback, success pass-through, and (for the first and
  third) RPC-failure fallback via `handleError`.

No gaps remain: every orchestration (`*`) function and every
plugin/github/plugin-runtime function is characterized.
