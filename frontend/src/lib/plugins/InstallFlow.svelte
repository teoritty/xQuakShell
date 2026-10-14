<script lang="ts">
  // preview -> consent -> install, for every origin: a registered source, a local folder or bundle,
  // and a plugin offered by the user's other devices (spec 6.4).
  //
  // It owns the sequence so no section has to know that an install is three calls, and so the two
  // origins cannot drift apart: a local folder and a repository differ only in which preview RPC
  // runs and which trust warnings that preview produces. Everything after that is shared.
  import { createEventDispatcher } from 'svelte';
  import InstallConsentDialog from './InstallConsentDialog.svelte';
  import {
    LOCAL_INSTALL_WARNING_KEY,
    localTrustWarnings,
    requiredConsents,
    sourceTrustWarnings,
    type ConsentAnswers,
    type ConsentItem,
    type TrustWarning,
  } from './installConsent';
  import { t } from '../../i18n/messages';
  import { installFromPath, installFromSource } from '../../actions/pluginsActions';
  import { installOfferedPlugin } from '../../actions/syncPluginActions';
  import { listPlugins, previewPluginInstall } from '../../api/plugins';
  import { previewSyncPluginOfferRpc } from '../../api/syncPlugins';
  import { previewGitHubPluginInstall } from '../../api/githubPlugins';
  import { githubInstallPreviewLines } from '../pluginDisplay';
  import type { GitHubPluginMetadata } from '../../api/githubPlugins';
  import type { PluginSourceDTO } from '../../api/pluginSources';

  const dispatch = createEventDispatcher();

  let open = false;
  let busy = false;
  let title = 'Install plugin';
  let summary: string[] = [];
  let consents: ConsentItem[] = [];
  let warnings: TrustWarning[] = [];
  let originWarning = '';
  let blockedReason = '';

  /** What confirm() will do. Set by whichever entry point opened the dialog. */
  let pending:
    | { kind: 'source'; sourceId: string; tag: string }
    | { kind: 'path'; path: string }
    | { kind: 'offer'; pluginId: string }
    | null = null;

  function fail(e: unknown, context: string) {
    const message = e instanceof Error ? e.message : String(e);
    dispatch('error', { message: `${context}: ${message}` });
    reset();
  }

  function reset() {
    open = false;
    busy = false;
    pending = null;
    blockedReason = '';
    originWarning = '';
  }

  /** Opens the confirmation for a plugin offered by a registered source. */
  export async function fromSource(
    source: PluginSourceDTO,
    plugin: GitHubPluginMetadata,
    releaseTag: string,
  ) {
    try {
      const preview = await previewGitHubPluginInstall(source.id, releaseTag);
      title = `Install ${preview.name}`;
      summary = githubInstallPreviewLines(preview.name, preview.releaseTag, preview.version, $t);
      consents = requiredConsents(preview);
      warnings = sourceTrustWarnings(preview);
      originWarning = '';
      blockedReason = preview.platformSupported
        ? ''
        : `No release asset targets ${preview.currentPlatform}.`;
      pending = { kind: 'source', sourceId: source.id, tag: preview.releaseTag || releaseTag };
      open = true;
    } catch (e) {
      fail(e, 'Preview install');
    }
  }

  /** Opens the confirmation for a folder or bundle the user picked off this machine. */
  export async function fromPath(path: string) {
    try {
      const preview = await previewPluginInstall(path);
      title = `Install ${preview.name}`;
      summary = [preview.name, `Version: ${preview.version}`, path];
      consents = requiredConsents(preview);
      warnings = localTrustWarnings(preview);
      originWarning = $t(LOCAL_INSTALL_WARNING_KEY);
      blockedReason = '';
      pending = { kind: 'path', path };
      open = true;
    } catch (e) {
      fail(e, 'Preview install');
    }
  }

  /**
   * Opens the confirmation for a plugin the user's other devices recorded. The consent screen is the
   * same one a source install shows: an offer is a suggestion from the user's own devices, and it
   * grants nothing the user has not read here.
   */
  export async function fromOffer(pluginId: string) {
    try {
      const preview = await previewSyncPluginOfferRpc(pluginId);
      if (!preview) throw new Error('Sync is unavailable.');
      title = `Install ${preview.name}`;
      summary = githubInstallPreviewLines(preview.name, preview.releaseTag, preview.version, $t);
      consents = requiredConsents(preview);
      warnings = sourceTrustWarnings(preview);
      originWarning = $t('sync.plugins.fromOtherDevice');
      blockedReason = preview.platformSupported ? '' : `No release asset targets ${preview.currentPlatform}.`;
      pending = { kind: 'offer', pluginId };
      open = true;
    } catch (e) {
      fail(e, 'Preview install');
    }
  }

  /** An offer install ends in 'installed', in 'mismatch' (nothing installed), or throws. */
  async function confirmOffer(pluginId: string, answers: ConsentAnswers) {
    const res = await installOfferedPlugin(pluginId, answers, false);
    if (res.error) throw new Error(res.error);
    reset();
    if (res.status === 'mismatch') {
      dispatch('mismatch', { pluginId, answers });
      return;
    }
    dispatch('installed', { plugins: await listPlugins() });
  }

  async function confirm(answers: ConsentAnswers) {
    if (!pending) return;
    busy = true;
    try {
      if (pending.kind === 'offer') {
        await confirmOffer(pending.pluginId, answers);
        return;
      }
      const plugins =
        pending.kind === 'source'
          ? await installFromSource(pending.sourceId, pending.tag, answers)
          : await installFromPath(pending.path, answers);
      reset();
      dispatch('installed', { plugins });
    } catch (e) {
      // The dialog stays open on failure: the consents are still ticked, so a retry after a
      // transient error does not make the user re-read and re-grant every permission.
      busy = false;
      const message = e instanceof Error ? e.message : String(e);
      dispatch('error', { message: `Install plugin: ${message}` });
    }
  }
</script>

<InstallConsentDialog
  show={open}
  {title}
  {summary}
  {consents}
  {warnings}
  {originWarning}
  {busy}
  {blockedReason}
  on:cancel={reset}
  on:confirm={(e) => confirm(e.detail.answers)}
/>
