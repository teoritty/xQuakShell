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
  /**
   * Message keys, not text. Both describe a permission the user is about to grant a third-party
   * binary, so they live under the security namespace where a language pack on disk can translate
   * them for a new language but never reword an existing one - "the plugin can ask the vault for
   * your passwords" is exactly the sentence an attacker would want softened.
   */
  labelKey: string;
  detailKey: string;
}

const CONSENT_CATALOGUE: Record<ConsentKey, Omit<ConsentItem, 'key'>> = {
  secret: {
    labelKey: 'security.plugin.consent.secret.label',
    detailKey: 'security.plugin.consent.secret.detail',
  },
  auth: {
    labelKey: 'security.plugin.consent.auth.label',
    detailKey: 'security.plugin.consent.auth.detail',
  },
  tunnel: {
    labelKey: 'security.plugin.consent.tunnel.label',
    detailKey: 'security.plugin.consent.tunnel.detail',
  },
  multiSession: {
    labelKey: 'security.plugin.consent.multiSession.label',
    detailKey: 'security.plugin.consent.multiSession.detail',
  },
  network: {
    labelKey: 'security.plugin.consent.network.label',
    detailKey: 'security.plugin.consent.network.detail',
  },
  exec: {
    labelKey: 'security.plugin.consent.exec.label',
    detailKey: 'security.plugin.consent.exec.detail',
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
  /**
   * A message key under the security namespace, or - for `text` - a sentence the backend produced.
   * Compatibility issues arrive already worded from Go and are shown as they came: they name
   * capability versions, and inventing a translation for a string this layer cannot parse would be
   * guessing at what the backend meant.
   */
  textKey?: string;
  vars?: Record<string, string | number>;
  text?: string;
}

/**
 * Trust warnings are separate from consents because they are not a permission the user grants -
 * they describe how much the bytes can be believed, and there is nothing to tick.
 */
export function localTrustWarnings(preview: PluginInstallPreview | null): TrustWarning[] {
  if (!preview) return [];
  const warnings: TrustWarning[] = [];
  if (preview.untrustedSignatureWarning) {
    warnings.push({ severity: 'critical', textKey: 'security.plugin.warning.untrustedSignature' });
  } else if (preview.unsignedWarning) {
    warnings.push({ severity: 'caution', textKey: 'security.plugin.warning.unsigned' });
  }
  if (!preview.checksumPresent) {
    warnings.push({ severity: 'caution', textKey: 'security.plugin.warning.noChecksum' });
  }
  return warnings;
}

export function sourceTrustWarnings(preview: GitHubPluginPreview | null): TrustWarning[] {
  if (!preview) return [];
  const warnings: TrustWarning[] = [];
  if (preview.unsignedPlugin) {
    warnings.push({ severity: 'caution', textKey: 'security.plugin.warning.unsigned' });
  }
  if (preview.untrustedSource) {
    warnings.push({ severity: 'critical', textKey: 'security.plugin.warning.untrustedSource' });
  }
  if (!preview.platformSupported) {
    warnings.push({
      severity: 'critical',
      textKey: 'security.plugin.warning.noAssetForPlatform',
      vars: { platform: preview.currentPlatform },
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
 * It is a key rather than prose in a component so the same sentence appears wherever a local
 * install can be started, including any entry point added later.
 */
export const LOCAL_INSTALL_WARNING_KEY = 'security.plugin.warning.localInstall';
