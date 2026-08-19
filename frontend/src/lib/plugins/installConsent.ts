// The install consent catalogue: which warnings a preview raises, and what each one costs the
// user if they accept it.
//
// One module for both origins on purpose. A local folder and a repository produce previews whose
// warning fields differ only in the names of the two signing flags, and the old panel carried two
// hand-maintained copies of the six checkboxes - which is how a grant added to one origin comes
// to be missing from the other.
//
// This decides what to ASK. It does not decide what is allowed: the backend re-checks every
// consent in enforceInstallConsents and refuses an install whose grants do not cover its warnings,
// whatever this file rendered.
import type { PluginInstallPreview } from '../../api/plugins';
import type { GitHubPluginPreview } from '../../api/githubPlugins';

export type ConsentKey =
  | 'secret'
  | 'auth'
  | 'tunnel'
  | 'multiSession'
  | 'network'
  | 'exec';

export type ConsentAnswers = Record<ConsentKey, boolean>;

export const NO_CONSENTS: ConsentAnswers = {
  secret: false,
  auth: false,
  tunnel: false,
  multiSession: false,
  network: false,
  exec: false,
};

export interface ConsentItem {
  key: ConsentKey;
  label: string;
  /** What the plugin can do once this is granted, in the user's terms rather than the API's. */
  detail: string;
}

const CONSENT_CATALOGUE: Record<ConsentKey, Omit<ConsentItem, 'key'>> = {
  secret: {
    label: 'Read stored secrets',
    detail: 'The plugin can ask the vault for connection passwords and key passphrases.',
  },
  auth: {
    label: 'Act as an authentication provider',
    detail: 'The plugin can answer SSH authentication challenges on your behalf.',
  },
  tunnel: {
    label: 'Act as a tunnel provider',
    detail: 'The plugin can carry session traffic, and therefore see it.',
  },
  multiSession: {
    label: 'Share one process across sessions',
    detail: 'One plugin process serves every session, so a fault in it affects all of them.',
  },
  network: {
    label: 'Open arbitrary network connections',
    detail: 'The plugin is not restricted to the hosts it declared. It can reach any address.',
  },
  exec: {
    label: 'Run commands over the exec channel',
    detail: 'The plugin can execute commands on the remote host.',
  },
};

/** The union of the fields both preview shapes carry, so the rest of this file reads one thing. */
interface WarningFlags {
  requiresSecretAccess?: boolean;
  requiresAuthProviderAccess?: boolean;
  requiresTunnelProviderAccess?: boolean;
  multiSessionWarning?: boolean;
  arbitraryNetworkWarning?: boolean;
  execAccessWarning?: boolean;
}

/** The consents a preview requires, in a fixed order so the list does not reshuffle between opens. */
export function requiredConsents(preview: WarningFlags | null): ConsentItem[] {
  if (!preview) return [];
  const raised: Record<ConsentKey, boolean> = {
    secret: preview.requiresSecretAccess === true,
    auth: preview.requiresAuthProviderAccess === true,
    tunnel: preview.requiresTunnelProviderAccess === true,
    multiSession: preview.multiSessionWarning === true,
    network: preview.arbitraryNetworkWarning === true,
    exec: preview.execAccessWarning === true,
  };
  return (Object.keys(CONSENT_CATALOGUE) as ConsentKey[])
    .filter((key) => raised[key])
    .map((key) => ({ key, ...CONSENT_CATALOGUE[key] }));
}

/** True only when every raised consent has been ticked. An empty list is satisfied. */
export function allConsentsGiven(required: ConsentItem[], answers: ConsentAnswers): boolean {
  return required.every((item) => answers[item.key]);
}

export interface TrustWarning {
  /** `critical` is reserved for a signature that exists and does not verify. */
  severity: 'critical' | 'caution';
  text: string;
}

/**
 * Trust warnings are separate from consents because they are not a permission the user grants -
 * they describe how much the bytes can be believed, and there is nothing to tick.
 */
export function localTrustWarnings(preview: PluginInstallPreview | null): TrustWarning[] {
  if (!preview) return [];
  const warnings: TrustWarning[] = [];
  if (preview.untrustedSignatureWarning) {
    warnings.push({
      severity: 'critical',
      text: 'This plugin is signed by a key you do not trust.',
    });
  } else if (preview.unsignedWarning) {
    warnings.push({
      severity: 'caution',
      text: 'This plugin is not signed. Nothing proves who built it.',
    });
  }
  if (!preview.checksumPresent) {
    warnings.push({
      severity: 'caution',
      text: 'No checksum accompanies this plugin, so its files cannot be verified.',
    });
  }
  return warnings;
}

export function sourceTrustWarnings(preview: GitHubPluginPreview | null): TrustWarning[] {
  if (!preview) return [];
  const warnings: TrustWarning[] = [];
  if (preview.unsignedPlugin) {
    warnings.push({
      severity: 'caution',
      text: 'This plugin is not signed. Nothing proves who built it.',
    });
  }
  if (preview.untrustedSource) {
    warnings.push({
      severity: 'critical',
      text: 'This repository is not marked as trusted.',
    });
  }
  if (!preview.platformSupported) {
    warnings.push({
      severity: 'critical',
      text: `No release asset targets ${preview.currentPlatform}.`,
    });
  }
  if (!preview.compatible) {
    for (const issue of preview.compatibilityIssues) {
      warnings.push({ severity: 'critical', text: issue });
    }
  }
  return warnings;
}

/**
 * The banner shown before any install that did not come from a registered source.
 *
 * It is a constant rather than prose in a component so the same sentence appears wherever a local
 * install can be started, including any entry point added later.
 */
export const LOCAL_INSTALL_WARNING =
  'Installing from a folder or bundle skips every source check: nothing verifies where these ' +
  'files came from. Install only plugins you built yourself or obtained from someone you trust.';
