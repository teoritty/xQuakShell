// Typed mirror of the generated Wails bindings in `frontend/wailsjs/go/main/App.d.ts`.
// Keep this interface in sync with that file: it is the compiler-checked contract
// between frontend code and the Go backend, replacing untyped `window.go.main.App` access.
import type { wails } from '../../wailsjs/go/models';

// `RawMessage` in the generated bindings serializes as an arbitrary JSON
// value over the Wails bridge; there is no corresponding exported TS type.
type RawMessage = any;

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

export interface AppGateway {
  AddGitHubRepository?(arg1: wails.AddGitHubRepositoryRequest): Promise<void>;

  AddKnownHost(arg1: string, arg2: string): Promise<void>;

  CancelTransfer(arg1: string): Promise<void>;

  Chmod(arg1: string, arg2: string, arg3: number): Promise<void>;

  ChmodRecursive(arg1: string, arg2: string, arg3: number, arg4: string): Promise<void>;

  Chown(arg1: string, arg2: string, arg3: number, arg4: number): Promise<void>;

  ChownRecursive(arg1: string, arg2: string, arg3: number, arg4: number, arg5: string): Promise<void>;

  ClearAuditLog?(arg1: string): Promise<void>;

  CloseSession(arg1: string): Promise<void>;

  CopyLocalPath(arg1: string, arg2: string): Promise<void>;

  CreateFilePath(arg1: string, arg2: string, arg3: string): Promise<void>;

  CreateLocalFile(arg1: string): Promise<void>;

  CreateVault(arg1: string): Promise<void>;

  DeleteAuditEntry?(arg1: number): Promise<void>;

  DeleteConnection(arg1: string): Promise<void>;

  DeleteFolder(arg1: string): Promise<void>;

  DeletePassword(arg1: string): Promise<void>;

  DisableAuditSecretLogging?(): Promise<void>;

  Download(arg1: string, arg2: string, arg3: string): Promise<void>;

  EnableAuditSecretLogging?(arg1: boolean): Promise<void>;

  ExecutePluginCommand?(arg1: string, arg2: string, arg3: RawMessage): Promise<RawMessage>;

  FetchGitHubPlugins?(arg1: wails.FetchGitHubPluginsRequest): Promise<wails.GitHubPluginListDTO>;

  GeneratePluginPublisherKeyPair?(): Promise<wails.PluginPublisherKeyPairDTO>;

  GetAllConnections(): Promise<Array<wails.ConnectionDTO>>;

  GetAuditSessionState?(): Promise<wails.AuditSessionStateDTO>;

  GetFolders(): Promise<Array<wails.FolderDTO>>;

  GetIdentities(): Promise<Array<wails.IdentityDTO>>;

  GetKnownHosts(): Promise<Array<wails.KnownHostDTO>>;

  GetPingResults(): Promise<Array<wails.PingResultDTO>>;

  GetPlatform(): Promise<string>;

  GetPluginConnectionProtocols?(): Promise<Array<wails.ConnectionProtocolDTO>>;

  GetPluginContributions?(): Promise<wails.PluginContributionsDTO>;

  GetPluginSettings?(): Promise<wails.PluginSettingsDTO>;

  GetPortableDataRoot(): Promise<string>;

  GetSessionState(arg1: string): Promise<wails.SessionDTO>;

  GetSettings(): Promise<wails.AppSettingsDTO>;

  GetVersionInfo?(): Promise<{ appVersion: string; coreVersion: string; pluginApiVersion: string }>;

  GetTempDir(): Promise<string>;

  GetUserHomeDir(): Promise<string>;

  ImportIdentity(arg1: string, arg2: string): Promise<string>;

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

  InstallGitHubPlugin?(
    arg1: string,
    arg2: string,
    arg3: boolean,
    arg4: boolean,
    arg5: boolean,
    arg6: boolean,
    arg7: boolean,
    arg8: boolean
  ): Promise<void>;

