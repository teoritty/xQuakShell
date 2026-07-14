import {
  activeSessionId,
  platform,
  type RemoteNode, type SSHIdentityMeta,
} from './appState';
import {
  importPassword,
  deletePassword,
  importIdentity,
  importPuTTYPPK,
  importPuTTYRegPreview,
  importPuTTYRegAsConnections,
  type PuTTYSessionPreview,
} from '../api/credentials';
import { addKnownHost, removeKnownHost } from '../api/knownHosts';
import { type AppSettings } from '../api/settings';
import {
  searchAuditLog,
  deleteAuditEntry,
  clearAuditLog,
  getAuditSessionState,
  enableAuditSecretLogging,
  disableAuditSecretLogging,
} from '../api/audit';
import {
  resolveHostKeyRpc,
  getPlatform,
} from '../api/sessions';

export {
  reportEmbedViewport,
  reportEmbedActivity,
  getPlatform,
} from '../api/sessions';
export { unlockVault, lockVault } from '../actions/vaultActions';
export { sendTerminalInput, terminalResize } from '../api/terminal';

export {
  takePendingTerminalOutput,
  clearPendingTerminalOutput,
  registerTerminalOutputConsumer,
} from '../terminal/outputBuffer';
export {
  type SessionHotkeysSettings,
  type AppSettings,
  type AuditEntry,
  type AuditSessionState,
  DEFAULT_SESSION_HOTKEYS,
} from '../api/settings';

export {
  refreshFolders,
  saveFolder,
  deleteFolder,
  moveFolder,
  moveFolders,
  reorderFolders,
  createNewFolderInFolder,
} from '../actions/folderActions';
export {
  refreshAllConnections,
  refreshIdentities,
  saveConnection,
  deleteConnection,
  moveConnections,
  reorderConnections,
  createNewConnectionInFolder,
} from '../actions/connectionActions';

export { importPassword, deletePassword };

export {
  openSession,
  closeSession,
  createSessionFromSelection,
  focusNextSessionTab,
  focusPrevSessionTab,
  closeActiveSession,
} from '../actions/sessionActions';

export async function resolveHostKey(sessionId: string, action: string, host: string, authorizedKey: string): Promise<void> {
  return resolveHostKeyRpc(sessionId, action, host, authorizedKey);
}

import {
  listPath,
  uploadFile,
  downloadFile,
  cancelTransfer,
  removePath,
  mkdirPath,
  createFilePath,
  copyLocalPath,
  renamePath,
  chmodPath,
  chownPath,
  chmodPathRecursive,
  chownPathRecursive,
  type ApplyTarget,
} from '../api/remoteFs';
export {
  listPath,
  uploadFile,
  downloadFile,
  cancelTransfer,
  removePath,
  mkdirPath,
  createFilePath,
  copyLocalPath,
  renamePath,
  chmodPath,
  chownPath,
  chmodPathRecursive,
  chownPathRecursive,
  type ApplyTarget,
};

export {
  removeLocalPath,
  mkdirLocalPath,
  renameLocalPath,
  createLocalFile,
  selectLocalFile,
  selectLocalDirectory,
  listLocalPath,
  getPortableDataRoot,
  getUserHomeDir,
  getTempDir,
  openFileWithSystem,
  startFileWatch,
  type LocalNode,
} from '../api/localFs';

export {
  addKnownHost,
  removeKnownHost,
};

export {
  importIdentity,
  importPuTTYPPK,
  importPuTTYRegPreview,
  importPuTTYRegAsConnections,
  type PuTTYSessionPreview,
};

export { normalizeHotkey, parseHotkeyEvent } from '../hotkeys/hotkeys';

export {
  getSettings,
  saveSettings,
  applyAppearanceSettings,
} from '../actions/settingsActions';

export {
  searchAuditLog,
  deleteAuditEntry,
  clearAuditLog,
  getAuditSessionState,
  enableAuditSecretLogging,
  disableAuditSecretLogging,
};

export { subscribeToEvents, sftpReadyPaths } from '../events/subscribe';

export {
  type PluginInfo,
  type PluginInstallPreview,
  type PluginSettings,
  type PluginPublisherKeyPair,
  listPlugins,
  pingPlugin,
  setPluginEnabled,
  selectPluginSourceDir,
  selectPluginBundleFile,
  getPluginSettings,
  savePluginSettings,
  generatePluginPublisherKeyPair,
  previewPluginInstall,
  installPluginRpc,
} from '../api/plugins';
import {
  type GitHubRepository,
  type GitHubReleaseSummary,
  type GitHubPluginMetadata,
  type GitHubPluginList,
  type GitHubPluginPreview,
  listGitHubRepositories,
  addGitHubRepository,
  removeGitHubRepository,
  setGitHubRepositoryTrust,
  fetchGitHubPlugins,
  previewGitHubPluginInstall,
} from '../api/githubPlugins';
export type {
  GitHubRepository,
  GitHubReleaseSummary,
  GitHubPluginMetadata,
  GitHubPluginList,
  GitHubPluginPreview,
} from '../api/githubPlugins';
export {
  listGitHubRepositories,
  addGitHubRepository,
  removeGitHubRepository,
  setGitHubRepositoryTrust,
  fetchGitHubPlugins,
  previewGitHubPluginInstall,
} from '../api/githubPlugins';

export {
  type FieldDef,
  type FieldGroup,
  type ConnectionProtocol,
  connectionProtocols,
  connectionProtocolCatalogKey,
  refreshConnectionProtocols,
  getPluginConnectionProtocols,
  invalidateProtocolsCache,
  installPlugin,
  installGitHubPlugin,
  uninstallGitHubPlugin,
} from '../actions/protocolActions';

export {
  getPluginContributions,
  executePluginCommand,
  preparePluginViewPanel,
  relayPluginViewMessage,
  releasePluginViewPanel,
} from '../api/pluginRuntime';
export type {
  PluginCommand,
  PluginContributions,
  PluginAuthMethodContribution,
  PluginTunnelProviderContribution,
  PluginView,
  PluginStatusBarItem,
} from '../api/pluginRuntime';
