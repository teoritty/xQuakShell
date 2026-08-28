// Typed mirror of the generated Wails bindings in `frontend/wailsjs/go/main/App.d.ts`.
// Keep this interface in sync with that file: it is the compiler-checked contract
// between frontend code and the Go backend, replacing untyped `window.go.main.App` access.
import type { wails } from '../../wailsjs/go/models';

// `RawMessage` in the generated bindings serializes as an arbitrary JSON
// value over the Wails bridge; there is no corresponding exported TS type.
type RawMessage = any;

// Mirrors internal/presentation/wails.PluginSettingsSaveResultDTO. `reauthRequired` is a
// result rather than a thrown error because the caller acts on it: it re-asks for the master
// password and retries. Which changes need that password is decided in Go
// (domain.PluginTrustWeakened) and deliberately not mirrored here — a second copy of the rule
// would drift, and the drifted copy is the one an attacker aims at.
export interface PluginSettingsSaveResult {
  saved: boolean;
  reauthRequired: boolean;
}

// Mirrors internal/presentation/wails.PluginSourceDTO. Declared here rather than imported from
// wailsjs/go/models because that file is regenerated from the bound Go struct, and a hand-written
// mirror is what the compiler checks the rest of the frontend against.
//
// `available` is not the same question as "is the listing empty". A source that cannot answer at
// all shows `unavailableReason` and disables its actions; an available source with nothing in it
// is a repository that has published no release yet, which is a state the user can fix.
export interface PluginSourceDTO {
  id: string;
  kind: 'forge' | 'marketplace';
  displayName: string;
  trusted: boolean;
  removable: boolean;
  available: boolean;
  unavailableReason?: string;
  addedAt?: string;
  lastFetchedAt?: string;
}

// --- Transfer conflict planning (FileZilla-style existing-file handling) ---
// Mirrors the Go DTOs in internal/presentation/wails/dto_transfers.go. Defined
// here (the backend seam) rather than in api/ so the dependency direction stays
// api -> backend.

export interface ConflictInfoDTO {
  size: number;
  modTime: string;
  isDir: boolean;
}

export interface PlannedFileDTO {
  source: string;
  target: string;
  size: number;
  srcModTime: string;
  conflict?: ConflictInfoDTO;
}

export interface TransferPlanDTO {
  kind: string;
  // Operation id assigned during planning; the executor reuses it so the
  // scanning and byte-transfer phases share one Transfers-panel item.
  opID: string;
  dirs: string[];
  files: PlannedFileDTO[];
}

export interface ResolvedActionDTO {
  target: string;
  action: string;
  newName?: string;
}

export interface ExecutePlanDTO {
  plan: TransferPlanDTO;
  resolutions: ResolvedActionDTO[];
}

// Every method here is required, and none of them used to be: 43 of the 118 carried a `?`.
// The marker described nothing real. Wails generates App.d.ts from the bound Go struct and
// never emits an optional binding, and all 118 names appear there unconditionally - the split
// into 43 optional and 75 required matched no property of the bridge, it was just whichever
// ones happened to be added with a `?`.
//
// The cost of the fiction was paid where it is hardest to see. Under `strict: false` a call to
// a possibly-undefined method is not reported at all, so api/surfaces.ts copied the guard shape
// from api/terminal.ts - `if (!app) return;` and then call - without noticing that terminal's
// methods are required and surfaces' were not. Eleven call sites across five api/ modules ended
// up guarding the gateway but not the method, and nothing could say so.
//
// The runtime guards that DO exist, `if (!app?.SearchAuditLog) return []` in api/audit.ts and
// its siblings, stay exactly as they are. They are not dead code: audit.test.ts,
// githubPlugins.test.ts, pluginRuntime.test.ts and plugins.test.ts each drive a gateway with one
// method proxied away and assert the wrapper degrades instead of throwing. Deleting a guard
// turns one of those red, which is the correct outcome and the reason this comment names them.
export interface AppGateway {
  AddGitHubRepository(arg1: wails.AddGitHubRepositoryRequest): Promise<void>;

  CancelTransfer(arg1: string): Promise<void>;

  Chmod(arg1: string, arg2: string, arg3: number): Promise<void>;

  ChmodRecursive(arg1: string, arg2: string, arg3: number, arg4: string): Promise<void>;

  Chown(arg1: string, arg2: string, arg3: number, arg4: number): Promise<void>;

  ChownRecursive(arg1: string, arg2: string, arg3: number, arg4: number, arg5: string): Promise<void>;

  ClearAuditLog(arg1: string): Promise<void>;

  CloseSession(arg1: string): Promise<void>;

  CopyLocalPath(arg1: string, arg2: string): Promise<void>;

