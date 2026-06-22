/** PostCSS plugin: scale px lengths via --ui-scale for adaptive UI sizing. */

const UI_SCALE_VAR = 'var(--ui-scale, 1)';

function shouldSkipFile(file) {
  if (!file) return false;
  return file.includes('xterm.css') || file.includes('node_modules');
}

function transformPx(value) {
  if (!value || typeof value !== 'string') return value;
  if (value.includes('var(--ui-scale')) return value;

  return value.replace(/(\d+(?:\.\d+)?)px/g, (match, num) => {
    if (parseFloat(num) === 0) return '0';
    return `calc(${num}px * ${UI_SCALE_VAR})`;
  });
}

module.exports = () => ({
  postcssPlugin: 'postcss-ui-scale',
  Declaration(decl) {
    if (shouldSkipFile(decl.source?.input?.file)) return;
    decl.value = transformPx(decl.value);
  },
});

module.exports.postcss = true;
