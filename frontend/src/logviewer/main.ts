import { mount } from 'svelte';
import LogViewerApp from './LogViewerApp.svelte';
import '../style.css';
import { initLocale } from '../i18n/apply';

// The log viewer is a second entry point with its own HTML, so it mounts
// separately. See the note in src/main.ts about the removed constructor.
// The log window is a separate process with its own bindings, so it loads the language itself.
// The log lines it shows stay English; only the window's own controls are translated.
await initLocale();

const app = mount(LogViewerApp, { target: document.getElementById('app')! });
export default app;