  CreateFilePath(arg1: string, arg2: string, arg3: string): Promise<void>;

  CreateLocalFile(arg1: string): Promise<void>;

  CreateVault(arg1: string): Promise<void>;

  DeleteAuditEntry(arg1: number): Promise<void>;

  DeleteConnection(arg1: string): Promise<void>;

  DeleteFolder(arg1: string): Promise<void>;

  DeletePassword(arg1: string): Promise<void>;

  DisableAuditSecretLogging(): Promise<void>;

  Download(arg1: string, arg2: string, arg3: string): Promise<void>;

  EnableAuditSecretLogging(arg1: boolean): Promise<void>;

  ExecutePluginCommand(arg1: string, arg2: string, arg3: RawMessage): Promise<RawMessage>;

  FetchGitHubPlugins(arg1: wails.FetchGitHubPluginsRequest): Promise<wails.GitHubPluginListDTO>;

  GeneratePluginPublisherKeyPair(): Promise<wails.PluginPublisherKeyPairDTO>;

  GetAllConnections(): Promise<Array<wails.ConnectionDTO>>;

  GetAuditSessionState(): Promise<wails.AuditSessionStateDTO>;

  GetFolders(): Promise<Array<wails.FolderDTO>>;

  // Key-manager bindings. Typed inline rather than through wails.* because these shapes are
  // generated at build time and the checked-in models lag behind the Go side; GetVersionInfo
  // already does the same. StoredKeyShape deliberately has no private-key field.
  GetKeys(): Promise<Array<StoredKeyShape>>;

  GetKeyUsages(arg1: string): Promise<Array<KeyUsageShape>>;

  GenerateKey(arg1: string, arg2: number, arg3: string, arg4: string, arg5: KeyOptionsShape): Promise<StoredKeyShape>;

  ImportKey(arg1: string, arg2: string, arg3: string, arg4: KeyOptionsShape): Promise<StoredKeyShape>;

  RenameKey(arg1: string, arg2: string): Promise<void>;

  SetKeyPolicy(arg1: string, arg2: KeyOptionsShape): Promise<void>;

  ChangeKeyPassphrase(arg1: string, arg2: string, arg3: string): Promise<void>;

  DeleteKey(arg1: string): Promise<void>;

  ExportKey(arg1: string, arg2: string, arg3: string, arg4: string): Promise<string>;

  DeployKey(arg1: string, arg2: string): Promise<{ added: boolean; alreadyPresent: boolean; path: string }>;

  PlanKeyMigration(arg1: string): Promise<{ required: boolean; keys: Array<{ id: string; comment: string; keyType: string }> }>;

  CompleteKeyMigration(arg1: string, arg2: Record<string, string>): Promise<{ fromVersion: number; toVersion: number; converted: string[]; skipped: string[]; backupPath: string }>;

  GetKnownHosts(): Promise<Array<wails.KnownHostDTO>>;

  GetPingResults(): Promise<Array<wails.PingResultDTO>>;

  GetPlatform(): Promise<string>;

  GetPluginConnectionProtocols(): Promise<Array<wails.ConnectionProtocolDTO>>;

  GetPluginContributions(): Promise<wails.PluginContributionsDTO>;

  GetPluginSettings(): Promise<wails.PluginSettingsDTO>;

  GetPortableDataRoot(): Promise<string>;

  GetSessionState(arg1: string): Promise<wails.SessionDTO>;

  GetSettings(): Promise<wails.AppSettingsDTO>;

  GetVersionInfo(): Promise<{ appVersion: string; coreVersion: string; pluginApiVersion: string }>;
  GetUpdateStatus?(): Promise<wails.UpdateStatusDTO>;

  // Optional because the language catalogue is wired at the composition root and a build that
  // failed to load its embedded packs leaves it unset; the interface stays English rather than
  // failing to start.
  ListLocales?(): Promise<Array<wails.LocaleInfoDTO>>;
  GetLocaleMessages?(arg1: string): Promise<wails.LocaleMessagesDTO>;

  // Bound only by the log viewer process, which has no vault to read the language from and no
  // access to the main window's localStorage mirror, so it is told at launch instead.
  LaunchLocale?(): Promise<string>;

  GetTempDir(): Promise<string>;

  GetUserHomeDir(): Promise<string>;

  ImportPassword(arg1: string, arg2: string): Promise<string>;

  ImportPuTTYPPK(arg1: string, arg2: string): Promise<string>;

  ImportPuTTYReg(arg1: string): Promise<Array<wails.PuTTYSessionDTO>>;

  ImportPuTTYRegAsConnections(arg1: string, arg2: string): Promise<Array<wails.ConnectionDTO>>;

