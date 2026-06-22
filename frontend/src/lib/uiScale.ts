import { writable } from 'svelte/store';

export const DEFAULT_UI_SCALE_PERCENT = 100;
export const MIN_UI_SCALE_PERCENT = 75;
export const MAX_UI_SCALE_PERCENT = 200;

export const UI_SCALE_PRESETS = [75, 90, 100, 110, 125, 150, 175, 200] as const;

/** Reactive scale factor for Svelte templates (1.0 = 100%). */
export const uiScaleFactor = writable(1);

let currentScale = 1;
let iconObserver: MutationObserver | null = null;

export function normalizeUiScalePercent(value: number | undefined | null): number {
  const n = value ?? DEFAULT_UI_SCALE_PERCENT;
  return Math.min(MAX_UI_SCALE_PERCENT, Math.max(MIN_UI_SCALE_PERCENT, Math.round(n)));
}

export function getUiScaleFactor(): number {
  return currentScale;
}

export function scalePx(px: number): number {
  return Math.round(px * currentScale);
}

function scaleLucideIcon(svg: SVGElement): void {
  if (!svg.classList.contains('lucide')) return;

  if (!svg.dataset.uiBaseW) {
    const w = svg.getAttribute('width');
    if (w) svg.dataset.uiBaseW = w;
  }
  if (!svg.dataset.uiBaseH) {
    const h = svg.getAttribute('height');
    if (h) svg.dataset.uiBaseH = h;
  }
  if (!svg.dataset.uiBaseStroke && svg.hasAttribute('stroke-width')) {
    svg.dataset.uiBaseStroke = svg.getAttribute('stroke-width') ?? '';
  }

  const baseW = parseFloat(svg.dataset.uiBaseW || '24');
  const baseH = parseFloat(svg.dataset.uiBaseH || String(baseW));
  svg.setAttribute('width', String(Math.round(baseW * currentScale)));
  svg.setAttribute('height', String(Math.round(baseH * currentScale)));

  if (svg.dataset.uiBaseStroke) {
    const baseStroke = parseFloat(svg.dataset.uiBaseStroke);
    if (Number.isFinite(baseStroke)) {
      svg.setAttribute('stroke-width', String(baseStroke * currentScale));
    }
  }
}

function resyncLucideIcons(root: ParentNode = document): void {
  root.querySelectorAll('svg.lucide').forEach((el) => scaleLucideIcon(el as SVGElement));
}

function ensureIconObserver(): void {
  if (iconObserver || typeof MutationObserver === 'undefined') return;
  iconObserver = new MutationObserver((mutations) => {
    for (const mutation of mutations) {
      mutation.addedNodes.forEach((node) => {
        if (node instanceof SVGElement && node.classList.contains('lucide')) {
          scaleLucideIcon(node);
          return;
        }
        if (node instanceof Element) {
          resyncLucideIcons(node);
        }
      });
    }
  });
  iconObserver.observe(document.documentElement, { childList: true, subtree: true });
}

/** Applies adaptive UI scale: layout reflow via CSS --ui-scale, not browser zoom. */
export function applyUiScalePercent(percent: number): void {
  const normalized = normalizeUiScalePercent(percent);
  currentScale = normalized / 100;
  document.documentElement.style.removeProperty('zoom');
  document.documentElement.style.setProperty('--ui-scale', String(currentScale));
  uiScaleFactor.set(currentScale);
  ensureIconObserver();
  resyncLucideIcons();
  window.dispatchEvent(new Event('resize'));
  window.dispatchEvent(
    new CustomEvent('ui-scale-changed', {
      detail: { percent: normalized, factor: currentScale },
    }),
  );
}