  InstallPlugin?(
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
  GetDiscoveryTree?(arg1: string): Promise<wails.DiscoverySnapshotDTO>;

  InvokeDiscoveryAction?(
    arg1: string,
    arg2: string,
    arg3: Array<string>,
    arg4: string
  ): Promise<void>;

  SetDiscoveryObserved?(arg1: string, arg2: Array<string>): Promise<void>;

  // --- Plugin UI surfaces (ADR-015). A surface is addressed by its own id; the session it
  // borrowed its authorization from never crosses this seam either. ---
  CloseSurface?(arg1: string): Promise<void>;

  SendSurfaceInput?(arg1: string, arg2: string): Promise<void>;

  ResizeSurface?(arg1: string, arg2: number, arg3: number): Promise<void>;

  SubmitPluginDialog?(arg1: string, arg2: Record<string, string>): Promise<void>;

  CancelPluginDialog?(arg1: string): Promise<void>;

  IsVaultUnlocked(): Promise<boolean>;

  ListGitHubRepositories?(): Promise<Array<wails.GitHubRepositoryDTO>>;

  ListLocalPath(arg1: string, arg2: boolean): Promise<Array<wails.LocalNodeDTO>>;

  ListPath(arg1: string, arg2: string): Promise<Array<wails.RemoteNodeDTO>>;

  ListPlugins?(): Promise<Array<wails.PluginDTO>>;

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

  PingPlugin?(arg1: string): Promise<wails.PluginPingResultDTO>;

  PreparePluginViewPanel?(arg1: string, arg2: string): Promise<string>;

  PreviewGitHubPluginInstall?(arg1: string, arg2: string): Promise<wails.GitHubPluginPreviewResponseDTO>;

  PreviewPluginInstall?(arg1: string): Promise<wails.PluginInstallPreviewDTO>;

  RelayPluginViewMessage?(arg1: string, arg2: RawMessage): Promise<void>;

  ReleasePluginViewPanel?(arg1: string): Promise<void>;

  RemoveGitHubRepository?(arg1: string): Promise<void>;

  RemoveKnownHost(arg1: string): Promise<void>;

  RemoveLocalPath(arg1: string): Promise<void>;

  RemovePath(arg1: string, arg2: string): Promise<void>;

  RenameLocalPath(arg1: string, arg2: string): Promise<void>;

  RenamePath(arg1: string, arg2: string, arg3: string): Promise<void>;

  ReorderConnections(arg1: Array<string>, arg2: string): Promise<void>;

  ReorderFolders(arg1: Array<string>, arg2: string): Promise<void>;

  ReportActivity(): Promise<void>;

  ReportEmbedActivity?(arg1: string, arg2: boolean): Promise<void>;

  ReportEmbedViewport?(arg1: string, arg2: number, arg3: number, arg4: number): Promise<void>;

  ReportMinimized(): Promise<void>;

  ReportRestored(): Promise<void>;

  ResolveHostKey(arg1: string, arg2: string, arg3: string, arg4: string): Promise<void>;

  SaveConnection(arg1: wails.ConnectionDTO): Promise<wails.ConnectionDTO>;

  SaveFolder(arg1: wails.FolderDTO): Promise<wails.FolderDTO>;

  SavePluginSettings?(arg1: wails.PluginSettingsDTO): Promise<void>;

  SaveSettings(arg1: wails.AppSettingsDTO): Promise<void>;

  SearchAuditLog?(
    arg1: string,
    arg2: string,
    arg3: string,
    arg4: string,
    arg5: number,
    arg6: number
  ): Promise<Array<wails.AuditEntryDTO>>;

  SelectLocalDirectory(): Promise<string>;

  SelectLocalFile(): Promise<string>;

  SelectPluginBundleFile?(): Promise<string>;

  SelectPluginSourceDir?(): Promise<string>;

  SendTerminalInput(arg1: string, arg2: string, arg3: string): Promise<void>;

  SetGitHubRepositoryTrust?(arg1: wails.SetGitHubRepositoryTrustRequest): Promise<void>;

  SetPluginEnabled?(arg1: string, arg2: boolean): Promise<void>;

  StartFileWatch(arg1: string): Promise<void>;

  StartPlugin(arg1: string): Promise<void>;

  TerminalResize(arg1: string, arg2: number, arg3: number): Promise<void>;

  UninstallGitHubPlugin?(arg1: string, arg2: boolean): Promise<void>;

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
}