  GetSSHConfigDefaultPath(): Promise<string>;

  PreviewSSHConfig(arg1: string): Promise<wails.SSHConfigPreviewDTO>;

  ImportSSHConfig(
    arg1: string,
    arg2: Array<string>,
    arg3: string,
    arg4: boolean
  ): Promise<wails.SSHConfigImportResultDTO>;

  InstallGitHubPlugin(
    arg1: string,
    arg2: string,
    arg3: boolean,
    arg4: boolean,
    arg5: boolean,
    arg6: boolean,
    arg7: boolean,
    arg8: boolean
  ): Promise<void>;

  InstallPlugin(
    arg1: string,
    arg2: boolean,
    arg3: boolean,
    arg4: boolean,
    arg5: boolean,
    arg6: boolean,
    arg7: boolean
  ): Promise<wails.PluginDTO>;

  // --- Discovery subtrees (ADR-014). Everything is addressed by connectionId;
  // the backend's sessionId never crosses this seam. ---
  GetDiscoveryTree(arg1: string): Promise<wails.DiscoverySnapshotDTO>;

  InvokeDiscoveryAction(
    arg1: string,
    arg2: string,
    arg3: Array<string>,
    arg4: string
  ): Promise<void>;

  SetDiscoveryObserved(arg1: string, arg2: Array<string>): Promise<void>;

  // --- Plugin UI surfaces (ADR-015). A surface is addressed by its own id; the session it
  // borrowed its authorization from never crosses this seam either. ---
  CloseSurface(arg1: string): Promise<void>;

  SendSurfaceInput(arg1: string, arg2: string): Promise<void>;

  ResizeSurface(arg1: string, arg2: number, arg3: number): Promise<void>;

  SubmitPluginDialog(arg1: string, arg2: Record<string, string>): Promise<void>;

  CancelPluginDialog(arg1: string): Promise<void>;

  // Typed structurally rather than through wails.*: the generated model bindings are checked in
  // and regenerating them is a separate step, while this shape is small and stable.
  DescribeDiscoveryNode(
    arg1: string,
    arg2: string,
    arg3: string
  ): Promise<{ sections: unknown[]; values: Record<string, string>; editable: boolean }>;

  ApplyDiscoveryNodeDetails(
    arg1: string,
    arg2: string,
    arg3: string,
    arg4: Record<string, string>
  ): Promise<void>;

  IsVaultUnlocked(): Promise<boolean>;

  ListGitHubRepositories(): Promise<Array<wails.GitHubRepositoryDTO>>;

  ListLocalPath(arg1: string, arg2: boolean): Promise<Array<wails.LocalNodeDTO>>;

  ListPath(arg1: string, arg2: string): Promise<Array<wails.RemoteNodeDTO>>;

  ListPluginSources(): Promise<Array<PluginSourceDTO>>;

  ListPlugins(): Promise<Array<wails.PluginDTO>>;

  LockVault(): Promise<void>;

  MkdirLocalPath(arg1: string): Promise<void>;

  MkdirPath(arg1: string, arg2: string, arg3: string): Promise<void>;

  MoveConnections(arg1: Array<string>, arg2: string): Promise<void>;

  MoveFolder(arg1: string, arg2: string): Promise<void>;

  OpenFileWithSystem(arg1: string, arg2: string): Promise<void>;

  OpenSession(arg1: string): Promise<string>;

  PingConnection(arg1: string): Promise<void>;

  PlanUpload(arg1: string, arg2: Array<string>, arg3: string): Promise<TransferPlanDTO>;

  PlanDownload(arg1: string, arg2: Array<string>, arg3: string): Promise<TransferPlanDTO>;

  PlanLocalCopy(arg1: Array<string>, arg2: string): Promise<TransferPlanDTO>;

  ExecuteUpload(arg1: string, arg2: ExecutePlanDTO): Promise<void>;

  ExecuteDownload(arg1: string, arg2: ExecutePlanDTO): Promise<void>;

  ExecuteLocalCopy(arg1: ExecutePlanDTO): Promise<void>;

  PingPlugin(arg1: string): Promise<wails.PluginPingResultDTO>;

  PreparePluginViewPanel(arg1: string, arg2: string): Promise<string>;

  PreviewGitHubPluginInstall(arg1: string, arg2: string): Promise<wails.GitHubPluginPreviewResponseDTO>;

  PreviewPluginInstall(arg1: string): Promise<wails.PluginInstallPreviewDTO>;

  RelayPluginViewMessage(arg1: string, arg2: RawMessage): Promise<void>;

  ReleasePluginViewPanel(arg1: string): Promise<void>;

  RemoveGitHubRepository(arg1: string): Promise<void>;

