import type { ForwardRule } from '../../stores/appState';

export const DRAFT_RULE_UI_PREFIX = 'draft-rule-';

/** True when rule id was generated only for UI list keys, not persisted by backend. */
export function isDraftRuleUiId(id: string): boolean {
  return id.startsWith(DRAFT_RULE_UI_PREFIX);
}

export function createDraftRuleUiId(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return `${DRAFT_RULE_UI_PREFIX}${crypto.randomUUID()}`;
  }
  return `${DRAFT_RULE_UI_PREFIX}${Date.now()}`;
}

export function ensureRuleUiId(rule: ForwardRule): ForwardRule {
  if (rule.id) return { ...rule };
  return { ...rule, id: createDraftRuleUiId() };
}

function isDraftRuleComplete(rule: ForwardRule): boolean {
  if (rule.bindPort <= 0) return false;
  if (rule.kind === 'local' || rule.kind === 'remote') {
    return (rule.targetHost?.trim() ?? '') !== '' && (rule.targetPort ?? 0) > 0;
  }
  if (rule.kind === 'dynamic') {
    return (rule.pluginId?.trim() ?? '') !== '' && (rule.providerId?.trim() ?? '') !== '';
  }
  return false;
}

/** Remove UI-only rule ids so backend remains the canonical ID source. */
export function stripDraftRuleIdsForSave(rules: ForwardRule[]): ForwardRule[] {
  return rules.map((rule) => {
    if (!rule.id || isDraftRuleUiId(rule.id)) {
      const { id: _id, ...rest } = rule;
      return { ...rest, id: '' } as ForwardRule;
    }
    return rule;
  });
}

export function filterDraftRules(rules: ForwardRule[]): ForwardRule[] {
  return rules.filter(isDraftRuleComplete);
}

/**
 * Adopt backend-assigned rule IDs without dropping in-progress draft rows.
 *
 * Incomplete rules are local-only editor rows and are excluded from save payloads
 * by filterDraftRules. Persisted rules are sent in filterDraftRules order, so
 * saved rules zip 1:1 with complete draft rules while incomplete rows stay untouched.
 */
export function adoptPersistedRuleIds(draftRules: ForwardRule[], savedRules: ForwardRule[]): ForwardRule[] {
  let i = 0;
  return draftRules.map((rule) => {
    if (!isDraftRuleComplete(rule)) return rule;
    const saved = savedRules[i++];
    return saved?.id ? { ...rule, id: saved.id } : rule;
  });
}
