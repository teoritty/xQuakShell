# FileZilla-style transfer conflict dialog

Status: approved (2026-07-19)

## Problem

Drag-and-drop file transfers (local↔remote panes) and OS file drops from Explorer
silently overwrite existing files at the destination. There is no
conflict-resolution UI. FileZilla shows a "target file already exists" dialog with
a choice of actions; we want equivalent behavior.

## Scope

- **Upload** (drag local → remote pane).
- **Download** (drag remote → local pane).
- **Local copy** (OS Explorer drop into the local pane → `copyLocalPath`).
- Actions (FileZilla's set **minus Resume**): Overwrite, Overwrite if source
  newer, Overwrite if different size, Overwrite if different size or source newer,
  Rename, Skip.
- A persisted **default action** in settings, separately for uploads and
  downloads (`""`/`ask` = prompt).

Out of scope: Resume (SFTP offset append) — may be added later.

## Model (domain)

`internal/domain/transfer_conflict.go` (pure, no I/O):

- `ConflictAction` — typed enum with the 6 actions plus `ConflictAsk` sentinel
  (settings default only). `ParseConflictAction`/`String` for DTO round-trips.
- `FileStat { Exists bool; IsDir bool; Size int64; ModTime time.Time }`.
- `ResolveConflict(action ConflictAction, src, tgt FileStat) ConflictOutcome`
  where `ConflictOutcome` ∈ {`OutcomeWrite`, `OutcomeSkip`, `OutcomeRename`}.
  - No target (`!tgt.Exists`) → always `OutcomeWrite`.
  - Overwrite → Write. Rename → Rename. Skip → Skip.
  - Conditional actions evaluate against `src`/`tgt`; condition false → Skip.
- `NextAvailableName(base string, exists func(name string) bool) string` — pure
  numbering: `file.txt` → `file (1).txt` → `file (2).txt`, extension-aware.

Conflict = a **file** whose target path already exists (file or dir; a
dir-vs-file type mismatch is still a conflict). Directory-into-existing-directory
is a merge, never a conflict.

## Planning (usecase)

`internal/usecase/transfer_planner.go` — `TransferPlanner` builds a `TransferPlan`
without moving bytes:

- `TransferPlan { Kind string; Dirs []string; Files []PlannedFile }`.
- `PlannedFile { Source, Target string; Size int64; SrcModTime time.Time;
  Conflict *ConflictInfo }`; `ConflictInfo { Size int64; ModTime time.Time; IsDir bool }`.
- Dependencies injected as small ports (function/interface) so the same planner
  serves all three kinds:
  - source walk (local via `HostFileSystem`, remote via `RemoteFS.List`),
  - `pathOps` (Join/Split) — `path.*` for remote targets, `filepath.*` for local,
  - target-directory listing (one `List` per target dir → name→`FileStat` index),
    so conflict detection costs one round trip per directory, not per file.

Efficiency: sources are walked once; targets probed by directory. This is the
deliberate tradeoff vs a fully-streaming "backend asks frontend mid-transfer"
bridge — simpler and reliable, negligible overhead for typical drops.

## Execution (usecase)

Extend `TransferService` with `ExecutePlan(ctx, kind, sessionID, plan, resolutions, onProgress)`:

- `resolutions map[string]ResolvedAction` keyed by target path (only conflicts
  need an entry; others default to Write).
- Per file: `outcome := ResolveConflict(action, src, tgtFromPlan)`.
  Conditional actions use plan-time target metadata (sub-second-old; same TOCTOU
  window FileZilla has — documented).
  - Write: ensure parent dir; if target is a dir (type mismatch) remove it first;
    move the file to the target path.
  - Rename: pick a free name via `NextAvailableName` against the plan's
    sibling-name set (plus names used earlier this batch); move there.
  - Skip: no-op.
- File movement is delegated to a `fileMover` (SRP: one executor loop, three thin
  movers):
  - upload mover — `RemoteFS.Mkdir/RemoveAll/Upload`.
  - download mover — `RemoteFS.Download` + `HostFileSystem.Mkdir/Remove`.
  - localcopy mover — `HostFileSystem.Mkdir/Remove/CopyTo`.
- Aggregated progress via the existing `TransferProgress` pipeline (sum sizes of
  files that will be written); one batch `transferID` in the shared
  `cancelRegistry` for cancel.

`HostFileSystem` gains `CopyTo(srcPath, destPath string) error` (explicit
destination, needed for Rename); existing `Copy(src, destDir)` delegates to it.

## Presentation (Wails)

`handlers_transfers.go` + a DTO file:

- `PlanUpload(sessionID string, localPaths []string, remoteDir string) (TransferPlanDTO, error)`
- `PlanDownload(sessionID string, remotePaths []string, localDir string) (TransferPlanDTO, error)`
- `PlanLocalCopy(srcPaths []string, destDir string) (TransferPlanDTO, error)`
- `ExecuteUpload(sessionID string, req ExecutePlanDTO) error` (and Download/LocalCopy).

The plan travels through the trusted frontend (ADR-007: host UI is the trust
anchor; not plugin IPC), so no server-side plan registry is required. Paths are
still sanitized by the FS layer.

## Settings

`TransferSettings` += `DefaultUploadExistsAction`, `DefaultDownloadExistsAction`
(enum strings; `""`/`ask` = prompt). SettingsDialog Transfer section exposes both.

## Frontend

- `api/transferPlan.ts` — thin RPC wrappers.
- `conflictResolver.ts` (pure) — given a plan's conflicts + settings default,
  drives resolution: apply stored default without prompting; otherwise emit
  conflicts one at a time; "Always use this action" resolves the rest; Cancel
  aborts the whole batch. Unit-tested.
- `ConflictDialog.svelte` — FileZilla layout: Source/Target file boxes (name,
  path, size, date), 6-action radio group, editable rename field (shown for
  Rename, pre-filled with the auto-numbered suggestion), "Always use this action"
  checkbox, "Apply to current queue only" checkbox (unchecked → also persist as
  the settings default), OK/Cancel.
- `actions/transferActions.ts` — orchestration: plan → resolve → execute; used by
  the DnD integration points (`FileTree` upload drop, `LocalFileTree` download and
  OS-drop copy, `SessionView` handlers).

## Testing (TDD)

- domain: `ResolveConflict` across every action × newer/older/same/size/type
  mismatch; `NextAvailableName`.
- usecase: planner (target-path computation, directory expansion, conflict
  detection via fake ports); executor (skip/overwrite/rename/conditional, type
  mismatch, aggregated progress, cancel) via fakes.
- frontend: `conflictResolver` (default-from-settings skips dialog, Always applies
  to rest, Cancel aborts).

## SRP boundaries

One file / one function per reason to change: the pure resolution rules
(`transfer_conflict.go`), the planner, the executor loop, each `fileMover`, the
`pathOps` variants, the resolver orchestration, and the dialog are each isolated.