  RemoveKnownHost(arg1: string): Promise<void>;

  RemoveLocalPath(arg1: string): Promise<void>;

  RemovePath(arg1: string, arg2: string): Promise<void>;

  RenameLocalPath(arg1: string, arg2: string): Promise<void>;

  RenamePath(arg1: string, arg2: string, arg3: string): Promise<void>;

  ReorderConnections(arg1: Array<string>, arg2: string): Promise<void>;

  ReorderFolders(arg1: Array<string>, arg2: string): Promise<void>;

  ReportActivity(): Promise<void>;

  ReportEmbedActivity(arg1: string, arg2: boolean): Promise<void>;

  ReportEmbedViewport(arg1: string, arg2: number, arg3: number, arg4: number): Promise<void>;

  ReportMinimized(): Promise<void>;

  ReportRestored(): Promise<void>;

  ResolveHostKey(arg1: string, arg2: string): Promise<void>;

  ResolvePeerTrust(arg1: string, arg2: string, arg3: string): Promise<void>;

  GetPeerTrust(): Promise<Array<{ scope: string; subject: string; fingerprint: string; addedAt: string }>>;

  RemovePeerTrust(arg1: string, arg2: string): Promise<void>;

  SaveConnection(arg1: wails.ConnectionDTO): Promise<wails.ConnectionDTO>;

  SaveFolder(arg1: wails.FolderDTO): Promise<wails.FolderDTO>;

  SavePluginSettings(
    arg1: wails.PluginSettingsDTO,
    arg2: string,
  ): Promise<PluginSettingsSaveResult>;

  SaveSettings(arg1: wails.AppSettingsDTO): Promise<void>;

  SearchAuditLog(
    arg1: string,
    arg2: string,
    arg3: string,
    arg4: string,
    arg5: number,
    arg6: number
  ): Promise<Array<wails.AuditEntryDTO>>;

  SelectLocalDirectory(): Promise<string>;

  SelectLocalFile(): Promise<string>;

  SelectPluginBundleFile(): Promise<string>;

  SelectPluginSourceDir(): Promise<string>;

  SendTerminalInput(arg1: string, arg2: string, arg3: string): Promise<void>;

  SetGitHubRepositoryTrust(arg1: wails.SetGitHubRepositoryTrustRequest): Promise<void>;

  SetPluginEnabled(arg1: string, arg2: boolean): Promise<void>;

  StartFileWatch(arg1: string): Promise<void>;

  StartPlugin(arg1: string): Promise<void>;

  TerminalResize(arg1: string, arg2: number, arg3: number): Promise<void>;

  UninstallGitHubPlugin(arg1: string, arg2: boolean): Promise<void>;

  UnlockVault(arg1: string): Promise<void>;

  Upload(arg1: string, arg2: string, arg3: string): Promise<void>;

  ValidateTrustedPublisherKey(arg1: string): Promise<void>;

  VaultExists(): Promise<boolean>;
}

export interface RuntimeGateway {
  /**
   * Wails returns an unsubscribe function; the type allows void because the app's long-lived
   * subscriptions never used it and the test fakes do not return one. Callers that DO unsubscribe
   * (a terminal or a log surface, which outlive neither their tab nor each other) must handle
   * both shapes rather than assume a function is there.
   */
  EventsOn(event: string, cb: (data: any) => void): (() => void) | void;

  /**
   * Hands a URL to the OS so it opens in the user's real browser.
   *
   * This is the only way out of the app to the web that works. `window.open` cannot leave a Wails
   * webview, and it fails differently per platform, which is why it produced a bug report that read
   * as two unrelated ones: WebView2 answers it with a second chrome-less in-app window, while
   * WebKitGTK - what the Linux builds run on - drops the call silently unless the host handles its
   * `create` signal, so the button does nothing at all. Route external links through
   * lib/openExternal.ts rather than calling this directly.
   */
  BrowserOpenURL(url: string): void;
}

// Shapes for the key-manager bindings above.
export interface StoredKeyShape {
  id: string;
  comment: string;
  keyType: string;
  bits?: number;
  publicKey?: string;
  fingerprint?: string;
  encrypted: boolean;
  policy?: string;
  cachePolicy?: string;
  cacheTtlSeconds?: number;
  allowPlugins: boolean;
  nonExportable: boolean;
  migrationPending: boolean;
  createdAt?: string;
  source?: string;
}

export interface KeyUsageShape {
  connectionId: string;
  connectionName: string;
  username: string;
  hop?: string;
}

export interface KeyOptionsShape {
  cachePolicy: string;
  cacheTtlSeconds: number;
  allowPlugins: boolean;
  nonExportable: boolean;
}
