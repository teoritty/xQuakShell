import { mount } from 'svelte';
import LogViewerApp from './LogViewerApp.svelte';
import '../style.css';
import { applyLocale, initLocale } from '../i18n/apply';
import { fetchLaunchLocale } from '../api/locale';

// The log viewer is a second entry point with its own HTML, so it mounts
// separately. See the note in src/main.ts about the removed constructor.
// The log window is a separate process with its own bindings, so it loads the language itself.
// The log lines it shows stay English; only the window's own controls are translated.
//
// The language comes from the parent over the command line rather than from initLocale's
// localStorage mirror: that mirror belongs to the main window's WebView and reads back empty
// here, which left this window English while the rest of the application was not. initLocale
// remains the fallback for a parent that did not say.
const launched = await fetchLaunchLocale();
if (launched) {
  await applyLocale(launched);
} else {
  await initLocale();
}

const app = mount(LogViewerApp, { target: document.getElementById('app')! });
export default app;
