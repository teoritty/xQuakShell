// Typed mirror of the generated Wails bindings in `frontend/wailsjs/go/main/App.d.ts`.
// Keep this interface in sync with that file: it is the compiler-checked contract
// between frontend code and the Go backend, replacing untyped `window.go.main.App` access.
import type { wails } from '../../wailsjs/go/models';

// `RawMessage` in the generated bindings serializes as an arbitrary JSON
// value over the Wails bridge; there is no corresponding exported TS type.
type RawMessage = any;

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

  InstallGitHubPlugin?(
    arg1: string,
    arg2: string,
    arg3: boolean,
    arg4: boolean,
    arg5: boolean,
    arg6: boolean,
    arg7: boolean
  ): Promise<void>;

  InstallPlugin?(
    arg1: string,
    arg2: boolean,
    arg3: boolean,
    arg4: boolean,
    arg5: boolean,
    arg6: boolean
  ): Promise<wails.PluginDTO>;

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
}

export interface RuntimeGateway {
  EventsOn(event: string, cb: (data: any) => void): void;
}
