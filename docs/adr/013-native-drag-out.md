# ADR-013: Native Drag-Out of Files to the OS

## Status

Deferred

## Context

The file manager supports dragging files *into* the app: an OS Explorer drag over a
Remote/Local pane uploads/copies the dropped items. That inbound path is handled
entirely on the frontend (`frontend/src/lib/osFileDrop.ts`) on top of the Wails
runtime's `OnFileDrop`/`OnFileDropOff` callbacks — no Go relay (see the
os-file-drop architecture notes).

Users also want the reverse: dragging a file or folder *out* of a pane onto the OS
desktop or another application. There is currently no counterpart, and adding one
is not a small change:

- **Wails v2 exposes no drag-*source* API.** The runtime only offers the inbound
  `OnFileDrop` family. There is no `startDrag` equivalent (as Electron has via
  `webContents.startDrag`).
- **The webview owns its own HTML5 drag loop.** A real OS drag-source has to be
  native. On Windows that means an OLE `DoDragDrop` call with an `IDataObject`
  carrying `CF_HDROP`, driven on the UI thread via Cgo against the WebView2 window
  (HWND). Kicking that off from inside a webview `dragstart` is not
  straightforward and likely needs a custom mouse-driven drag trigger rather than
  the browser's native drag.
- **Remote files need staging.** A remote file has no local path to hand the OS, so
  it must first be downloaded to a temp directory. The plumbing exists
  (`app.go` `Download` + `GetTempDir()` → `portableData.EnsureTempDir`), but
  staging is asynchronous while `dragstart` is synchronous, so the two don't
  compose without a "prepare, then drag" flow.
- **Platform + version sensitivity.** The native path is Windows-only and depends
  on WebView2 behavior/version.

## Decision

Defer implementation. The cost and risk are high (native Cgo, UI-thread OLE,
Windows-only, WebView2-version-sensitive) and the best mechanism is not yet clear
— candidates include a native `DoDragDrop` source, a Chromium `DownloadURL`
file-promise served from a local endpoint, or a future Wails release that exposes
a drag-source primitive. We are not committing to one until a spike settles the
approach.

## Consequences

- The feature gap remains: files can be dragged in but not out.
- When revisited, start with a minimal native `DoDragDrop` spike that proves a
  drag can be initiated from the WebView2 window at all, before wiring either
  pane. Reuse the existing staging plumbing (`app.go` `Download`/`GetTempDir`,
  `internal/usecase/transfer_service.go`) for remote files.
- No inbound drop behavior changes; `osFileDrop.ts` is untouched.
